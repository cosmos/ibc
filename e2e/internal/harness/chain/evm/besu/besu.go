// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	clibesu "github.com/cosmos/ibc/cli/besu"
	chainpkg "github.com/cosmos/ibc/e2e/internal/harness/chain"
	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm/container"
	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm/poll"
)

const (
	DefaultDockerImage = "hyperledger/besu:26.5.0"
	DockerImageEnv     = "BESU_DOCKER_IMAGE"

	besuEpochLength         = 30000
	besuReadyTimeout        = 90 * time.Second
	besuStopTimeout         = 30 * time.Second
	besuLogTimeout          = 10 * time.Second
	besuDiscardTimeout      = 10 * time.Second
	besuStartupLogTailBytes = 4096
	besuTreasuryBalanceHex  = "0xc9f2c9cd04674edea40000000"
	besuOperatorConfigFile  = "operatorConfigFile.json"
)

type Spec struct {
	ID        string
	ChainID   uint64
	Consensus clibesu.Consensus
	WorkDir   string
	RunID     string
	Image     string
	// BlockPeriod is rounded to whole seconds and rejected if it rounds to zero.
	BlockPeriod time.Duration
	// Nodes is how many nodes run the chain, one when zero. Only the first
	// validates from genesis; the others follow it and can be voted in.
	Nodes int
}

type Chain struct {
	*evm.EVMClient
	evm.Identity

	// nodes run the chain; the first validates from genesis and serves its RPC.
	nodes    []*node
	api      string
	logID    string
	wsURL    string
	treasury evm.Account
	wait     evm.TransactionWait

	mu      sync.Mutex
	stopped bool
	closed  bool

	fundingMu sync.Mutex
}

// node is one Besu container running the chain.
type node struct {
	container testcontainers.Container
	address   common.Address
	rpc       *rpc.Client
}

var (
	_ chainpkg.Chain          = (*Chain)(nil)
	_ chainpkg.EOAFunder      = (*Chain)(nil)
	_ chainpkg.ValidatorVoter = (*Chain)(nil)
)

func DockerImage() string {
	if image := os.Getenv(DockerImageEnv); image != "" {
		return image
	}
	return DefaultDockerImage
}

