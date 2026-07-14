package besu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/moby/moby/api/types/mount"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/e2e/internal/harness/internal/containerutil"
	"github.com/cosmos/ibc/e2e/internal/harness/internal/poll"

	chainpkg "github.com/cosmos/ibc/e2e/internal/harness/chain"
	containertypes "github.com/moby/moby/api/types/container"
)

const (
	DefaultDockerImage = "hyperledger/besu:26.5.0"
	DockerImageEnv     = "BESU_DOCKER_IMAGE"

	besuEpochLength         = 30000
	besuReadyTimeout        = 90 * time.Second
	besuStopTimeout         = 30 * time.Second
	besuLogTimeout          = 10 * time.Second
	besuStartupLogTailBytes = 4096
	besuTreasuryBalanceHex  = "0xc9f2c9cd04674edea40000000"
)

type Spec struct {
	ID      string
	ChainID uint64
	WorkDir string
	RunID   string
	Image   string
	// BlockPeriod is rounded to whole seconds and rejected if it rounds to zero.
	BlockPeriod time.Duration
}

type Chain struct {
	*evm.EVMClient
	evm.Identity

	container testcontainers.Container
	logID     string
	treasury  evm.Account
	wait      evm.TransactionWait

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

func StartQBFT(ctx context.Context, spec Spec) (result *Chain, err error) {
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
	treasury, err := evm.NewAccount()
	if err != nil {
		return nil, fmt.Errorf("besu chain %s: create funding treasury: %w", spec.ID, err)
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
	if writeErr := writeBesuOperatorConfig(configPath, spec.ChainID, periodSecs, treasury.Address()); writeErr != nil {
		return nil, writeErr
	}
	networkFilesDir := filepath.Join(chainDir, "networkFiles")
	if removeErr := os.RemoveAll(networkFilesDir); removeErr != nil {
		return nil, fmt.Errorf("remove stale besu network files: %w", removeErr)
	}

	namePrefix := containerutil.NamePrefix(spec.RunID, spec.ID)
	labels := containerutil.Labels(spec.RunID)
	generatorName := namePrefix + "-generate"
	generator, genErr := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		Started: true,
		ContainerRequest: testcontainers.ContainerRequest{
			Name:       generatorName,
			Image:      spec.Image,
			Labels:     labels,
			WaitingFor: wait.ForExit().WithExitTimeout(besuReadyTimeout),
			ConfigModifier: func(config *containertypes.Config) {
				config.WorkingDir = "/work"
			},
			HostConfigModifier: func(config *containertypes.HostConfig) {
				config.Mounts = append(config.Mounts, mount.Mount{
					Type:   mount.TypeBind,
					Source: chainDir,
					Target: "/work",
				})
			},
			Cmd: []string{
				"operator", "generate-blockchain-config",
				"--config-file=qbftConfigFile.json",
				"--to=networkFiles",
				"--private-key-file-name=key",
			},
		},
	})
	defer func() {
		if generator != nil {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), besuStopTimeout)
			defer cancel()
			if cleanupErr := generator.Terminate(cleanupCtx); cleanupErr != nil {
				err = errors.Join(err, fmt.Errorf("remove besu config generator: %w", cleanupErr))
			}
		}
	}()
	// Besu may exit non-zero after writing complete artifacts.
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
	if generator != nil {
		cleanupCtx, cancel := context.WithTimeout(ctx, besuStopTimeout)
		cleanupErr := generator.Terminate(cleanupCtx)
		cancel()
		if cleanupErr != nil {
			return nil, fmt.Errorf("remove besu config generator: %w", cleanupErr)
		}
		generator = nil
	}

	bc := &Chain{
		logID:    spec.ID,
		treasury: treasury,
		wait: evm.TransactionWait{
			Timeout:      20 * time.Duration(periodSecs) * time.Second,
			PollInterval: 250 * time.Millisecond,
		},
	}
	defer func() {
		if result == nil {
			if stopErr := bc.Stop(); stopErr != nil {
				err = errors.Join(err, fmt.Errorf("clean up failed besu start: %w", stopErr))
			}
		}
	}()

	request := testcontainers.ContainerRequest{
		Name:         namePrefix,
		Image:        spec.Image,
		Labels:       labels,
		ExposedPorts: []string{"8545/tcp"},
		HostConfigModifier: func(config *containertypes.HostConfig) {
			containerutil.BindPortsToLoopback(config, "8545/tcp")
			config.Mounts = append(
				config.Mounts,
				mount.Mount{
					Type:     mount.TypeBind,
					Source:   filepath.Join(chainDir, "genesis.json"),
					Target:   "/config/genesis.json",
					ReadOnly: true,
				},
				mount.Mount{Type: mount.TypeBind, Source: dataDir, Target: "/var/lib/besu"},
			)
		},
		Cmd: []string{
			"--data-path=/var/lib/besu",
			"--genesis-file=/config/genesis.json",
			"--min-gas-price=0",
			"--rpc-http-enabled",
			"--rpc-http-host=0.0.0.0",
			"--rpc-http-api=ETH,NET,WEB3",
			"--host-allowlist=*",
		},
	}
	bc.container, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: request,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("start besu container %s: %w", namePrefix, err)
	}
	host, err := bc.container.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve besu host: %w", err)
	}
	port, err := bc.container.MappedPort(ctx, "8545/tcp")
	if err != nil {
		return nil, fmt.Errorf("resolve besu RPC port: %w", err)
	}
	bc.Identity = evm.NewIdentity(spec.ID, fmt.Sprintf("http://%s:%s", host, port.Port()))

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
	return bc, nil
}

