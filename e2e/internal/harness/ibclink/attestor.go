// Package ibclink manages IBC Link processes as black boxes.
//
// Readiness means that the process loaded its private configuration and serves
// LatestHeight for the configured attestor. The current Link
// implementation returns a synthetic timestamp from that RPC; this package
// therefore does not claim that the process can produce or submit attestations.
package ibclink

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"gopkg.in/yaml.v3"

	"github.com/cosmos/ibc/link/keyfile"

	attestorv2 "github.com/cosmos/ibc/link/api/v2/attestor"
)

const (
	configFilename = "ibc.yml"
	keyFilename    = "attestor.json"
	logFilename    = "attestor.log"

	signerAlias = "managed-attestor-signer"

	attestorStartupTimeout = 30 * time.Second
	probeRequestTimeout    = 500 * time.Millisecond
	attestorStopGrace      = 12 * time.Second
	startupLogTailBytes    = 4096
)

// AttestorLaunch contains all process-local inputs needed to start one Link attestor.
// WorkDir must name a path that does not already exist; StartAttestor creates it with
// owner-only permissions and keeps the private key out of process arguments.
type AttestorLaunch struct {
	BinaryPath    string
	WorkDir       string
	Name          string
	ChainID       string
	PrivateKeyHex string
	RPCURL        string
	ICS26Router   string
}

type readinessResult struct {
	address string
	err     error
}

// AttestorProcess is a running IBC Link attestor whose public height endpoint has been
// successfully probed. It owns the subprocess group, but the caller owns the
// containing workspace and decides when its files are removed.
type AttestorProcess struct {
	signerAddress common.Address
	endpoint      string
	name          string
	client        attestorv2.AttestationServiceClient
	out           *logWriter
	readiness     <-chan readinessResult
	handle        *processHandle
}

// StartAttestor writes a private Link configuration and ECDSA key, starts `ibc
// attestor run`, and returns only after LatestHeight succeeds.
func StartAttestor(ctx context.Context, launch AttestorLaunch) (*AttestorProcess, error) {
	key, err := validateAttestorLaunch(launch)
	if err != nil {
		return nil, err
	}
	binaryPath, err := filepath.Abs(launch.BinaryPath)
	if err != nil {
		return nil, fmt.Errorf("start IBC Link attestor: absolute binary path: %w", err)
	}

	paths, err := prepareAttestorWorkspace(launch, key, "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	out, err := newLogWriter(paths.log)
	if err != nil {
		return nil, fmt.Errorf("start IBC Link attestor: create log: %w", err)
	}

	cmd := exec.Command(
		binaryPath,
		"attestor", "run",
		"--home", paths.dir,
		"--config", configFilename,
	)
	cmd.Dir = paths.dir
	cmd.Stderr = out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		out.close()
		return nil, fmt.Errorf("start IBC Link attestor: capture stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		out.close()
		return nil, fmt.Errorf("start IBC Link attestor binary %q: %w", binaryPath, err)
	}
	readiness := make(chan readinessResult, 1)
	stdoutDone := make(chan struct{})
	go func() {
		defer close(stdoutDone)
		observeReadiness(stdout, out, readiness)
	}()

	p := &AttestorProcess{
		signerAddress: crypto.PubkeyToAddress(key.PublicKey),
		name:          launch.Name,
		out:           out,
		readiness:     readiness,
	}
	p.handle = reapProcess(cmd, processHooks{
		BeforeWait: func() { <-stdoutDone },
		AfterWait:  out.close,
	})

	if err := p.awaitReady(ctx); err != nil {
		if cleanupErr := p.killAfterFailedStart(); cleanupErr != nil {
			return p, errors.Join(err, fmt.Errorf("clean up failed IBC Link attestor start: %w", cleanupErr))
		}
		return nil, err
	}
	return p, nil
}

// SignerAddress is the Ethereum address derived from the private ECDSA key
// loaded by the child process.
func (p *AttestorProcess) SignerAddress() common.Address { return p.signerAddress }

func (p *AttestorProcess) latestAttestableHeight(ctx context.Context) (uint64, error) {
	response, err := p.client.LatestHeight(
		ctx,
		connect.NewRequest(&attestorv2.LatestHeightRequest{Attestor: p.name}),
	)
	if err != nil {
		return 0, fmt.Errorf("call latest attestable height at %s: %w", p.endpoint, err)
	}
	return response.Msg.Height, nil
}

// Stop asks the whole subprocess group to exit and escalates to SIGKILL after
// a fixed grace period or when ctx expires. It is safe to call more than once.
func (p *AttestorProcess) Stop(ctx context.Context) error {
	return p.handle.signalAndWait(ctx, syscall.SIGTERM, attestorStopGrace)
}

func (p *AttestorProcess) awaitReady(ctx context.Context) error {
	startupCtx, cancel := context.WithTimeout(ctx, attestorStartupTimeout)
	defer cancel()
	address, err := p.awaitAddress(startupCtx)
	if err != nil {
		return err
	}
	p.endpoint = "http://" + address
	p.client = attestorv2.NewAttestationServiceClient(
		&http.Client{Timeout: probeRequestTimeout},
		p.endpoint,
	)

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	var lastProbeErr error
	for {
		probeCtx, probeCancel := context.WithTimeout(startupCtx, probeRequestTimeout)
		_, probeErr := p.latestAttestableHeight(probeCtx)
		probeCancel()
		if probeErr == nil {
			return nil
		}
		lastProbeErr = probeErr

		select {
		case <-p.handle.doneCh():
			return p.readinessExitError()
		case <-startupCtx.Done():
			return fmt.Errorf(
				"IBC Link attestor was not ready at %s: %w (last probe: %s); logs: %s",
				p.endpoint,
				startupCtx.Err(),
				lastProbeErr.Error(),
				p.logTail(),
			)
		case <-ticker.C:
		}
	}
}

func (p *AttestorProcess) awaitAddress(ctx context.Context) (string, error) {
	select {
	case result := <-p.readiness:
		if result.err != nil {
			if errors.Is(result.err, io.EOF) {
				return "", fmt.Errorf(
					"IBC Link attestor exited before readiness: %w; logs: %s",
					result.err,
					p.logTail(),
				)
			}
			return "", fmt.Errorf(
				"IBC Link attestor readiness failed: %w; logs: %s",
				result.err,
				p.logTail(),
			)
		}
		parsed, err := netip.ParseAddrPort(result.address)
		if err != nil || !parsed.Addr().IsLoopback() || parsed.Port() == 0 {
			return "", fmt.Errorf(
				"IBC Link attestor announced invalid readiness address %q",
				result.address,
			)
		}
		return result.address, nil
	case <-p.handle.doneCh():
		return "", p.readinessExitError()
	case <-ctx.Done():
		return "", fmt.Errorf(
			"IBC Link attestor did not announce readiness: %w; logs: %s",
			ctx.Err(),
			p.logTail(),
		)
	}
}

func (p *AttestorProcess) readinessExitError() error {
	processErr := p.handle.err()
	if processErr == nil {
		processErr = errors.New("process exited successfully")
	}
	return fmt.Errorf(
		"IBC Link attestor exited before readiness: %w; logs: %s",
		processErr,
		p.logTail(),
	)
}

func (p *AttestorProcess) killAfterFailedStart() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return p.handle.signalAndWait(ctx, syscall.SIGKILL, 0)
}

