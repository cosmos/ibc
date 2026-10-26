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
	"github.com/testcontainers/testcontainers-go/network"
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
	besuP2PPort             = 30303
	// besuJVMOptions trims each validator's heap and startup CPU, since a
	// two-chain environment boots eight Besu JVMs at once. C1-only
	// compilation and the serial collector nearly halve startup CPU; test
	// chains never need C2's peak throughput.
	besuJVMOptions = "-Xms128m -Xmx512m -XX:ActiveProcessorCount=2 -XX:TieredStopAtLevel=1 -XX:+UseSerialGC"
)

// besuBoots admits two chains to boot at once, one environment's pair: a
// booting Besu JVM is CPU-bound for tens of seconds, and parallel tests
// booting thirty-odd JVMs together starve nodes past besuReadyTimeout on a
// four-core host. A running chain costs little.
var besuBoots = make(chan struct{}, 2)

// validatorCount is the number of validators every managed Besu chain runs:
// the smallest BFT set that tolerates a faulty validator, so a header can
// reach quorum without every seal.
const validatorCount = 4

type Spec struct {
	ID        string
	ChainID   uint64
	Consensus clibesu.Consensus
	WorkDir   string
	RunID     string
	Image     string
	// BlockPeriod is rounded to whole seconds and rejected if it rounds to zero.
	BlockPeriod time.Duration
	// Followers is how many nodes beyond the validators run the chain. They
	// sync from genesis and can be voted in.
	Followers int
}

type Chain struct {
	*evm.EVMClient
	evm.Identity

	network *testcontainers.DockerNetwork
	// nodes run the chain: the validators, then the followers. The Chain's
	// RPC is the first validator's.
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
	key       *ecdsa.PrivateKey
	dataDir   string
	container testcontainers.Container
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
	if spec.Followers < 0 {
		return nil, fmt.Errorf("besu chain %s: %d followers", spec.ID, spec.Followers)
	}
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

	select {
	case besuBoots <- struct{}{}:
	case <-ctx.Done():
		return nil, fmt.Errorf("besu chain %s: wait to boot: %w", spec.ID, ctx.Err())
	}
	defer func() { <-besuBoots }()

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
	nodes, err := prepareBesuNodeDirs(chainDir, spec.Followers)
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
		nodes:    nodes,
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

	bc.network, err = network.New(ctx, network.WithLabels(labels))
	if err != nil {
		return nil, fmt.Errorf("create besu network: %w", err)
	}

	// The nodes start together: started one by one on a loaded host, the
	// first validators escalate through the doubling BFT round timeouts while
	// they wait for a quorum.
	startErrs := make([]error, len(nodes))
	var started sync.WaitGroup
	for i, n := range nodes {
		started.Go(func() {
			name := fmt.Sprintf("%s-%d", namePrefix, i)
			n.container, startErrs[i] = container.Start(ctx, besuNodeRequest(
				name, spec.Image, labels, bc.network.Name, hostUser, chainDir, n.dataDir, bc.api,
			))
			if startErrs[i] != nil {
				startErrs[i] = fmt.Errorf("start besu container %s: %w", name, startErrs[i])
			}
		})
	}
	started.Wait()
	if err = errors.Join(startErrs...); err != nil {
		return nil, err
	}
	if peerErr := peerNodes(ctx, nodes); peerErr != nil {
		return nil, besuStartupError(bc, fmt.Errorf("besu chain %s: %w", spec.ID, peerErr))
	}

	rpcNode := nodes[0].container
	rpcURL, err := rpcNode.PortEndpoint(ctx, "8545/tcp", "http")
	if err != nil {
		return nil, fmt.Errorf("resolve besu RPC endpoint: %w", err)
	}
	bc.Identity = evm.NewIdentity(spec.ID, rpcURL)

	bc.wsURL, err = rpcNode.PortEndpoint(ctx, "8546/tcp", "ws")
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
	return bc, nil
}

func (bc *Chain) WSURL() string { return bc.wsURL }

// ValidatorKeys returns the genesis validators' private keys.
func (bc *Chain) ValidatorKeys() []*ecdsa.PrivateKey {
	keys := make([]*ecdsa.PrivateKey, validatorCount)
	for i, n := range bc.nodes[:validatorCount] {
		keys[i] = n.key
	}
	return keys
}

// votingAPI is the RPC namespace of the consensus's validator votes.
func votingAPI(consensus clibesu.Consensus) string {
	if consensus == clibesu.IBFT2 {
		return "ibft"
	}
	return "qbft"
}

// Nodes is the validator address of each node, the genesis validators first.
func (bc *Chain) Nodes() []common.Address {
	addresses := make([]common.Address, len(bc.nodes))
	for i, n := range bc.nodes {
		addresses[i] = n.address()
	}
	return addresses
}

// Validators is the validator set at the head.
func (bc *Chain) Validators(ctx context.Context) ([]common.Address, error) {
	return bc.validatorsAt(ctx, bc.nodes[0])
}

