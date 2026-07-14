// Package attestor manages the current IBC Link attestor process as a black box.
//
// Readiness means that the process loaded its private configuration and serves
// LatestAttestableHeight for the configured attestor. The current Link
// implementation returns a synthetic timestamp from that RPC; this package
// therefore does not claim that the process can produce or submit attestations.
package attestor

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"gopkg.in/yaml.v3"

	"github.com/cosmos/ibc/link/harness/internal/ports"
	"github.com/cosmos/ibc/link/harness/internal/proc"
)

const (
	configFilename = "ibc.yml"
	keyFilename    = "attestor.json"
	logFilename    = "attestor.log"

	signerAlias = "managed-attestor-signer"

	latestAttestableHeightPath = "/ibc.v2.attestor.AttestationService/LatestAttestableHeight"
	defaultStartupTimeout      = 30 * time.Second
	probeRequestTimeout        = 500 * time.Millisecond
	stopGrace                  = 12 * time.Second
	startupLogTailBytes        = 4096
)

// Spec contains all process-local inputs needed to start one Link attestor.
// WorkDir must name a path that does not already exist; Start creates it with
// owner-only permissions and keeps the private key out of process arguments.
type Spec struct {
	BinaryPath    string
	WorkDir       string
	Name          string
	ChainID       string
	PrivateKeyHex string
}

// Process is a running IBC Link attestor whose public height endpoint has been
// successfully probed. It owns the subprocess group, but the caller owns the
// containing workspace and decides when its files are removed.
type Process struct {
	signerAddress common.Address
	endpoint      string
	name          string
	http          *http.Client
	out           *logWriter
	handle        *proc.Handle
}

// Start writes a private Link configuration and ECDSA key, starts `ibc
// attestor run`, and returns only after LatestAttestableHeight succeeds.
func Start(ctx context.Context, spec Spec) (*Process, error) {
	key, err := validateSpec(spec)
	if err != nil {
		return nil, err
	}
	binaryPath, err := filepath.Abs(spec.BinaryPath)
	if err != nil {
		return nil, fmt.Errorf("start IBC Link attestor: absolute binary path: %w", err)
	}

	port, err := ports.FreePort()
	if err != nil {
		return nil, fmt.Errorf("start IBC Link attestor: allocate listen port: %w", err)
	}
	listenAddress := "127.0.0.1:" + strconv.Itoa(port)
	endpoint := "http://" + listenAddress

	paths, err := prepareWorkspace(spec, key, listenAddress)
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
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		out.close()
		return nil, fmt.Errorf("start IBC Link attestor binary %q: %w", binaryPath, err)
	}

	p := &Process{
		signerAddress: crypto.PubkeyToAddress(key.PublicKey),
		endpoint:      endpoint,
		name:          spec.Name,
		http:          &http.Client{Timeout: probeRequestTimeout},
		out:           out,
	}
	p.handle = proc.Reap(cmd, proc.Hooks{AfterWait: out.close})

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
func (p *Process) SignerAddress() common.Address { return p.signerAddress }