func Start(ctx context.Context, spec Spec) (result *Chain, err error) {
	if spec.ID == "" {
		return nil, errors.New("besu chain id is empty")
	}
	if spec.ChainID == 0 {
		return nil, fmt.Errorf("besu chain %s: EVM chain id is required", spec.ID)
	}
	if consensusErr := spec.Consensus.Validate(); consensusErr != nil {
		return nil, fmt.Errorf("besu chain %s: %w", spec.ID, consensusErr)
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
	if spec.Nodes < 0 {
		return nil, fmt.Errorf("besu chain %s: %d nodes", spec.ID, spec.Nodes)
	}
	spec.Nodes = max(spec.Nodes, 1)
	treasury, err := evm.NewAccount()
	if err != nil {
		return nil, fmt.Errorf("besu chain %s: create funding treasury: %w", spec.ID, err)
	}
	periodSecs := int(math.Round(spec.BlockPeriod.Seconds()))
	if periodSecs <= 0 {
		return nil, fmt.Errorf(
			"besu chain %s: block period %v is below the 1s granularity of BFT blockperiodseconds",
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

	configPath := filepath.Join(chainDir, besuOperatorConfigFile)
	writeErr := writeBesuOperatorConfig(configPath, spec.Consensus, spec.ChainID, periodSecs, treasury.Address())
	if writeErr != nil {
		return nil, writeErr
	}
	networkFilesDir := filepath.Join(chainDir, "networkFiles")
	if removeErr := os.RemoveAll(networkFilesDir); removeErr != nil {
		return nil, fmt.Errorf("remove stale besu network files: %w", removeErr)
	}

	namePrefix := container.NamePrefix(spec.RunID, spec.ID)
	labels := container.Labels(spec.RunID)
	hostUser := fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
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
				config.User = hostUser
			},
			HostConfigModifier: func(config *containertypes.HostConfig) {
				// The generator only writes files; disable publishing the image's exposed ports.
				config.NetworkMode = "none"
				config.Mounts = append(config.Mounts, mount.Mount{
					Type:   mount.TypeBind,
					Source: chainDir,
					Target: "/work",
				})
			},
			Cmd: []string{
				"operator", "generate-blockchain-config",
				"--config-file=" + besuOperatorConfigFile,
				"--to=networkFiles",
				"--private-key-file-name=key",
			},
		},
	})
	defer func() {
		if cleanupErr := container.Terminate(generator); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("remove besu config generator: %w", cleanupErr))
		}
	}()
	// Besu may exit non-zero after writing complete artifacts.
	dataDir, err := prepareBesuNodeDir(chainDir)
	if err != nil {
		if genErr != nil {
			return nil, errors.Join(
				fmt.Errorf("generate besu %s config: %w", spec.Consensus, genErr),
				fmt.Errorf("validate generated besu %s artifacts: %w", spec.Consensus, err),
			)
		}
		return nil, fmt.Errorf("validate generated besu %s artifacts: %w", spec.Consensus, err)
	}
	if generator != nil {
		if cleanupErr := container.Terminate(generator); cleanupErr != nil {
			return nil, fmt.Errorf("remove besu config generator: %w", cleanupErr)
		}
		generator = nil
	}

	bc := &Chain{
		api:      votingAPI(spec.Consensus),
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

	nodes := nodeLauncher{
		image:    spec.Image,
		labels:   labels,
		hostUser: hostUser,
		genesis:  filepath.Join(chainDir, "genesis.json"),
		api:      bc.api,
	}
	validator, err := nodes.start(ctx, namePrefix, dataDir, nil, "8545/tcp", "8546/tcp")
	if err != nil {
		return nil, err
	}
	bc.nodes = append(bc.nodes, validator)
	rpcURL, err := validator.container.PortEndpoint(ctx, "8545/tcp", "http")
	if err != nil {
		return nil, fmt.Errorf("resolve besu RPC endpoint: %w", err)
	}
	bc.Identity = evm.NewIdentity(spec.ID, rpcURL)

	bc.wsURL, err = validator.container.PortEndpoint(ctx, "8546/tcp", "ws")
	if err != nil {
		return nil, fmt.Errorf("resolve besu websocket endpoint: %w", err)
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

	if spec.Nodes > 1 {
		if followErr := bc.startFollowers(ctx, nodes, spec, chainDir, namePrefix); followErr != nil {
			return nil, besuStartupError(bc, followErr)
		}
	}
	return bc, nil
}

// votingAPI is the RPC namespace of the consensus's validator votes.
func votingAPI(consensus clibesu.Consensus) string {
	if consensus == clibesu.IBFT2 {
		return "ibft"
	}
	return "qbft"
}

// startFollowers starts the nodes after the first, peered with it over the
// Docker bridge, and waits until each has synced to its head.
func (bc *Chain) startFollowers(
	ctx context.Context,
	nodes nodeLauncher,
	spec Spec,
	chainDir, namePrefix string,
) error {
	validator := bc.nodes[0]
	ip, err := validator.container.ContainerIP(ctx)
	if err != nil || ip == "" {
		return errors.Join(fmt.Errorf("besu chain %s: no bridge IP for the validator node", spec.ID), err)
	}
	key, err := readNodeKey(filepath.Join(chainDir, "node", "data", "key"))
	if err != nil {
		return err
	}
	staticNodes, err := json.Marshal([]string{enode(key, ip)})
	if err != nil {
		return fmt.Errorf("marshal besu static nodes: %w", err)
	}

	for i := 1; i < spec.Nodes; i++ {
		dataDir := filepath.Join(chainDir, fmt.Sprintf("node-%d", i), "data")
		if mkdirErr := os.MkdirAll(dataDir, 0o777); mkdirErr != nil {
			return fmt.Errorf("create besu node data dir: %w", mkdirErr)
		}
		_ = os.Chmod(dataDir, 0o777)
		followerKey, keyErr := crypto.GenerateKey()
		if keyErr != nil {
			return fmt.Errorf("generate besu node key: %w", keyErr)
		}
		keyHex := hexutil.Encode(crypto.FromECDSA(followerKey))
		if writeErr := os.WriteFile(filepath.Join(dataDir, "key"), []byte(keyHex), 0o644); writeErr != nil {
			return fmt.Errorf("write besu node key: %w", writeErr)
		}
		if writeErr := os.WriteFile(filepath.Join(dataDir, "static-nodes.json"), staticNodes, 0o644); writeErr != nil {
			return fmt.Errorf("write besu static nodes: %w", writeErr)
		}

		follower, startErr := nodes.start(ctx, fmt.Sprintf("%s-node-%d", namePrefix, i), dataDir, []string{
			"--static-nodes-file=/var/lib/besu/static-nodes.json",
			"--sync-mode=FULL",
		}, "8545/tcp")
		if startErr != nil {
			return startErr
		}
		bc.nodes = append(bc.nodes, follower)
		if syncErr := bc.waitSynced(ctx, follower); syncErr != nil {
			return fmt.Errorf("besu chain %s node %d: %w", spec.ID, i, syncErr)
		}
	}
	return nil
}

// waitSynced waits until n has the validator node's head.
func (bc *Chain) waitSynced(ctx context.Context, n *node) error {
	head, err := bc.Height(ctx)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, besuReadyTimeout)
	defer cancel()
	client := ethclient.NewClient(n.rpc)
	return poll.Until(ctx, bc.wait.PollInterval, func(ctx context.Context) (bool, error) {
		height, err := client.BlockNumber(ctx)
		return err == nil && height >= head, nil
	})
}

