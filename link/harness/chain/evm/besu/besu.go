package besu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/cosmos/ibc/link/harness/chain/evm"
	"github.com/cosmos/ibc/link/harness/internal/dockercli"
	"github.com/cosmos/ibc/link/harness/internal/poll"
	"github.com/cosmos/ibc/link/harness/internal/ports"

	chainpkg "github.com/cosmos/ibc/link/harness/chain"
)

const (
	// DefaultDockerImage is the pinned Besu Docker image used by e2e unless BESU_DOCKER_IMAGE overrides it.
	DefaultDockerImage = "hyperledger/besu:26.5.0"
	// DockerImageEnv is the environment variable that overrides DefaultDockerImage.
	DockerImageEnv = "BESU_DOCKER_IMAGE"

	besuEpochLength         = 30000
	besuReadyTimeout        = 90 * time.Second
	besuStopTimeout         = 30 * time.Second
	besuLogTimeout          = 10 * time.Second
	besuStartupLogTailBytes = 4096
)

// Spec configures one logical chain backed by a single-validator Besu QBFT node in Docker. One
// validator is deliberate: everything the harness (or the relayer) can observe — real QBFT sealing
// at the configured block period, instant finality, EIP-1559 JSON-RPC — is identical with more
// validators, because the RPC node is always this one and no capability exposes consensus-level
// faults. A multi-validator network would only add peering machinery and containers that nothing
// asserts against.
type Spec struct {
	ID      string
	ChainID uint64
	WorkDir string
	RunID   string
	Image   string
	// RelayerKeyHex is the hex secp256k1 relayer signing key (the wire chain's EVMSignerKey). Its derived
	// address is genesis-funded so the relayer can pay gas; Besu has no dev-account prealloc like Anvil.
	RelayerKeyHex string
	// BlockPeriod is the QBFT block period rendered into the genesis (blockperiodseconds). QBFT's
	// granularity is whole seconds, so it is rounded to the nearest second and rejected if that rounds to
	// zero. The harness passes the chain's resolved TimingProfile.BlockInterval, so the cadence the network
	// seals at and the cadence the waits are budgeted for are the same number by construction.
	BlockPeriod time.Duration
}

// Chain is a Docker-backed Besu QBFT chain managed by the harness.
type Chain struct {
	*evm.EVMClient
	evm.Identity

	container string // the validator container's Docker name
	logID     string // key the container's logs are filed under in diagnostics

	mu      sync.Mutex
	stopped bool
	closed  bool
}

var (
	_ chainpkg.Chain            = (*Chain)(nil)
	_ chainpkg.ReceiverProvider = (*Chain)(nil)
	_ evm.ClientProvider        = (*Chain)(nil)
)

// DockerImage returns the configured Besu image, defaulting to the pinned image used by e2e.
func DockerImage() string {
	if image := os.Getenv(DockerImageEnv); image != "" {
		return image
	}
	return DefaultDockerImage
}