// EnsureEOABalance transfers only the shortfall from the Chain's private
// treasury, then verifies the requested minimum on-chain.
func (bc *Chain) EnsureEOABalance(ctx context.Context, address common.Address, minimum *big.Int) error {
	if minimum == nil || minimum.Sign() < 0 {
		return fmt.Errorf("besu ensure EOA balance: minimum must be non-nil and non-negative")
	}
	if err := bc.RequireEOA(ctx, address); err != nil {
		return fmt.Errorf("besu ensure EOA balance: %w", err)
	}

	bc.fundingMu.Lock()
	defer bc.fundingMu.Unlock()

	current, err := bc.Client().BalanceAt(ctx, address, nil)
	if err != nil {
		return fmt.Errorf("besu query balance for %s: %w", address, err)
	}
	if current.Cmp(minimum) >= 0 {
		return nil
	}
	if address == bc.treasury.Address() {
		return fmt.Errorf(
			"besu treasury balance %s is below requested minimum %s",
			current,
			minimum,
		)
	}

	shortfall := new(big.Int).Sub(minimum, current)
	if _, sendErr := bc.BroadcastTx(ctx, bc.wait, bc.treasury, &address, nil, shortfall); sendErr != nil {
		return fmt.Errorf("besu fund %s with %s wei: %w", address, shortfall, sendErr)
	}
	got, err := bc.Client().BalanceAt(ctx, address, nil)
	if err != nil {
		return fmt.Errorf("besu verify balance for %s: %w", address, err)
	}
	if got.Cmp(minimum) < 0 {
		return fmt.Errorf("besu verify balance for %s: got %s, want at least %s", address, got, minimum)
	}
	return nil
}

func (bc *Chain) CollectLogs(ctx context.Context) map[string]string {
	ctx, cancel := context.WithTimeout(ctx, besuLogTimeout)
	defer cancel()

	if bc.container == nil {
		return nil
	}
	reader, err := bc.container.Logs(ctx)
	if err != nil {
		return nil
	}
	defer reader.Close() //nolint:errcheck
	data, err := io.ReadAll(reader)
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

	if container != nil {
		if err := container.Terminate(ctx); err != nil {
			return err
		}
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
	CancunTime        uint64         `json:"cancunTime"`
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

func writeBesuOperatorConfig(path string, chainID uint64, blockPeriodSecs int, treasury common.Address) error {
	data, err := json.MarshalIndent(newBesuOperatorConfig(chainID, blockPeriodSecs, treasury), "", "  ")
	if err != nil {
		return fmt.Errorf("marshal besu QBFT config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write besu QBFT config %s: %w", path, err)
	}
	return nil
}

func newBesuOperatorConfig(chainID uint64, blockPeriodSecs int, treasury common.Address) besuOperatorConfig {
	alloc := map[string]besuFund{
		besuAllocKey(treasury): {Balance: besuTreasuryBalanceHex},
	}
	return besuOperatorConfig{
		Genesis: besuGenesis{
			Config: besuGenesisConfig{
				ChainID:           chainID,
				BerlinBlock:       0,
				LondonBlock:       0,
				ShanghaiTime:      0,
				CancunTime:        0,
				ZeroBaseFee:       true,
				ContractSizeLimit: 2147483647,
				QBFT: besuQBFTConfig{
					BlockPeriodSeconds: blockPeriodSecs,
					EpochLength:        besuEpochLength,
					// QBFT requires the round timeout to exceed one block period.
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
	// The bind-mounted key must be readable by Besu's uid 1000 on native Linux.
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
	ctx, cancel := context.WithTimeout(ctx, besuReadyTimeout)
	defer cancel()

	var startHeight uint64
	var sawHeight bool
	var lastErr error
	err := poll.Until(ctx, 250*time.Millisecond, func(ctx context.Context) (bool, error) {
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
