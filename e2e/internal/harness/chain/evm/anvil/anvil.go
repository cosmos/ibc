package anvil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"github.com/testcontainers/testcontainers-go"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm/container"
	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm/poll"

	chainpkg "github.com/cosmos/ibc/e2e/internal/harness/chain"
	containertypes "github.com/moby/moby/api/types/container"
)

const (
	DefaultDockerImage = "ghcr.io/foundry-rs/foundry:v1.7.1"
	DockerImageEnv     = "ANVIL_DOCKER_IMAGE"

	anvilGasLimit     = 50_000_000
	anvilReadyTimeout = 30 * time.Second
	// Terminal shutdown leaves time for Anvil to flush its --state file.
	anvilStopGrace   = 5 * time.Second
	anvilStopTimeout = 15 * time.Second
	anvilLogTimeout  = 10 * time.Second

	containerStateDir     = "/anvil-state"
	anvilStartupTailBytes = 4096
	pollInterval          = 50 * time.Millisecond
)

type Spec struct {
	ID      string
	ChainID uint64
	LogPath string
	RunID   string
	Image   string
	// StatePath stores chain state on graceful container shutdown; empty derives it from LogPath.
	StatePath string

	// BlockTime > 0 seals blocks via --block-time (whole seconds; rounded, rejected if it rounds
	// to zero); 0 keeps Anvil in instant/automine mode.
	BlockTime time.Duration
}

type Chain struct {
	client *evm.EVMClient
	evm.Identity

	logPath string

	spec      Spec
	container testcontainers.Container

	clientMu sync.RWMutex

	mu      sync.Mutex
	stopped bool
	closed  bool

	fundingMu sync.Mutex
}

var (
	_ chainpkg.Chain     = (*Chain)(nil)
	_ chainpkg.EOAFunder = (*Chain)(nil)
)

func DockerImage() string {
	if image := os.Getenv(DockerImageEnv); image != "" {
		return image
	}
	return DefaultDockerImage
}

func Start(ctx context.Context, spec Spec) (*Chain, error) {
	if spec.ID == "" {
		return nil, errors.New("anvil chain id is empty")
	}
	if spec.ChainID == 0 {
		return nil, fmt.Errorf("anvil chain %s: EVM chain id is required", spec.ID)
	}
	var err error
	if spec.StatePath == "" {
		spec.StatePath, err = deriveStatePath(spec)
		if err != nil {
			return nil, err
		}
	}
	spec, stateContainerPath, mountSpec, err := normalizeDockerSpec(spec)
	if err != nil {
		return nil, err
	}

	container, rpcURL, ec, err := launchAnvil(ctx, spec, stateContainerPath, mountSpec)
	if err != nil {
		return nil, err
	}

	return &Chain{
		client:    ec,
		Identity:  evm.NewIdentity(spec.ID, rpcURL),
		logPath:   spec.LogPath,
		spec:      spec,
		container: container,
	}, nil
}

func launchAnvil(
	ctx context.Context,
	spec Spec,
	stateContainerPath, mountSpec string,
) (testcontainers.Container, string, *evm.EVMClient, error) {
	args := []string{
		"--port", "8545",
		"--host", "0.0.0.0",
		"--chain-id", strconv.FormatUint(spec.ChainID, 10),
		"--gas-limit", strconv.Itoa(anvilGasLimit),
		"--state", stateContainerPath,
		// Managed chains use explicit signers. Keeping Anvil's deterministic accounts disabled
		// avoids creating ambient identities, and quiet mode keeps its mnemonic out of diagnostics.
		"--accounts", "0",
		"--quiet",
	}
	if spec.BlockTime > 0 {
		secs := uint64(math.Round(spec.BlockTime.Seconds()))
		if secs == 0 {
			return nil, "", nil, fmt.Errorf(
				"anvil (chain %s): block time %v is below the 1s granularity of --block-time",
				spec.ID,
				spec.BlockTime,
			)
		}
		args = append(args, "--block-time", strconv.FormatUint(secs, 10))
	}

	request := testcontainers.ContainerRequest{
		Name:         containerName(spec),
		Image:        spec.Image,
		Entrypoint:   []string{"anvil"},
		Cmd:          args,
		ExposedPorts: []string{"8545/tcp"},
		Labels:       container.Labels(spec.RunID),
		HostConfigModifier: func(config *containertypes.HostConfig) {
			container.BindPortsToLoopback(config, "8545/tcp")
			config.Mounts = append(config.Mounts, mount.Mount{
				Type:   mount.TypeBind,
				Source: mountSpec,
				Target: containerStateDir,
			})
		},
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: request,
	})
	if err != nil {
		return nil, "", nil, fmt.Errorf("create anvil container (chain %s): %w", spec.ID, err)
	}
	if startErr := container.Start(ctx); startErr != nil {
		startErr = fmt.Errorf("start anvil container (chain %s): %w", spec.ID, startErr)
		return nil, "", nil, errors.Join(startErr, cleanupFailedStart(container))
	}
	host, err := container.Host(ctx)
	if err != nil {
		return nil, "", nil, errors.Join(fmt.Errorf("resolve anvil host: %w", err), cleanupFailedStart(container))
	}
	port, err := container.MappedPort(ctx, "8545/tcp")
	if err != nil {
		return nil, "", nil, errors.Join(
			fmt.Errorf("resolve anvil RPC port: %w", err),
			cleanupFailedStart(container),
		)
	}
	rpcURL := fmt.Sprintf("http://%s:%s", host, port.Port())

	ec, err := connectAnvil(ctx, spec, rpcURL)
	if err != nil {
		startupErr := anvilStartupError(container, err)
		return nil, "", nil, errors.Join(startupErr, cleanupFailedStart(container))
	}
	return container, rpcURL, ec, nil
}

