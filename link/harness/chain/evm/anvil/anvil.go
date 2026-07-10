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
	// DefaultDockerImage is the pinned Anvil Docker image used by e2e unless ANVIL_DOCKER_IMAGE overrides it.
	DefaultDockerImage = "ghcr.io/foundry-rs/foundry:v1.7.1"
	// DockerImageEnv is the environment variable that overrides DefaultDockerImage.
	DockerImageEnv = "ANVIL_DOCKER_IMAGE"

	// anvilGasLimit is the 50M block gas limit passed to anvil at startup.
	anvilGasLimit = 50_000_000
	// anvilReadyTimeout bounds how long we poll eth_blockNumber for the node to come up.
	anvilReadyTimeout = 30 * time.Second
	// anvilStopGrace is how long docker stop waits after SIGTERM before SIGKILL. It MUST leave anvil time
	// to dump its --state file on graceful shutdown, which the FaultInjector restart (StartNode) reloads.
	anvilStopGrace   = 5 * time.Second
	anvilStopTimeout = 15 * time.Second
	anvilPullTimeout = 10 * time.Minute
	anvilLogTimeout  = 10 * time.Second

	containerStateDir     = "/anvil-state"
	anvilStartupTailBytes = 4096
	// pollInterval is the cadence for readiness and height waits.
	pollInterval = 50 * time.Millisecond
)

// Spec configures a node to start.
type Spec struct {
	ID      string // logical chain id used across the harness (for EVM, the numeric chain id as text)
	ChainID uint64 // EVM numeric chain id passed to --chain-id
	LogPath string // file that receives anvil docker logs (empty: discard)
	RunID   string // harness run id used in Docker names/labels
	Image   string // Docker image override (empty: DockerImage())
	// StatePath is the anvil --state file: state is loaded from it on start and dumped to it on a
	// graceful stop, so the FaultInjector can stop and restart the node WITHOUT losing the deployed
	// fixtures. Empty: derived from LogPath; a spec with neither must set StatePath explicitly.
	StatePath string

	// BlockTime is the fixed block cadence. 0 (the default) leaves anvil in instant/automine mode — a tx
	// mines the moment it arrives. A value > 0 renders --block-time <seconds> so anvil seals a block on that
	// interval instead, modeling a real chain's block time. Sub-second values are not expressible (anvil's
	// --block-time is whole seconds), so it is rounded to the nearest second and rejected if that rounds to
	// zero. Note: with a timer sealing blocks every interval, mining is no longer purely on-demand, so the
	// BlockController workflow of "pause, submit, observe the tx pending, then mine exactly one block" is not
	// reliable on such a node. ProvidesCapability withholds it below.
	BlockTime time.Duration
}

// Chain is a Docker-backed Anvil node managed by the harness. The embedded EVMClient is its EVM view
// (the evm.ClientProvider capability) plus the optional BlockController capability (see controls.go) and FaultInjector
// (see faults.go). StopNode/StartNode reuse the same container, port, and --state file, so a restarted node
// keeps its chain state and the daemon's existing RPC connection recovers.
type Chain struct {
	*evm.EVMClient
	evm.Identity

	logPath string

	// Restart parameters (set once at Start, reused by StartNode).
	spec Spec
	port int

	container string

	// mu guards the container + embedded EVMClient swap a StartNode performs, against Stop/Kill teardown.
	// Fault injection and normal chain ops on the same chain are single-threaded by test convention; mu
	// only protects the swap from a concurrent teardown.
	mu      sync.Mutex
	stopped bool // node currently stopped via StopNode (so teardown does not double-stop a dead container)
	closed  bool
}

var (
	_ chainpkg.Chain            = (*Chain)(nil)
	_ chainpkg.ReceiverProvider = (*Chain)(nil)
	_ evm.ClientProvider        = (*Chain)(nil)
)

// DockerImage returns the configured Anvil image, defaulting to the pinned image used by e2e.
func DockerImage() string {
	if image := os.Getenv(DockerImageEnv); image != "" {
		return image
	}
	return DefaultDockerImage
}

// Start launches anvil on a dynamically allocated free port, waits for semantic readiness
// (eth_blockNumber succeeds), and returns a connected chain. ctx governs startup only;
// the container stays alive until Stop is called, so a canceled caller context does not kill the node.
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

// launchAnvil starts one anvil container on the given port (with state persistence and the configured gas
// limit), waits for semantic readiness, and returns the container name plus a connected EVM client. On any
// failure past a successful docker run it tears down both the client and the container so nothing leaks.
func launchAnvil(
	ctx context.Context,
	spec Spec,
	port int,
	stateContainerPath, mountSpec string,
) (string, *evm.EVMClient, error) {
	rpcURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	container := containerName(spec)

	// No --silent: anvil's banner and per-block lines feed the diagnostics bundle. --state loads chain
	// state on start (if the file exists) and dumps it on a graceful stop, so a StopNode/StartNode cycle
	// preserves the deployed fixtures; a fresh start with a missing file is an empty genesis as usual.
	args := []string{
		"--port", "8545",
		"--host", "0.0.0.0",
		"--chain-id", strconv.FormatUint(spec.ChainID, 10),
		"--gas-limit", strconv.Itoa(anvilGasLimit),
		"--state", stateContainerPath,
	}
	// A non-zero BlockTime switches anvil from instant/automine to fixed-interval sealing. anvil's
	// --block-time is whole seconds, so round to the nearest and reject a positive value that rounds to 0
	// rather than silently launching in automine.
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
		// docker run can create the container and still error (ctx canceled mid-run, daemon hiccup); nothing
		// else owns it on this branch, so best-effort remove it by name, mirroring the connectAnvil failure
		// path below.
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
	// HTTP dial is lazy, so this never fails on a not-yet-bound port; readiness is the probe below, and the
	// same connection is reused for the wrapper to avoid a second dial.
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

// pullGroup dedupes concurrent ensureDockerImage calls for the same image: when two chains launch in
// parallel off the same anvil image, only one inspect+pull runs and both share its result.
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

// deriveStatePath places the anvil --state file beside the log file when the spec did not set one — a
// unique sibling path keeps two chains (and two runs) from clobbering each other's state dump, and it is
// cleaned up with the log's directory. A spec with neither StatePath nor LogPath is rejected: anvil
// re-dumps the state file on every graceful stop, so a location nothing owns would leak one per run.
func deriveStatePath(spec Spec) (string, error) {
	if spec.LogPath == "" {
		return "", fmt.Errorf(
			"anvil (chain %s): spec needs StatePath or LogPath (the --state file is derived from LogPath)",
			spec.ID,
		)
	}
	return strings.TrimSuffix(spec.LogPath, ".log") + ".state.json", nil
}

// CollectLogs snapshots the current Docker logs for diagnostics and mirrors them into LogPath.
func (ac *Chain) CollectLogs(ctx context.Context) map[string]string {
	log := ac.snapshotLogs(ctx)
	if log == "" {
		return nil
	}
	return map[string]string{ac.ID(): log}
}

// Stop closes the client, gracefully stops the node (SIGTERM via docker stop -t 5), snapshots logs, and
// removes the container. Safe after StopNode (the container is already stopped, so this removes it).
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