// validatorsAt is the validator set at n's head.
func (bc *Chain) validatorsAt(ctx context.Context, n *node) ([]common.Address, error) {
	var validators []common.Address
	err := n.rpc.CallContext(ctx, &validators, bc.api+"_getValidatorsByBlockNumber", "latest")
	if err != nil {
		return nil, fmt.Errorf("besu chain %s: %s validators: %w", bc.logID, n.address(), err)
	}
	// Besu answers null when it has no header at its head number, and a BFT
	// chain never has an empty set.
	if len(validators) == 0 {
		return nil, fmt.Errorf("besu chain %s: %s reports no validators", bc.logID, n.address())
	}
	return validators, nil
}

// VoteValidator has every node in the validator set vote validator in or out,
// waits until every node's head reflects it, then discards the votes. Only a node
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
		if slices.Contains(current, n.address()) {
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
			return fmt.Errorf("besu chain %s: %s votes on %s: %w", bc.logID, n.address(), validator, voteErr)
		}
	}
	waitCtx, cancel := context.WithTimeout(ctx, bc.wait.Timeout)
	defer cancel()
	// Every node, not only the one serving RPC, must see the change before the
	// deferred discard cancels any vote not yet sealed.
	var observer *node
	var readErr error
	err = poll.Until(waitCtx, bc.wait.PollInterval, func(ctx context.Context) (bool, error) {
		for _, observer = range bc.nodes {
			current, readErr = bc.validatorsAt(ctx, observer)
			if readErr != nil || slices.Contains(current, validator) != add {
				return false, nil
			}
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf(
			"besu chain %s: vote on %s did not pass, %s has validators %v: %w",
			bc.logID, validator, observer.address(), current, errors.Join(err, readErr),
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
				"besu chain %s: %s discards its vote on %s: %w", bc.logID, n.address(), validator, err,
			))
		}
	}
	return errors.Join(errs...)
}

func (n *node) address() common.Address { return crypto.PubkeyToAddress(n.key.PublicKey) }

// enode is the address peers dial key's node at.
func enode(key *ecdsa.PrivateKey, ip string) string {
	return fmt.Sprintf("enode://%x@%s:%d", crypto.FromECDSAPub(&key.PublicKey)[1:], ip, besuP2PPort)
}

// peerNodes connects every pair of nodes once they all listen for peers,
// and keeps each node's RPC client. Each node dials only the nodes before
// it, so every pair has a single connection: with two-way dialing, Besu ends
// up with duplicate connections and its deduplication can close both,
// isolating a node long enough to drop its pending transactions. Static
// nodes would dial at startup, and Besu backs off for 30s or more after a
// peer that was not listening yet, by which time the BFT round timeouts have
// escalated.
func peerNodes(ctx context.Context, nodes []*node) error {
	ctx, cancel := context.WithTimeout(ctx, besuReadyTimeout)
	defer cancel()

	enodes := make([]string, len(nodes))
	for i, n := range nodes {
		url, err := n.container.PortEndpoint(ctx, "8545/tcp", "http")
		if err != nil {
			return fmt.Errorf("resolve node %d RPC endpoint: %w", i, err)
		}
		if n.rpc, err = rpc.DialContext(ctx, url); err != nil {
			return fmt.Errorf("dial node %d: %w", i, err)
		}
		ip, err := n.container.ContainerIP(ctx)
		if err != nil {
			return fmt.Errorf("resolve node %d address: %w", i, err)
		}
		enodes[i] = enode(n.key, ip)
	}
	for i, n := range nodes {
		var lastErr error
		err := poll.Until(ctx, 250*time.Millisecond, func(ctx context.Context) (bool, error) {
			var listening bool
			lastErr = n.rpc.CallContext(ctx, &listening, "net_listening")
			return lastErr == nil && listening, nil
		})
		if err != nil {
			return fmt.Errorf("node %d listening for peers: %w (last probe: %w)", i, err, lastErr)
		}
	}
	for i, n := range nodes {
		for j := range i {
			var added bool
			if err := n.rpc.CallContext(ctx, &added, "admin_addPeer", enodes[j]); err != nil {
				return fmt.Errorf("node %d add peer %d: %w", i, j, err)
			}
			if !added {
				return fmt.Errorf("node %d refused peer %d", i, j)
			}
		}
	}
	return nil
}