func connectAnvil(ctx context.Context, spec Spec, rpcURL string) (*evm.EVMClient, error) {
	// HTTP dialing is lazy and succeeds on an unbound port; readiness is probed separately below.
	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return nil, fmt.Errorf("dial anvil (chain %s): %w", spec.ID, err)
	}

	probe := func(ctx context.Context) error {
		_, blockErr := client.BlockNumber(ctx)
		return blockErr
	}
	if readyErr := waitReady(ctx, probe, anvilReadyTimeout); readyErr != nil {
		client.Close()
		return nil, fmt.Errorf("anvil (chain %s) readiness: %w", spec.ID, readyErr)
	}

	return evm.NewVerifiedClient(ctx, client, spec.ChainID, fmt.Sprintf("anvil (chain %s)", spec.ID))
}

func (ac *Chain) WithEVMClient(use func(*evm.EVMClient) error) error {
	ac.clientMu.RLock()
	defer ac.clientMu.RUnlock()
	if ac.client == nil {
		return fmt.Errorf("anvil chain %s: EVM client is unavailable", ac.ID())
	}
	return use(ac.client)
}

func (ac *Chain) Height(ctx context.Context) (uint64, error) {
	var height uint64
	err := ac.WithEVMClient(func(client *evm.EVMClient) error {
		var err error
		height, err = client.Height(ctx)
		return err
	})
	return height, err
}

func (ac *Chain) replaceEVMClient(client *evm.EVMClient) {
	ac.clientMu.Lock()
	defer ac.clientMu.Unlock()
	if ac.client != nil {
		ac.client.Close()
	}
	ac.client = client
}

func (ac *Chain) closeEVMClient() {
	ac.clientMu.Lock()
	defer ac.clientMu.Unlock()
	if ac.client != nil {
		ac.client.Close()
		ac.client = nil
	}
}

// EnsureEOABalance uses Anvil's development control to set and then verify the
// requested minimum without exposing that control through the resolved Chain.
func (ac *Chain) EnsureEOABalance(ctx context.Context, address common.Address, minimum *big.Int) error {
	if minimum == nil || minimum.Sign() < 0 {
		return fmt.Errorf("anvil ensure EOA balance: minimum must be non-nil and non-negative")
	}

	ac.fundingMu.Lock()
	defer ac.fundingMu.Unlock()

	return ac.WithEVMClient(func(client *evm.EVMClient) error {
		if err := client.RequireEOA(ctx, address); err != nil {
			return fmt.Errorf("anvil ensure EOA balance: %w", err)
		}
		current, err := client.Client().BalanceAt(ctx, address, nil)
		if err != nil {
			return fmt.Errorf("anvil query balance for %s: %w", address, err)
		}
		if current.Cmp(minimum) >= 0 {
			return nil
		}
		if setErr := client.RPCClient().CallContext(
			ctx,
			nil,
			"anvil_setBalance",
			address,
			hexutil.EncodeBig(minimum),
		); setErr != nil {
			return fmt.Errorf("anvil set balance for %s: %w", address, setErr)
		}
		got, err := client.Client().BalanceAt(ctx, address, nil)
		if err != nil {
			return fmt.Errorf("anvil verify balance for %s: %w", address, err)
		}
		if got.Cmp(minimum) < 0 {
			return fmt.Errorf("anvil verify balance for %s: got %s, want at least %s", address, got, minimum)
		}
		return nil
	})
}

func normalizeDockerSpec(spec Spec) (Spec, string, string, error) {
	if spec.Image == "" {
		spec.Image = DockerImage()
	}
	if spec.RunID == "" {
		spec.RunID = fmt.Sprintf("manual-%d-%d", os.Getpid(), time.Now().UnixNano())
	}
	statePath, err := filepath.Abs(spec.StatePath)
	if err != nil {
		return Spec{}, "", "", fmt.Errorf("absolute anvil state path (chain %s): %w", spec.ID, err)
	}
	stateDir := filepath.Dir(statePath)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return Spec{}, "", "", fmt.Errorf("create anvil state dir %s: %w", stateDir, err)
	}
	spec.StatePath = statePath
	containerPath := path.Join(containerStateDir, filepath.Base(statePath))
	return spec, containerPath, stateDir, nil
}