// StartQBFT starts a single-validator Besu QBFT chain and returns its RPC endpoint.
func StartQBFT(ctx context.Context, spec Spec) (*Chain, error) {
	if spec.ID == "" {
		return nil, errors.New("besu chain id is empty")
	}
	if spec.ChainID == 0 {
		return nil, fmt.Errorf("besu chain %s: EVM chain id is required", spec.ID)
	}
	if spec.Image == "" {
		spec.Image = DockerImage()
	}
	if spec.RunID == "" {
		return nil, fmt.Errorf("besu chain %s: run id is empty", spec.ID)
	}
	if spec.WorkDir == "" {
		return nil, fmt.Errorf("besu chain %s: work dir is empty", spec.ID)
	}
	if spec.RelayerKeyHex == "" {
		return nil, fmt.Errorf("besu chain %s: relayer key is empty", spec.ID)
	}
	relayer, err := evm.AccountFromHex(spec.RelayerKeyHex)
	if err != nil {
		return nil, fmt.Errorf("besu chain %s: relayer key: %w", spec.ID, err)
	}
	periodSecs := int(math.Round(spec.BlockPeriod.Seconds()))
	if periodSecs <= 0 {
		return nil, fmt.Errorf(
			"besu chain %s: block period %v is below the 1s granularity of QBFT blockperiodseconds",
			spec.ID,
			spec.BlockPeriod,
		)
	}

	chainDir, err := filepath.Abs(spec.WorkDir)
	if err != nil {
		return nil, fmt.Errorf("absolute besu work dir: %w", err)
	}
	if mkdirErr := os.MkdirAll(chainDir, 0o777); mkdirErr != nil {
		return nil, fmt.Errorf("create besu work dir %s: %w", chainDir, mkdirErr)
	}
	_ = os.Chmod(chainDir, 0o777)

	configPath := filepath.Join(chainDir, "qbftConfigFile.json")
	if writeErr := writeBesuOperatorConfig(configPath, spec.ChainID, periodSecs, relayer.Addr); writeErr != nil {
		return nil, writeErr
	}
	networkFilesDir := filepath.Join(chainDir, "networkFiles")
	if removeErr := os.RemoveAll(networkFilesDir); removeErr != nil {
		return nil, fmt.Errorf("remove stale besu network files: %w", removeErr)
	}

	namePrefix := dockercli.NamePrefix(spec.RunID, spec.ID)
	labels := dockercli.RunLabels(spec.RunID)
	generatorName := namePrefix + "-generate"
	_, _ = dockercli.Output(ctx, "rm", "-f", generatorName)
	genArgs := []string{
		"run", "--rm",
		"--name", generatorName,
	}
	genArgs = append(genArgs, labels...)
	genArgs = append(genArgs,
		"-v", chainDir+":/work",
		"-w", "/work",
		spec.Image,
		"operator", "generate-blockchain-config",
		"--config-file=qbftConfigFile.json",
		"--to=networkFiles",
		"--private-key-file-name=key",
	)
	// Besu may exit non-zero after writing complete output; fresh, usable staged artifacts are authoritative.
	_, genErr := dockercli.Output(ctx, genArgs...)
	dataDir, err := prepareBesuNodeDir(chainDir)
	if err != nil {
		if genErr != nil {
			return nil, errors.Join(
				fmt.Errorf("generate besu QBFT config: %w", genErr),
				fmt.Errorf("validate generated besu QBFT artifacts: %w", err),
			)
		}
		return nil, fmt.Errorf("validate generated besu QBFT artifacts: %w", err)
	}

	port, err := ports.FreePort()
	if err != nil {
		return nil, fmt.Errorf("allocate besu rpc port: %w", err)
	}

	bc := &Chain{
		Identity:  evm.NewIdentity(spec.ID, fmt.Sprintf("http://127.0.0.1:%d", port)),
		container: namePrefix,
		logID:     spec.ID,
	}
	ok := false
	defer func() {
		if !ok {
			_ = bc.Stop()
		}
	}()

	runArgs := []string{
		"run", "-d",
		"--name", bc.container,
		"-p", fmt.Sprintf("127.0.0.1:%d:8545", port),
	}
	runArgs = append(runArgs, labels...)
	runArgs = append(runArgs,
		"-v", filepath.Join(chainDir, "genesis.json")+":/config/genesis.json:ro",
		"-v", dataDir+":/var/lib/besu",
		spec.Image,
		"--data-path=/var/lib/besu",
		"--genesis-file=/config/genesis.json",
		"--min-gas-price=0",
		"--rpc-http-enabled",
		"--rpc-http-host=0.0.0.0",
		"--rpc-http-api=ETH,NET,WEB3",
		"--host-allowlist=*",
	)
	if _, runErr := dockercli.Output(ctx, runArgs...); runErr != nil {
		return nil, fmt.Errorf("start besu container %s: %w", bc.container, runErr)
	}

	client, err := ethclient.DialContext(ctx, bc.RPCURL())
	if err != nil {
		return nil, fmt.Errorf("dial besu chain %s: %w", spec.ID, err)
	}
	if readyErr := waitBesuReady(ctx, client, spec); readyErr != nil {
		client.Close()
		return nil, besuStartupError(bc, readyErr)
	}
	ec, err := evm.NewConnectedClient(ctx, client)
	if err != nil {
		client.Close()
		return nil, besuStartupError(bc, fmt.Errorf("connect besu chain %s: %w", spec.ID, err))
	}

	bc.EVMClient = ec
	ok = true
	return bc, nil
}