// Nodes is the validator address of each node, the genesis validator first.
func (bc *Chain) Nodes() []common.Address {
	addresses := make([]common.Address, len(bc.nodes))
	for i, n := range bc.nodes {
		addresses[i] = n.address
	}
	return addresses
}

// Validators is the validator set at the head.
func (bc *Chain) Validators(ctx context.Context) ([]common.Address, error) {
	var validators []common.Address
	err := bc.nodes[0].rpc.CallContext(ctx, &validators, bc.api+"_getValidatorsByBlockNumber", "latest")
	if err != nil {
		return nil, fmt.Errorf("besu chain %s validators: %w", bc.logID, err)
	}
	return validators, nil
}

// VoteValidator has every node in the validator set vote validator in or out,
// waits until the head's set reflects it, then discards the votes. Only a node
// of this chain can be voted in: a validator with no node halts the chain.
// Votes are discarded on failure too, so none outlives the call.
func (bc *Chain) VoteValidator(ctx context.Context, validator common.Address, add bool) (err error) {
	if add && !slices.Contains(bc.Nodes(), validator) {
		return fmt.Errorf("besu chain %s: %s runs no node of the chain", bc.logID, validator)
	}
	current, err := bc.Validators(ctx)
	if err != nil {
		return err
	}
	var voters []*node
	for _, n := range bc.nodes {
		if slices.Contains(current, n.address) {
			voters = append(voters, n)
		}
	}

	var proposed []*node
	defer func() {
		err = errors.Join(err, bc.discardVotes(ctx, proposed, validator))
	}()
	for _, n := range voters {
		// A failed call may still have registered the vote.
		proposed = append(proposed, n)
		if voteErr := n.rpc.CallContext(ctx, nil, bc.api+"_proposeValidatorVote", validator, add); voteErr != nil {
			return fmt.Errorf("besu chain %s: %s votes on %s: %w", bc.logID, n.address, validator, voteErr)
		}
	}
	waitCtx, cancel := context.WithTimeout(ctx, bc.wait.Timeout)
	defer cancel()
	err = poll.Until(waitCtx, bc.wait.PollInterval, func(ctx context.Context) (bool, error) {
		current, err = bc.Validators(ctx)
		return err == nil && slices.Contains(current, validator) == add, nil
	})
	if err != nil {
		return fmt.Errorf(
			"besu chain %s: vote on %s did not pass, validators %v: %w", bc.logID, validator, current, err,
		)
	}
	return nil
}

// discardVotes has each of nodes discard its vote on validator, even once ctx
// is done.
func (bc *Chain) discardVotes(ctx context.Context, nodes []*node, validator common.Address) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), besuDiscardTimeout)
	defer cancel()
	var errs []error
	for _, n := range nodes {
		if err := n.rpc.CallContext(ctx, nil, bc.api+"_discardValidatorVote", validator); err != nil {
			errs = append(errs, fmt.Errorf(
				"besu chain %s: %s discards its vote on %s: %w", bc.logID, n.address, validator, err,
			))
		}
	}
	return errors.Join(errs...)
}

// nodeLauncher starts the nodes of one chain.
type nodeLauncher struct {
	image    string
	labels   map[string]string
	hostUser string
	genesis  string
	api      string
}

// start runs a node on the key in dataDir, publishing ports on loopback.
func (l nodeLauncher) start(
	ctx context.Context,
	name, dataDir string,
	args []string,
	ports ...string,
) (*node, error) {
	key, err := readNodeKey(filepath.Join(dataDir, "key"))
	if err != nil {
		return nil, err
	}
	apis := "ETH,NET,WEB3," + strings.ToUpper(l.api)
	request := testcontainers.ContainerRequest{
		Name:         name,
		Image:        l.image,
		Labels:       l.labels,
		ExposedPorts: ports,
		ConfigModifier: func(config *containertypes.Config) {
			config.User = l.hostUser
		},
		HostConfigModifier: func(config *containertypes.HostConfig) {
			container.BindPortsToLoopback(config, ports...)
			config.Mounts = append(
				config.Mounts,
				mount.Mount{
					Type:     mount.TypeBind,
					Source:   l.genesis,
					Target:   "/config/genesis.json",
					ReadOnly: true,
				},
				mount.Mount{Type: mount.TypeBind, Source: dataDir, Target: "/var/lib/besu"},
			)
		},
		Cmd: append([]string{
			"--data-path=/var/lib/besu",
			"--genesis-file=/config/genesis.json",
			"--min-gas-price=0",
			"--discovery-enabled=false",
			"--rpc-http-enabled",
			"--rpc-http-host=0.0.0.0",
			"--rpc-http-api=" + apis,
			"--rpc-ws-enabled",
			"--rpc-ws-host=0.0.0.0",
			"--rpc-ws-api=" + apis,
			"--host-allowlist=*",
		}, args...),
	}
	c, err := container.Start(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("start besu container %s: %w", name, err)
	}
	n := &node{container: c, address: crypto.PubkeyToAddress(key.PublicKey)}
	url, err := c.PortEndpoint(ctx, "8545/tcp", "http")
	if err == nil {
		n.rpc, err = rpc.DialContext(ctx, url)
	}
	if err != nil {
		return nil, errors.Join(fmt.Errorf("connect besu node %s: %w", name, err), container.Terminate(c))
	}
	return n, nil
}