func besuNodeRequest(
	name, image string,
	labels map[string]string,
	networkName, hostUser, chainDir, dataDir, api string,
) testcontainers.ContainerRequest {
	apis := "ETH,NET,WEB3," + strings.ToUpper(api)
	return testcontainers.ContainerRequest{
		Name:  name,
		Image: image,
		// testcontainers writes into a request's labels, and nodes start concurrently.
		Labels:       maps.Clone(labels),
		Env:          map[string]string{"BESU_OPTS": besuJVMOptions},
		Networks:     []string{networkName},
		ExposedPorts: []string{"8545/tcp", "8546/tcp"},
		ConfigModifier: func(config *containertypes.Config) {
			config.User = hostUser
		},
		HostConfigModifier: func(config *containertypes.HostConfig) {
			container.BindPortsToLoopback(config, "8545/tcp", "8546/tcp")
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
			// Peers come only from peerNodes.
			"--discovery-enabled=false",
			"--nat-method=NONE",
			fmt.Sprintf("--p2p-port=%d", besuP2PPort),
			"--min-gas-price=0",
			"--rpc-http-enabled",
			"--rpc-http-host=0.0.0.0",
			"--rpc-http-api=ADMIN," + apis,
			"--rpc-ws-enabled",
			"--rpc-ws-host=0.0.0.0",
			"--rpc-ws-api=" + apis,
			"--host-allowlist=*",
		},
	}
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

	logs := make(map[string]string)
	for i, n := range bc.nodes {
		if n.container == nil {
			continue
		}
		if data := containerLogs(ctx, n.container); data != "" {
			logs[fmt.Sprintf("%s-%d", bc.logID, i)] = data
		}
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
	var b strings.Builder
	for _, id := range slices.Sorted(maps.Keys(logs)) {
		fmt.Fprintf(&b, "--- %s ---\n%s\n", id, evm.Tail(logs[id], besuStartupLogTailBytes))
	}
	return b.String()
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
	bc.mu.Unlock()

	// A container or network that fails to go is kept, so Stop retries it.
	errs := make([]error, len(bc.nodes))
	var terminated sync.WaitGroup
	for i, n := range bc.nodes {
		if n.rpc != nil {
			n.rpc.Close()
			n.rpc = nil
		}
		terminated.Go(func() {
			if errs[i] = container.Terminate(n.container); errs[i] == nil {
				n.container = nil
			}
		})
	}
	terminated.Wait()
	if err := errors.Join(errs...); err != nil {
		return err
	}
	if bc.network != nil {
		ctx, cancel := context.WithTimeout(context.Background(), besuStopTimeout)
		defer cancel()
		if removeErr := bc.network.Remove(ctx); removeErr != nil {
			return fmt.Errorf("remove besu network: %w", removeErr)
		}
		bc.network = nil
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
			Nodes: besuNodes{Generate: true, Count: validatorCount},
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

// prepareBesuNodeDirs stages a data dir holding the node key for every
// generated validator, then for followers more nodes on new keys.
func prepareBesuNodeDirs(chainDir string, followers int) ([]*node, error) {
	genesisSrc := filepath.Join(chainDir, "networkFiles", "genesis.json")
	genesisDst := filepath.Join(chainDir, "genesis.json")
	if err := copyFile(genesisSrc, genesisDst, 0o644); err != nil {
		return nil, fmt.Errorf("copy besu genesis: %w", err)
	}

	keyRoot := filepath.Join(chainDir, "networkFiles", "keys")
	entries, err := os.ReadDir(keyRoot)
	if err != nil {
		return nil, fmt.Errorf("read besu validator keys: %w", err)
	}
	if len(entries) != validatorCount {
		return nil, fmt.Errorf(
			"besu generated %d validator key dirs under %s, want %d",
			len(entries),
			keyRoot,
			validatorCount,
		)
	}

	keys := make([]*ecdsa.PrivateKey, 0, len(entries)+followers)
	for _, entry := range entries {
		if !entry.IsDir() {
			return nil, fmt.Errorf("besu validator key entry %s is not a directory", entry.Name())
		}
		keyData, err := os.ReadFile(filepath.Join(keyRoot, entry.Name(), "key"))
		if err != nil {
			return nil, fmt.Errorf("read validator key: %w", err)
		}
		key, err := crypto.HexToECDSA(strings.TrimPrefix(strings.TrimSpace(string(keyData)), "0x"))
		if err != nil {
			return nil, fmt.Errorf("parse validator key %s: %w", entry.Name(), err)
		}
		keys = append(keys, key)
	}
	for range followers {
		key, err := crypto.GenerateKey()
		if err != nil {
			return nil, fmt.Errorf("generate besu follower key: %w", err)
		}
		keys = append(keys, key)
	}

	nodes := make([]*node, len(keys))
	for i, key := range keys {
		dataDir := filepath.Join(chainDir, fmt.Sprintf("node-%d", i), "data")
		if err := os.MkdirAll(dataDir, 0o777); err != nil {
			return nil, fmt.Errorf("create besu node data dir: %w", err)
		}
		_ = os.Chmod(dataDir, 0o777)
		// The bind-mounted key must be readable by Besu's uid 1000 on native Linux.
		keyHex := []byte(hexutil.Encode(crypto.FromECDSA(key)))
		if err := os.WriteFile(filepath.Join(dataDir, "key"), keyHex, 0o644); err != nil {
			return nil, fmt.Errorf("write besu node key: %w", err)
		}
		nodes[i] = &node{key: key, dataDir: dataDir}
	}
	return nodes, nil
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