func (p *Process) latestAttestableHeight(ctx context.Context) (uint64, error) {
	body, err := json.Marshal(latestHeightRequest{Attestor: p.name})
	if err != nil {
		return 0, fmt.Errorf("encode latest attestable height request: %w", err)
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.endpoint+latestAttestableHeightPath,
		bytes.NewReader(body),
	)
	if err != nil {
		return 0, fmt.Errorf("build latest attestable height request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")

	resp, err := p.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("call latest attestable height at %s: %w", p.endpoint, err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return 0, fmt.Errorf(
			"call latest attestable height at %s: %s: %s",
			p.endpoint,
			resp.Status,
			strings.TrimSpace(string(responseBody)),
		)
	}

	var result latestHeightResponse
	if decodeErr := json.NewDecoder(resp.Body).Decode(&result); decodeErr != nil {
		return 0, fmt.Errorf("decode latest attestable height response: %w", decodeErr)
	}
	height, err := parseProtoUint64(result.Height)
	if err != nil {
		return 0, fmt.Errorf("decode latest attestable height response: %w", err)
	}
	return height, nil
}

// Stop asks the whole subprocess group to exit and escalates to SIGKILL after
// a fixed grace period or when ctx expires. It is safe to call more than once.
func (p *Process) Stop(ctx context.Context) error {
	return p.handle.SignalAndWait(ctx, syscall.SIGTERM, stopGrace)
}

func (p *Process) awaitReady(ctx context.Context) error {
	startupCtx, cancel := context.WithTimeout(ctx, defaultStartupTimeout)
	defer cancel()

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
		case <-p.handle.Done():
			processErr := p.handle.Err()
			if processErr == nil {
				processErr = errors.New("process exited successfully")
			}
			return fmt.Errorf(
				"IBC Link attestor exited before readiness: %w; logs: %s",
				processErr,
				p.logTail(),
			)
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

func (p *Process) killAfterFailedStart() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return p.handle.SignalAndWait(ctx, syscall.SIGKILL, 0)
}

func (p *Process) logTail() string {
	return strings.TrimSpace(string(p.out.tailSnapshot()))
}

func validateSpec(spec Spec) (*ecdsa.PrivateKey, error) {
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

func prepareWorkspace(spec Spec, key *ecdsa.PrivateKey, listenAddress string) (workspacePaths, error) {
	dir, err := filepath.Abs(spec.WorkDir)
	if err != nil {
		return workspacePaths{}, fmt.Errorf("start IBC Link attestor: absolute work dir: %w", err)
	}
	if mkdirErr := os.Mkdir(dir, 0o700); mkdirErr != nil {
		return workspacePaths{}, fmt.Errorf("start IBC Link attestor: create private work dir %q: %w", dir, mkdirErr)
	}

	keysDir := filepath.Join(dir, "keys")
	if mkdirErr := os.Mkdir(keysDir, 0o700); mkdirErr != nil {
		return workspacePaths{}, fmt.Errorf("start IBC Link attestor: create private keys dir: %w", mkdirErr)
	}
	keyPath := filepath.Join(keysDir, keyFilename)
	keyData, err := json.Marshal(keyFile{
		Type:       "ecdsa",
		PrivateKey: base64.StdEncoding.EncodeToString(crypto.FromECDSA(key)),
	})
	if err != nil {
		return workspacePaths{}, fmt.Errorf("start IBC Link attestor: encode private key file: %w", err)
	}
	if writeErr := writePrivateFile(keyPath, keyData); writeErr != nil {
		return workspacePaths{}, fmt.Errorf("start IBC Link attestor: write private key file: %w", writeErr)
	}

	config := fileConfig{
		Server: serverConfig{ListenAddress: listenAddress},
		DB: dbConfig{
			Type: "sqlite",
			URL:  filepath.Join(dir, "ibc.db"),
		},
		Attestor: attestorConfig{Attestations: []attestationConfig{{
			ChainID: spec.ChainID,
			Name:    spec.Name,
			Signer:  signerAlias,
		}}},
		Signers: []signerConfig{{
			Alias: signerAlias,
			Type:  "local",
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

type latestHeightRequest struct {
	Attestor string `json:"attestor"`
}

type latestHeightResponse struct {
	Height json.RawMessage `json:"height"`
}

func parseProtoUint64(raw json.RawMessage) (uint64, error) {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return 0, errors.New("height is missing")
	}
	text = strings.Trim(text, `"`)
	height, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid height %q: %w", text, err)
	}
	return height, nil
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
	Server   serverConfig   `yaml:"server"`
	DB       dbConfig       `yaml:"db"`
	Attestor attestorConfig `yaml:"attestor"`
	Signers  []signerConfig `yaml:"signers"`
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
}

type signerConfig struct {
	Alias string `yaml:"alias"`
	Type  string `yaml:"type"`
	File  string `yaml:"file"`
}

type keyFile struct {
	Type       string `json:"type"`
	PrivateKey string `json:"privateKeyBase64"`
}