func readNodeKey(path string) (*ecdsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read besu node key: %w", err)
	}
	key, err := crypto.HexToECDSA(strings.TrimPrefix(strings.TrimSpace(string(data)), "0x"))
	if err != nil {
		return nil, fmt.Errorf("parse besu node key %s: %w", path, err)
	}
	return key, nil
}

// enode is the address peers dial key's node at.
func enode(key *ecdsa.PrivateKey, ip string) string {
	return fmt.Sprintf("enode://%x@%s:30303", crypto.FromECDSAPub(&key.PublicKey)[1:], ip)
}

func (bc *Chain) WSURL() string { return bc.wsURL }

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

	logs := map[string]string{}
	for i, n := range bc.nodes {
		id := bc.logID
		if i > 0 {
			id = fmt.Sprintf("%s-node-%d", bc.logID, i)
		}
		if data := containerLogs(ctx, n.container); data != "" {
			logs[id] = data
		}
	}
	if len(logs) == 0 {
		return nil
	}
	return logs
}

func containerLogs(ctx context.Context, c testcontainers.Container) string {
	reader, err := c.Logs(ctx)
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

func besuStartupError(bc *Chain, err error) error {
	logs := besuStartupLogs(bc)
	if logs == "" {
		return err
	}
	return fmt.Errorf("%w\npartial besu logs:\n%s", err, logs)
}

func besuStartupLogs(bc *Chain) string {
	logs := bc.CollectLogs(context.Background())
	var out strings.Builder
	for _, id := range slices.Sorted(maps.Keys(logs)) {
		fmt.Fprintf(&out, "--- %s ---\n%s", id, evm.Tail(logs[id], besuStartupLogTailBytes))
	}
	return out.String()
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
	nodes := bc.nodes
	bc.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), besuStopTimeout)
	defer cancel()

	var err error
	for _, n := range nodes {
		n.rpc.Close()
		err = errors.Join(err, n.container.Terminate(ctx))
	}
	if err != nil {
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
	CancunTime        uint64         `json:"cancunTime"`
	ZeroBaseFee       bool           `json:"zeroBaseFee"`
	ContractSizeLimit int            `json:"contractSizeLimit"`
	QBFT              *besuBFTConfig `json:"qbft,omitempty"`
	IBFT2             *besuBFTConfig `json:"ibft2,omitempty"`
}

// besuBFTConfig is the genesis config QBFT and IBFT2 share.
type besuBFTConfig struct {
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

func writeBesuOperatorConfig(
	path string,
	consensus clibesu.Consensus,
	chainID uint64,
	blockPeriodSecs int,
	treasury common.Address,
) error {
	data, err := json.MarshalIndent(newBesuOperatorConfig(consensus, chainID, blockPeriodSecs, treasury), "", "  ")
	if err != nil {
		return fmt.Errorf("marshal besu %s config: %w", consensus, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write besu %s config %s: %w", consensus, path, err)
	}
	return nil
}

func newBesuOperatorConfig(
	consensus clibesu.Consensus,
	chainID uint64,
	blockPeriodSecs int,
	treasury common.Address,
) besuOperatorConfig {
	alloc := map[string]besuFund{
		besuAllocKey(treasury): {Balance: besuTreasuryBalanceHex},
	}
	bft := &besuBFTConfig{
		BlockPeriodSeconds: blockPeriodSecs,
		EpochLength:        besuEpochLength,
		// The round timeout must exceed one block period.
		RequestTimeoutSeconds: 2 * blockPeriodSecs,
	}
	config := besuOperatorConfig{
		Genesis: besuGenesis{
			Config: besuGenesisConfig{
				ChainID:           chainID,
				BerlinBlock:       0,
				LondonBlock:       0,
				ShanghaiTime:      0,
				CancunTime:        0,
				ZeroBaseFee:       true,
				ContractSizeLimit: 2147483647,
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
	if consensus == clibesu.IBFT2 {
		config.Genesis.Config.IBFT2 = bft
	} else {
		config.Genesis.Config.QBFT = bft
	}
	return config
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
