package attestor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	attestorv2 "github.com/cosmos/ibc/api/v2/attestor"
)

const testPrivateKey = "0000000000000000000000000000000000000000000000000000000000000006"

func TestStartProbesPublicEndpointAndStopsProcess(t *testing.T) {
	binary := helperBinary(t)
	workDir := filepath.Join(t.TempDir(), "attestor")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	process, err := Start(ctx, Spec{
		BinaryPath:    binary,
		WorkDir:       workDir,
		Name:          "attestor-a",
		ChainID:       "observed-chain-1",
		PrivateKeyHex: testPrivateKey,
	})
	require.NoError(t, err)
	require.Equal(t, common.HexToAddress("0xE57bFE9F44b819898F47BF37E5AF72a0783e1141"), process.SignerAddress())
	height, err := process.latestAttestableHeight(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(4242), height)

	configInfo, err := os.Stat(filepath.Join(workDir, configFilename))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), configInfo.Mode().Perm())
	workDirInfo, err := os.Stat(workDir)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), workDirInfo.Mode().Perm())
	keyPath := filepath.Join(workDir, "keys", keyFilename)
	keyInfo, err := os.Stat(keyPath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), keyInfo.Mode().Perm())
	keyData, err := os.ReadFile(keyPath)
	require.NoError(t, err)
	var stored keyFile
	require.NoError(t, json.Unmarshal(keyData, &stored))
	require.Equal(t, "ecdsa", stored.Type)
	require.Equal(t, testPrivateKey, fmt.Sprintf("%x", mustDecodeBase64(t, stored.PrivateKey)))

	logs, err := os.ReadFile(filepath.Join(workDir, logFilename))
	require.NoError(t, err)
	require.Contains(t, string(logs), "helper listening")
	childPIDData, err := os.ReadFile(filepath.Join(workDir, "helper-child.pid"))
	require.NoError(t, err)
	childPID, err := strconv.Atoi(strings.TrimSpace(string(childPIDData)))
	require.NoError(t, err)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	require.NoError(t, process.Stop(stopCtx))
	require.NoError(t, process.Stop(stopCtx), "Stop must be idempotent")
	require.ErrorIs(t, syscall.Kill(childPID, 0), syscall.ESRCH, "Stop must signal descendants in the process group")
	logs, err = os.ReadFile(filepath.Join(workDir, logFilename))
	require.NoError(t, err)
	require.Contains(t, string(logs), "helper stopped")
}

func TestStartReportsEarlyProcessExitWithLogs(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "ibc-fail")
	require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\necho deliberate-start-failure >&2\nexit 17\n"), 0o700))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := Start(ctx, Spec{
		BinaryPath:    binary,
		WorkDir:       filepath.Join(t.TempDir(), "attestor"),
		Name:          "attestor-a",
		ChainID:       "observed-chain-1",
		PrivateKeyHex: testPrivateKey,
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "exited before readiness")
	require.ErrorContains(t, err, "deliberate-start-failure")
}

