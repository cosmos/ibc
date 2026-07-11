package anvil

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

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
	*evm.EVMClient
	evm.Identity

	logPath string

	spec Spec
	port int

	container string

	mu      sync.Mutex
	stopped bool
	closed  bool
}

var (
	_ chainpkg.Chain            = (*Chain)(nil)
	_ chainpkg.ReceiverProvider = (*Chain)(nil)
	_ evm.ClientProvider        = (*Chain)(nil)
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
		EVMClient: ec,
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
		_ = removeContainer(context.Background(), container)
		return "", nil, fmt.Errorf("start anvil container (chain %s): %w", spec.ID, err)
	}

	ec, err := connectAnvil(ctx, spec, rpcURL)
	if err != nil {
		startupErr := anvilStartupError(container, err)
		_ = stopContainer(context.Background(), container)
		_ = removeContainer(context.Background(), container)
		return "", nil, startupErr
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
	var lastErr error
	err := poll.Until(ctx, pollInterval, timeout, func(ctx context.Context) (bool, error) {
		lastErr = probe(ctx)
		return lastErr == nil, nil
	})
	if err != nil {
		return fmt.Errorf("not ready after %s: %w (last probe: %w)", timeout, err, lastErr)
	}
	return nil
}

func anvilStartupError(container string, err error) error {
	logs := dockerLogs(context.Background(), container)
	if logs == "" {
		return err
	}
	return fmt.Errorf("%w\npartial anvil logs:\n%s", err, evm.Tail(logs, anvilStartupTailBytes))
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
		ac.Close()
		ac.closed = true
	}
	container := ac.container
	alreadyStopped := ac.stopped
	ac.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), anvilStopTimeout)
	defer cancel()
	var err error
	if !alreadyStopped {
		err = stopContainer(ctx, container)
	}
	ac.snapshotLogs(ctx)
	if rmErr := removeContainer(ctx, container); err == nil {
		err = rmErr
	}
	return err
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
		!dockercli.Missing(err) {
		return err
	}
	return nil
}

func removeContainer(ctx context.Context, container string) error {
	if _, err := dockercli.Output(ctx, "rm", "-f", container); err != nil && !dockercli.Missing(err) {
		return err
	}
	return nil
}