// CollectLogs captures the current Docker logs for the Besu validator.
func (bc *Chain) CollectLogs(ctx context.Context) map[string]string {
	ctx, cancel := context.WithTimeout(ctx, besuLogTimeout)
	defer cancel()

	data, err := dockercli.Output(ctx, "logs", bc.container)
	if err != nil || len(data) == 0 {
		return nil
	}
	return map[string]string{bc.logID: string(data)}
}

func besuStartupError(bc *Chain, err error) error {
	logs := besuStartupLogs(bc)
	if logs == "" {
		return err
	}
	return fmt.Errorf("%w\npartial besu logs:\n%s", err, logs)
}

func besuStartupLogs(bc *Chain) string {
	data := bc.CollectLogs(context.Background())[bc.logID]
	if data == "" {
		return ""
	}
	return fmt.Sprintf("--- %s ---\n%s", bc.logID, evm.Tail(data, besuStartupLogTailBytes))
}

// Stop closes the RPC client and removes the Besu container.
func (bc *Chain) Stop() error {
	bc.mu.Lock()
	if bc.stopped {
		bc.mu.Unlock()
		return nil
	}
	if !bc.closed && bc.EVMClient != nil {
		bc.Close()
		bc.closed = true
	}
	container := bc.container
	bc.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), besuStopTimeout)
	defer cancel()

	if _, err := dockercli.Output(ctx, "rm", "-f", container); err != nil && !dockercli.Missing(err) {
		return err
	}
	bc.mu.Lock()
	bc.stopped = true
	bc.mu.Unlock()
	return nil
}

type besuOperatorConfig struct {
	Genesis    besuGenesis    `json:"genesis"`
	Blockchain besuBlockchain `json:"blockchain"`
}

type besuGenesis struct {
	Config     besuGenesisConfig   `json:"config"`
	Nonce      string              `json:"nonce"`
	Timestamp  string              `json:"timestamp"`
	GasLimit   string              `json:"gasLimit"`
	Difficulty string              `json:"difficulty"`
	MixHash    string              `json:"mixHash"`
	Coinbase   string              `json:"coinbase"`
	Alloc      map[string]besuFund `json:"alloc"`
}

type besuGenesisConfig struct {
	ChainID           uint64         `json:"chainId"`
	BerlinBlock       uint64         `json:"berlinBlock"`
	LondonBlock       uint64         `json:"londonBlock"`
	ShanghaiTime      uint64         `json:"shanghaiTime"`
	ZeroBaseFee       bool           `json:"zeroBaseFee"`
	ContractSizeLimit int            `json:"contractSizeLimit"`
	QBFT              besuQBFTConfig `json:"qbft"`
}

type besuQBFTConfig struct {
	BlockPeriodSeconds    int `json:"blockperiodseconds"`
	EpochLength           int `json:"epochlength"`
	RequestTimeoutSeconds int `json:"requesttimeoutseconds"`
}

type besuFund struct {
	Balance string `json:"balance"`
}

type besuBlockchain struct {
	Nodes besuNodes `json:"nodes"`
}

type besuNodes struct {
	Generate bool `json:"generate"`
	Count    int  `json:"count"`
}

func writeBesuOperatorConfig(path string, chainID uint64, blockPeriodSecs int, relayerAddr common.Address) error {
	data, err := json.MarshalIndent(newBesuOperatorConfig(chainID, blockPeriodSecs, relayerAddr), "", "  ")
	if err != nil {
		return fmt.Errorf("marshal besu QBFT config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write besu QBFT config %s: %w", path, err)
	}
	return nil
}