func (p *AttestorProcess) logTail() string {
	return strings.TrimSpace(string(p.out.tailSnapshot()))
}

func validateAttestorLaunch(spec AttestorLaunch) (*ecdsa.PrivateKey, error) {
	switch {
	case spec.BinaryPath == "":
		return nil, errors.New("start IBC Link attestor: binary path is required")
	case spec.WorkDir == "":
		return nil, errors.New("start IBC Link attestor: work dir is required")
	case spec.Name == "":
		return nil, errors.New("start IBC Link attestor: name is required")
	case spec.ChainID == "":
		return nil, errors.New("start IBC Link attestor: chain id is required")
	case spec.PrivateKeyHex == "":
		return nil, errors.New("start IBC Link attestor: private key is required")
	case spec.RPCURL == "":
		return nil, errors.New("start IBC Link attestor: chain RPC URL is required")
	case spec.ICS26Router == "":
		return nil, errors.New("start IBC Link attestor: ICS26 router address is required")
	}
	key, err := crypto.HexToECDSA(strings.TrimPrefix(spec.PrivateKeyHex, "0x"))
	if err != nil {
		return nil, fmt.Errorf("start IBC Link attestor: parse private key: %w", err)
	}
	return key, nil
}

type workspacePaths struct {
	dir string
	log string
}