func TestStartRejectsInvalidInputsBeforeCreatingWorkspace(t *testing.T) {
	workDir := filepath.Join(t.TempDir(), "attestor")
	_, err := Start(context.Background(), Spec{
		BinaryPath:    "/definitely/not/a/binary",
		WorkDir:       workDir,
		Name:          "attestor-a",
		ChainID:       "observed-chain-1",
		PrivateKeyHex: "invalid",
	})
	require.ErrorContains(t, err, "parse private key")
	_, statErr := os.Stat(workDir)
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestStartRequiresFreshPrivateWorkspace(t *testing.T) {
	workDir := t.TempDir()
	_, err := Start(context.Background(), Spec{
		BinaryPath:    "/definitely/not/a/binary",
		WorkDir:       workDir,
		Name:          "attestor-a",
		ChainID:       "observed-chain-1",
		PrivateKeyHex: testPrivateKey,
	})
	require.ErrorContains(t, err, "create private work dir")
}

func TestStartRealIBCBinary(t *testing.T) {
	binary := os.Getenv("IBC_LINK_ATTESTOR_REAL_BIN")
	if binary == "" {
		t.Skip("set IBC_LINK_ATTESTOR_REAL_BIN to exercise a built Link binary")
	}
	absoluteBinary, err := filepath.Abs(binary)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	process, err := Start(ctx, Spec{
		BinaryPath:    absoluteBinary,
		WorkDir:       filepath.Join(t.TempDir(), "attestor"),
		Name:          "real-binary-attestor",
		ChainID:       "real-binary-probe",
		PrivateKeyHex: testPrivateKey,
	})
	require.NoError(t, err)
	require.NotZero(t, mustLatestHeight(ctx, t, process))
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer stopCancel()
	require.NoError(t, process.Stop(stopCtx))
}

func TestIBCLinkAttestorHelperProcess(_ *testing.T) {
	if os.Getenv("IBC_LINK_ATTESTOR_HELPER") != "1" {
		return
	}
	if err := runAttestorHelper(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func helperBinary(t *testing.T) string {
	t.Helper()
	testBinary, err := os.Executable()
	require.NoError(t, err)
	t.Setenv("IBC_LINK_ATTESTOR_TEST_BINARY", testBinary)

	path := filepath.Join(t.TempDir(), "ibc-helper")
	script := `#!/bin/sh
IBC_LINK_ATTESTOR_HELPER=1 exec "$IBC_LINK_ATTESTOR_TEST_BINARY" -test.run '^TestIBCLinkAttestorHelperProcess$' -- "$@"
`
	require.NoError(t, os.WriteFile(path, []byte(script), 0o700))
	return path
}

func runAttestorHelper() error {
	home, configName, err := helperConfigArgs(os.Args)
	if err != nil {
		return err
	}
	configData, err := os.ReadFile(filepath.Join(home, configName))
	if err != nil {
		return fmt.Errorf("helper read config: %w", err)
	}
	var config fileConfig
	if decodeErr := yaml.Unmarshal(configData, &config); decodeErr != nil {
		return fmt.Errorf("helper decode config: %w", decodeErr)
	}
	if len(config.Attestor.Attestations) != 1 || config.Attestor.Attestations[0].Name != "attestor-a" {
		return fmt.Errorf("helper received unexpected attestor config: %+v", config.Attestor)
	}
	if len(config.Signers) != 1 || config.Signers[0].Alias != signerAlias {
		return fmt.Errorf("helper received unexpected signer config: %+v", config.Signers)
	}
	if _, readErr := os.ReadFile(config.Signers[0].File); readErr != nil {
		return fmt.Errorf("helper read signer key: %w", readErr)
	}

	listener, err := net.Listen("tcp", config.Server.ListenAddress)
	if err != nil {
		return fmt.Errorf("helper listen: %w", err)
	}
	mux := http.NewServeMux()
	path, handler := attestorv2.NewAttestationServiceHandler(helperAttestationService{})
	mux.Handle(path, handler)
	server := &http.Server{Handler: mux}
	child := exec.Command("sleep", "60")
	if err := child.Start(); err != nil {
		return fmt.Errorf("helper start descendant: %w", err)
	}
	if err := os.WriteFile(
		filepath.Join(home, "helper-child.pid"),
		[]byte(strconv.Itoa(child.Process.Pid)+"\n"),
		0o600,
	); err != nil {
		return fmt.Errorf("helper write descendant pid: %w", err)
	}
	childDone := make(chan error, 1)
	go func() { childDone <- child.Wait() }()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	fmt.Fprintln(os.Stderr, "helper listening")
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case <-signals:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("helper shutdown: %w", err)
		}
		select {
		case <-childDone:
		case <-time.After(time.Second):
			return fmt.Errorf("helper descendant did not receive process-group signal")
		}
		fmt.Fprintln(os.Stderr, "helper stopped")
		return nil
	case err := <-done:
		return fmt.Errorf("helper server exited: %w", err)
	}
}

type helperAttestationService struct{}

func (helperAttestationService) LatestAttestableHeight(
	_ context.Context,
	req *connect.Request[attestorv2.LatestAttestableHeightRequest],
) (*connect.Response[attestorv2.LatestAttestableHeightResponse], error) {
	if req.Msg.Attestor != "attestor-a" {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("unknown attestor %q", req.Msg.Attestor))
	}
	return connect.NewResponse(&attestorv2.LatestAttestableHeightResponse{Height: 4242}), nil
}

func helperConfigArgs(args []string) (string, string, error) {
	var home, config string
	for i := 0; i < len(args)-1; i++ {
		switch args[i] {
		case "--home":
			home = args[i+1]
		case "--config":
			config = args[i+1]
		}
	}
	if home == "" || config == "" {
		return "", "", fmt.Errorf("helper requires --home and --config, got %q", args)
	}
	return home, config, nil
}

func mustDecodeBase64(t *testing.T, value string) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(value)
	require.NoError(t, err)
	return data
}

func mustLatestHeight(ctx context.Context, t *testing.T, process *Process) uint64 {
	t.Helper()
	height, err := process.latestAttestableHeight(ctx)
	require.NoError(t, err)
	return height
}
