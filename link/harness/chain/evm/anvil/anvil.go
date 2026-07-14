package anvil

import (
	"context"
	"errors"
	"fmt"
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
	"golang.org/x/sync/singleflight"

	"github.com/cosmos/ibc/link/harness/chain/evm"
	"github.com/cosmos/ibc/link/harness/internal/dockercli"
	"github.com/cosmos/ibc/link/harness/internal/poll"
	"github.com/cosmos/ibc/link/harness/internal/ports"

	chainpkg "github.com/cosmos/ibc/link/harness/chain"
)

const (
	DefaultDockerImage = "ghcr.io/foundry-rs/foundry:v1.7.1"
	DockerImageEnv     = "ANVIL_DOCKER_IMAGE"

	anvilGasLimit     = 50_000_000
	anvilReadyTimeout = 30 * time.Second
	// anvilStopGrace must leave time to dump the --state file that StartNode reloads.
	anvilStopGrace   = 5 * time.Second
	anvilStopTimeout = 15 * time.Second
	anvilPullTimeout = 10 * time.Minute
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
	// StatePath persists chain state across StopNode and StartNode; empty derives it from LogPath.
	StatePath string

	// BlockTime > 0 seals blocks via --block-time (whole seconds; rounded, rejected if it rounds
	// to zero); 0 keeps Anvil in instant/automine mode.
	BlockTime time.Duration
}

type Chain struct {
	client *evm.EVMClient
	evm.Identity

	logPath string

	spec Spec
	port int

	container string

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
	port, err := ports.FreePort()
	if err != nil {
		return nil, fmt.Errorf("allocate anvil port: %w", err)
	}
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

	container, ec, err := launchAnvil(ctx, spec, port, stateContainerPath, mountSpec)
	if err != nil {
		return nil, err
	}

	return &Chain{
		client:    ec,
		Identity:  evm.NewIdentity(spec.ID, fmt.Sprintf("http://127.0.0.1:%d", port)),
		logPath:   spec.LogPath,
		spec:      spec,
		port:      port,
		container: container,
	}, nil
}

func launchAnvil(
	ctx context.Context,
	spec Spec,
	port int,
	stateContainerPath, mountSpec string,
) (string, *evm.EVMClient, error) {
	rpcURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	container := containerName(spec)

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
			return "", nil, fmt.Errorf(
				"anvil (chain %s): block time %v is below the 1s granularity of --block-time",
				spec.ID,
				spec.BlockTime,
			)
		}
		args = append(args, "--block-time", strconv.FormatUint(secs, 10))
	}

	if err := ensureDockerImage(ctx, spec.Image); err != nil {
		return "", nil, fmt.Errorf("prepare anvil image %s: %w", spec.Image, err)
	}

	labels := dockercli.RunLabels(spec.RunID)
	_, _ = dockercli.Output(ctx, "rm", "-f", container)
	runArgs := []string{
		"run", "-d",
		"--name", container,
	}
	runArgs = append(runArgs, labels...)
	runArgs = append(runArgs,
		"-p", fmt.Sprintf("127.0.0.1:%d:8545", port),
		"-v", mountSpec,
		"--entrypoint", "anvil",
		spec.Image,
	)
	runArgs = append(runArgs, args...)
	if _, err := dockercli.Output(ctx, runArgs...); err != nil {
		startErr := fmt.Errorf("start anvil container (chain %s): %w", spec.ID, err)
		return "", nil, errors.Join(startErr, cleanupFailedStart(container, false))
	}

	ec, err := connectAnvil(ctx, spec, rpcURL)
	if err != nil {
		startupErr := anvilStartupError(container, err)
		return "", nil, errors.Join(startupErr, cleanupFailedStart(container, true))
	}
	return container, ec, nil
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
	return spec, containerPath, stateDir + ":" + containerStateDir, nil
}

var pullGroup singleflight.Group

func ensureDockerImage(ctx context.Context, image string) error {
	_, err, _ := pullGroup.Do(image, func() (any, error) {
		if _, inspectErr := dockercli.Output(ctx, "image", "inspect", image); inspectErr == nil {
			return nil, nil
		}
		pullCtx, cancel := context.WithTimeout(ctx, anvilPullTimeout)
		defer cancel()
		if _, pullErr := dockercli.Output(pullCtx, "pull", image); pullErr != nil {
			return nil, pullErr
		}
		return nil, nil
	})
	return err
}

func containerName(spec Spec) string {
	return dockercli.NamePrefix(spec.RunID, spec.ID) + "-anvil"
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

func anvilStartupError(container string, err error) error {
	ctx, cancel := context.WithTimeout(context.Background(), anvilLogTimeout)
	defer cancel()
	logs := dockerLogs(ctx, container)
	if logs == "" {
		return err
	}
	return fmt.Errorf("%w\npartial anvil logs:\n%s", err, evm.Tail(logs, anvilStartupTailBytes))
}

func cleanupFailedStart(container string, stop bool) error {
	var stopErr error
	if stop {
		ctx, cancel := context.WithTimeout(context.Background(), anvilStopTimeout)
		stopErr = stopContainer(ctx, container)
		cancel()
	}
	rmCtx, cancel := context.WithTimeout(context.Background(), anvilStopTimeout)
	defer cancel()
	return errors.Join(stopErr, removeContainer(rmCtx, container))
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
	if !ac.closed {
		ac.closeEVMClient()
		ac.closed = true
	}
	container := ac.container
	alreadyStopped := ac.stopped
	ac.mu.Unlock()

	var stopErr error
	if !alreadyStopped {
		stopCtx, cancel := context.WithTimeout(context.Background(), anvilStopTimeout)
		stopErr = stopContainer(stopCtx, container)
		cancel()
	}
	ac.snapshotLogs(context.Background())
	rmCtx, cancel := context.WithTimeout(context.Background(), anvilStopTimeout)
	rmErr := removeContainer(rmCtx, container)
	cancel()
	if stopErr == nil || rmErr == nil {
		ac.mu.Lock()
		ac.stopped = true
		ac.mu.Unlock()
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

func dockerLogs(ctx context.Context, container string) string {
	data, err := dockercli.Output(ctx, "logs", container)
	if err != nil || len(data) == 0 {
		return ""
	}
	return string(data)
}

func stopContainer(ctx context.Context, container string) error {
	if _, err := dockercli.Output(
		ctx,
		"stop",
		"-t",
		strconv.Itoa(int(anvilStopGrace.Seconds())),
		container,
	); err != nil &&
		!dockercli.MissingContainer(err) {
		return err
	}
	return nil
}

func removeContainer(ctx context.Context, container string) error {
	if _, err := dockercli.Output(ctx, "rm", "-f", container); err != nil && !dockercli.MissingContainer(err) {
		return err
	}
	return nil
}