func prepareAttestorWorkspace(
	spec AttestorLaunch,
	key *ecdsa.PrivateKey,
	listenAddress string,
) (workspacePaths, error) {
	dir, err := filepath.Abs(spec.WorkDir)
	if err != nil {
		return workspacePaths{}, fmt.Errorf("start IBC Link attestor: absolute work dir: %w", err)
	}
	if mkdirErr := os.Mkdir(dir, 0o700); mkdirErr != nil {
		return workspacePaths{}, fmt.Errorf("start IBC Link attestor: create private work dir %q: %w", dir, mkdirErr)
	}

	keyPath := filepath.Join(dir, "keys", keyFilename)
	if writeErr := keyfile.Store(keyPath, keyfile.ECDSA, crypto.FromECDSA(key)); writeErr != nil {
		return workspacePaths{}, fmt.Errorf("start IBC Link attestor: write private key file: %w", writeErr)
	}

	config := fileConfig{
		Server: serverConfig{ListenAddress: listenAddress},
		DB: dbConfig{
			Type: "sqlite",
			URL:  filepath.Join(dir, "ibc.db"),
		},
		Chains: []chainConfig{{
			ChainID: spec.ChainID,
			EVM: evmChainConfig{
				RPC:         spec.RPCURL,
				ICS26Router: spec.ICS26Router,
			},
		}},
		Attestor: attestorConfig{Attestations: []attestationConfig{{
			ChainID:        spec.ChainID,
			Name:           spec.Name,
			Signer:         signerAlias,
			FinalityOffset: 1,
		}}},
		Signers: []signerConfig{{
			Alias: signerAlias,
			Type:  typeLocal,
			File:  keyPath,
		}},
	}
	configData, err := yaml.Marshal(config)
	if err != nil {
		return workspacePaths{}, fmt.Errorf("start IBC Link attestor: encode config: %w", err)
	}
	if err := writePrivateFile(filepath.Join(dir, configFilename), configData); err != nil {
		return workspacePaths{}, fmt.Errorf("start IBC Link attestor: write config: %w", err)
	}
	return workspacePaths{dir: dir, log: filepath.Join(dir, logFilename)}, nil
}

func writePrivateFile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

type logWriter struct {
	mu   sync.Mutex
	tail []byte
	file *os.File
}

func newLogWriter(path string) (*logWriter, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &logWriter{file: file}, nil
}

func (w *logWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.appendTail(data)
	if w.file == nil {
		return 0, os.ErrClosed
	}
	return w.file.Write(data)
}

func observeReadiness(stdout io.Reader, logs io.Writer, ready chan<- readinessResult) {
	reader := bufio.NewReaderSize(stdout, startupLogTailBytes)
	line, tooLong, err := reader.ReadLine()
	_, _ = logs.Write(line)
	if !tooLong {
		_, _ = logs.Write([]byte{'\n'})
	}
	switch {
	case err != nil:
		ready <- readinessResult{err: fmt.Errorf("read readiness line: %w", err)}
	case tooLong:
		ready <- readinessResult{err: fmt.Errorf("readiness line exceeds %d bytes", startupLogTailBytes)}
	default:
		var readiness attestorv2.ProcessReadiness
		if err := json.Unmarshal(line, &readiness); err != nil {
			ready <- readinessResult{err: fmt.Errorf("decode readiness: %w", err)}
		} else if readiness.Event != attestorv2.ProcessReadinessEvent {
			ready <- readinessResult{err: fmt.Errorf("unexpected readiness event %q", readiness.Event)}
		} else {
			ready <- readinessResult{address: readiness.HTTP}
		}
	}
	_, _ = io.Copy(logs, reader)
}

func (w *logWriter) appendTail(data []byte) {
	if len(data) >= startupLogTailBytes {
		w.tail = append(w.tail[:0], data[len(data)-startupLogTailBytes:]...)
		return
	}
	w.tail = append(w.tail, data...)
	if overflow := len(w.tail) - startupLogTailBytes; overflow > 0 {
		copy(w.tail, w.tail[overflow:])
		w.tail = w.tail[:startupLogTailBytes]
	}
}

func (w *logWriter) tailSnapshot() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]byte(nil), w.tail...)
}

func (w *logWriter) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
}

type fileConfig struct {
	Server   serverConfig       `yaml:"server"`
	DB       dbConfig           `yaml:"db"`
	Chains   []chainConfig      `yaml:"chains"`
	Attestor attestorConfig     `yaml:"attestor"`
	Relayer  *relayerFileConfig `yaml:"relayer,omitempty"`
	Signers  []signerConfig     `yaml:"signers"`
}

type chainConfig struct {
	ChainID string         `yaml:"chainId"`
	EVM     evmChainConfig `yaml:"evm"`
}

type evmChainConfig struct {
	RPC         string `yaml:"rpc"`
	ICS26Router string `yaml:"ics26Router"`
}

type serverConfig struct {
	ListenAddress string `yaml:"listenAddr"`
}

type dbConfig struct {
	Type string `yaml:"type"`
	URL  string `yaml:"url"`
}

type attestorConfig struct {
	Attestations []attestationConfig `yaml:"attestations"`
}

type attestationConfig struct {
	ChainID string `yaml:"chainId"`
	Name    string `yaml:"name"`
	Signer  string `yaml:"signer"`

	// FinalityOffset is kept at 1 ("latest" minus one block) because the dev
	// chains behind the harness do not expose the "finalized" block tag.
	FinalityOffset uint `yaml:"finalityOffset"`
}

type signerConfig struct {
	Alias string `yaml:"alias"`
	Type  string `yaml:"type"`
	File  string `yaml:"file"`
}