func containerName(spec Spec) string {
	return container.NamePrefix(spec.RunID, spec.ID) + "-anvil"
}

func waitReady(ctx context.Context, probe func(context.Context) error, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var lastErr error
	err := poll.Until(ctx, pollInterval, func(ctx context.Context) (bool, error) {
		lastErr = probe(ctx)
		return lastErr == nil, nil
	})
	if err != nil {
		return fmt.Errorf("not ready after %s: %w (last probe: %w)", timeout, err, lastErr)
	}
	return nil
}

func anvilStartupError(container testcontainers.Container, err error) error {
	ctx, cancel := context.WithTimeout(context.Background(), anvilLogTimeout)
	defer cancel()
	logs := dockerLogs(ctx, container)
	if logs == "" {
		return err
	}
	return fmt.Errorf("%w\npartial anvil logs:\n%s", err, evm.Tail(logs, anvilStartupTailBytes))
}

func cleanupFailedStart(container testcontainers.Container) error {
	ctx, cancel := context.WithTimeout(context.Background(), anvilStopTimeout)
	defer cancel()
	return container.Terminate(ctx, testcontainers.StopTimeout(anvilStopGrace))
}

func deriveStatePath(spec Spec) (string, error) {
	if spec.LogPath == "" {
		return "", fmt.Errorf(
			"anvil (chain %s): spec needs StatePath or LogPath (the --state file is derived from LogPath)",
			spec.ID,
		)
	}
	return strings.TrimSuffix(spec.LogPath, ".log") + ".state.json", nil
}

func (ac *Chain) CollectLogs(ctx context.Context) map[string]string {
	log := ac.snapshotLogs(ctx)
	if log == "" {
		return nil
	}
	return map[string]string{ac.ID(): log}
}

func (ac *Chain) Stop() error {
	ac.mu.Lock()
	if ac.closed && ac.container == nil {
		ac.mu.Unlock()
		return nil
	}
	if !ac.closed {
		ac.closeEVMClient()
		ac.closed = true
	}
	container := ac.container
	alreadyStopped := ac.stopped
	ac.mu.Unlock()

	var stopErr error
	if alreadyStopped {
		resumeCtx, cancel := context.WithTimeout(context.Background(), anvilStopTimeout)
		stopErr = resumeContainer(resumeCtx, container)
		cancel()
	}
	if stopErr == nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), anvilStopTimeout)
		stopErr = stopContainer(stopCtx, container)
		cancel()
	}
	ac.snapshotLogs(context.Background())
	rmCtx, cancel := context.WithTimeout(context.Background(), anvilStopTimeout)
	rmErr := removeContainer(rmCtx, container)
	cancel()
	if rmErr == nil {
		ac.mu.Lock()
		ac.stopped = true
		ac.container = nil
		ac.mu.Unlock()
		return nil
	}
	return errors.Join(stopErr, rmErr)
}

func (ac *Chain) snapshotLogs(ctx context.Context) string {
	logCtx, cancel := context.WithTimeout(ctx, anvilLogTimeout)
	defer cancel()

	log := dockerLogs(logCtx, ac.container)
	if log != "" && ac.logPath != "" {
		_ = os.WriteFile(ac.logPath, []byte(log), 0o644)
	}
	return log
}

func dockerLogs(ctx context.Context, container testcontainers.Container) string {
	if container == nil {
		return ""
	}
	reader, err := container.Logs(ctx)
	if err != nil {
		return ""
	}
	defer reader.Close() //nolint:errcheck
	data, err := io.ReadAll(reader)
	if err != nil {
		return ""
	}
	return string(data)
}

func stopContainer(ctx context.Context, container testcontainers.Container) error {
	if container == nil || !container.IsRunning() {
		return nil
	}
	grace := anvilStopGrace
	return container.Stop(ctx, &grace)
}

func removeContainer(ctx context.Context, container testcontainers.Container) error {
	if container == nil {
		return nil
	}
	return container.Terminate(ctx)
}

func pauseContainer(ctx context.Context, container testcontainers.Container) error {
	docker, err := client.New(client.FromEnv)
	if err != nil {
		return err
	}
	defer docker.Close() //nolint:errcheck
	_, err = docker.ContainerPause(ctx, container.GetContainerID(), client.ContainerPauseOptions{})
	return err
}

func resumeContainer(ctx context.Context, container testcontainers.Container) error {
	docker, err := client.New(client.FromEnv)
	if err != nil {
		return err
	}
	defer docker.Close() //nolint:errcheck
	_, err = docker.ContainerUnpause(ctx, container.GetContainerID(), client.ContainerUnpauseOptions{})
	return err
}