func newBesuOperatorConfig(chainID uint64, blockPeriodSecs int, relayerAddr common.Address) besuOperatorConfig {
	balance := "0x" + chainpkg.GenesisPrefund().Text(16)
	alloc := map[string]besuFund{
		besuAllocKey(evm.FaucetAccount().Addr): {Balance: balance},
		besuAllocKey(relayerAddr):              {Balance: balance},
	}
	return besuOperatorConfig{
		Genesis: besuGenesis{
			Config: besuGenesisConfig{
				ChainID:           chainID,
				BerlinBlock:       0,
				LondonBlock:       0,
				ShanghaiTime:      0,
				ZeroBaseFee:       true,
				ContractSizeLimit: 2147483647,
				QBFT: besuQBFTConfig{
					BlockPeriodSeconds: blockPeriodSecs,
					EpochLength:        besuEpochLength,
					// The QBFT round timeout must exceed one block period or the network cannot make
					// progress; 2x is besu's documented guidance and preserves the previous 2s/4s pairing.
					RequestTimeoutSeconds: 2 * blockPeriodSecs,
				},
			},
			Nonce:      "0x0",
			Timestamp:  "0x58ee40ba",
			GasLimit:   "0x1fffffffffffff",
			Difficulty: "0x1",
			MixHash:    "0x63746963616c2062797a616e74696e65206661756c7420746f6c6572616e6365",
			Coinbase:   "0x0000000000000000000000000000000000000000",
			Alloc:      alloc,
		},
		Blockchain: besuBlockchain{
			Nodes: besuNodes{Generate: true, Count: 1},
		},
	}
}

func besuAllocKey(addr common.Address) string {
	return strings.TrimPrefix(strings.ToLower(addr.Hex()), "0x")
}

// prepareBesuNodeDir stages the validator's runtime files from the operator generator's output: the
// genesis at the chain dir root (bind-mounted read-only into the container) and the generated
// validator key in the node's data dir. It returns the data dir to mount.
func prepareBesuNodeDir(chainDir string) (string, error) {
	genesisSrc := filepath.Join(chainDir, "networkFiles", "genesis.json")
	genesisDst := filepath.Join(chainDir, "genesis.json")
	if err := copyFile(genesisSrc, genesisDst, 0o644); err != nil {
		return "", fmt.Errorf("copy besu genesis: %w", err)
	}

	keyRoot := filepath.Join(chainDir, "networkFiles", "keys")
	entries, err := os.ReadDir(keyRoot)
	if err != nil {
		return "", fmt.Errorf("read besu validator keys: %w", err)
	}
	if len(entries) == 0 || !entries[0].IsDir() {
		return "", fmt.Errorf("besu generated no validator key directory under %s", keyRoot)
	}

	dataDir := filepath.Join(chainDir, "node", "data")
	if err := os.MkdirAll(dataDir, 0o777); err != nil {
		return "", fmt.Errorf("create besu node data dir: %w", err)
	}
	_ = os.Chmod(dataDir, 0o777)
	// 0644, not 0600: the key is bind-mounted into the besu image, which runs as uid 1000. On native
	// Linux the host file is owned by the host uid, so a 0600 key would be unreadable inside the
	// container and QBFT would never become ready; the key is a throwaway test validator key, so
	// world-readable is fine. (Docker Desktop on macOS masks this by remapping ownership.)
	keySrc := filepath.Join(keyRoot, entries[0].Name(), "key")
	if err := copyFile(keySrc, filepath.Join(dataDir, "key"), 0o644); err != nil {
		return "", fmt.Errorf("copy validator key: %w", err)
	}
	return dataDir, nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, mode)
}

func waitBesuReady(ctx context.Context, client *ethclient.Client, spec Spec) error {
	var startHeight uint64
	var sawHeight bool
	var lastErr error
	err := poll.Until(ctx, 250*time.Millisecond, besuReadyTimeout, func(ctx context.Context) (bool, error) {
		got, err := client.ChainID(ctx)
		if err != nil {
			lastErr = err
			return false, nil
		}
		if got.Uint64() != spec.ChainID {
			return false, fmt.Errorf("besu chain %s reports chain id %d, want %d", spec.ID, got.Uint64(), spec.ChainID)
		}
		height, err := client.BlockNumber(ctx)
		if err != nil {
			lastErr = err
			return false, nil
		}
		if !sawHeight {
			startHeight = height
			sawHeight = true
			return false, nil
		}
		return height > startHeight, nil
	})
	if err != nil {
		return fmt.Errorf("besu chain %s readiness: %w (last probe: %w)", spec.ID, err, lastErr)
	}
	return nil
}
