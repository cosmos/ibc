// Package sandboxd launches a real Cosmos SDK "sandboxd" node (cosmos-sdk v0.54, CometBFT v0.39, POA
// consensus, cosmos/evm with a full eth JSON-RPC) as a supervised subprocess, porting the upstream
// scripts/local-node.sh recipe into Go: init → fund genesis accounts → patch genesis + config in Go (no
// jq/sed) → validate → start with every listener on a dynamic port (or disabled) → wait for SEMANTIC
// readiness (CometBFT has committed a block AND the eth JSON-RPC tracks it). It knows nothing about the
// harness's chain abstraction; the EVM-family wrapper (harness/chain/evm/sandbox) dials the node's
// JSON-RPC and presents it as an EVM-family chain.Chain.
package sandboxd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/cosmos/ibc/link/harness/internal/ports"
	"github.com/cosmos/ibc/link/harness/internal/proc"

	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
)

const (
	// binEnv overrides the sandboxd binary location, mirroring ibclink's IBC_BIN: point it at any built
	// sandboxd to run the harness against that instead of the repo's `make bin/sandboxd` output.
	binEnv = "SANDBOXD_BIN"

	// Denom is the EVM + staking denom (18 decimals, 1:1 with wei).
	Denom = "astake"
	// DisplayDenom is the human unit for Denom.
	DisplayDenom = "stake"

	// Bech32HRP is the node's account-address prefix: 20 account bytes encode to cosmos1... under it.
	// Both facets (EVM genesis funding, the cosmos family) must use it, so it lives with the node.
	Bech32HRP = "cosmos"

	// logLevel is passed to `start`; info is quiet enough for the diagnostics bundle yet keeps the
	// consensus/EVM progress lines that explain a stuck node.
	logLevel = "info"
	// jsonRPCNamespaces are the eth JSON-RPC modules the stub and harness need (eth for tx/logs/receipts,
	// net/web3 for chain-id/liveness). Others stay off to keep the surface small.
	jsonRPCNamespaces = "eth,net,web3"

	// cmdTimeout bounds each one-shot setup subcommand (init / add-genesis-account / genesis validate).
	cmdTimeout = 60 * time.Second
	// readyTimeout bounds the semantic-readiness poll. The reference localnet reached a committed,
	// caught-up block in ~9s; 90s is generous headroom for a cold module cache or a busy machine, and is a
	// bounded condition poll, never a sleep.
	readyTimeout = 90 * time.Second
	// stopGrace is how long Stop waits after SIGTERM before SIGKILL. A Cosmos node flushes its LevelDB
	// stores on a graceful stop, so this is deliberately roomy — that clean shutdown is also what a future
	// FaultInjector "restart preserves state via the home dir" story would rely on.
	stopGrace = 10 * time.Second
)

// GenesisAccount is one bech32 account funded at genesis with the given coins string (e.g.
// "1000000000000000000000000000000astake").
type GenesisAccount struct {
	Address string
	Coins   string
}

// Spec configures one sandboxd node. Every field is required except LogPath (empty discards output).
type Spec struct {
	ID         string           // logical chain id, used in errors and the moniker
	ChainID    string           // cosmos chain-id passed to `init --chain-id`
	EVMChainID uint64           // numeric EVM chain id for --evm.evm-chain-id (must be unique across chains)
	HomeDir    string           // node home directory (must not already contain a chain; created by init)
	LogPath    string           // combined stdout+stderr capture (empty: discard)
	Admin      string           // bech32 POA admin + IFT authority (must be one of the funded accounts)
	Genesis    []GenesisAccount // genesis-funded accounts

	// EnableGRPC opts the node's cosmos gRPC query server in, on its own dynamic port (Node.GRPCURL). The
	// EVM facet leaves it off (it drives the node purely over eth JSON-RPC); the cosmos facet turns it on
	// because the stub's typed bank/auth queries (banktypes/authtypes QueryClient: account number+sequence
	// for signing, deploy's funded-balance checks) and the harness reader's balance reads go over gRPC. The
	// REST (LCD) API server stays off either way; queries go over typed gRPC.
	EnableGRPC bool
}

func (s Spec) validate() error {
	switch {
	case s.ID == "":
		return fmt.Errorf("sandboxd: spec ID is empty")
	case s.ChainID == "":
		return fmt.Errorf("sandboxd chain %s: cosmos chain-id is empty", s.ID)
	case s.EVMChainID == 0:
		return fmt.Errorf("sandboxd chain %s: EVM chain id is zero", s.ID)
	case s.HomeDir == "":
		return fmt.Errorf("sandboxd chain %s: home dir is empty", s.ID)
	case s.Admin == "":
		return fmt.Errorf("sandboxd chain %s: admin address is empty", s.ID)
	case len(s.Genesis) == 0:
		return fmt.Errorf("sandboxd chain %s: no genesis accounts", s.ID)
	}
	return nil
}

func (s Spec) moniker() string { return "sandbox-" + s.ID }

// Node is a started, semantically-ready sandboxd process. JSONRPCURL is the eth endpoint the EVM wrapper
// and ibc link dial; CometRPCURL is the CometBFT RPC used for the readiness probe (and available for
// diagnostics).
type Node struct {
	id          string
	proc        *proc.Process
	jsonRPCURL  string
	cometRPCURL string
	grpcURL     string // cosmos gRPC dial target (host:port); empty unless Spec.EnableGRPC
}

func (n *Node) JSONRPCURL() string  { return n.jsonRPCURL }
func (n *Node) CometRPCURL() string { return n.cometRPCURL }

// GRPCURL is the cosmos gRPC dial target (host:port, no scheme — the form grpc.NewClient takes), or "" when
// the gRPC server was not enabled (EVM facet).
func (n *Node) GRPCURL() string { return n.grpcURL }
func (n *Node) LogPath() string { return n.proc.LogPath() }

// Stop gracefully terminates the node (SIGTERM, then SIGKILL after stopGrace).
func (n *Node) Stop() error { return n.proc.Stop(stopGrace) }

// StartNode brings up one sandboxd node per the recipe and returns it once semantically ready. ctx governs
// startup only; the process outlives a canceled ctx and stops on Stop (matching the anvil/besu providers).
func StartNode(ctx context.Context, spec Spec) (*Node, error) {
	if err := spec.validate(); err != nil {
		return nil, err
	}
	bin := resolveBin()

	// 1. Scaffold the home dir, keys, and default genesis/config.
	if err := runCmd(ctx, bin, "init", spec.moniker(), "--home", spec.HomeDir, "--chain-id", spec.ChainID); err != nil {
		return nil, err
	}

	// 2. Fund the genesis accounts (no keyring import needed — add-genesis-account takes a literal address).
	for _, acct := range spec.Genesis {
		if err := runCmd(
			ctx,
			bin,
			"genesis",
			"add-genesis-account",
			acct.Address,
			acct.Coins,
			"--home",
			spec.HomeDir,
		); err != nil {
			return nil, err
		}
	}

	// 3. Read back the consensus pubkey `init` generated, then patch the genesis to the recipe.
	pubKey, pubKeyType, err := readValidatorPubKey(spec.HomeDir)
	if err != nil {
		return nil, err
	}
	genesisPath := filepath.Join(spec.HomeDir, "config", "genesis.json")
	if patchErr := patchGenesis(genesisPath, genesisPatch{
		Denom:               Denom,
		DisplayDenom:        DisplayDenom,
		Admin:               spec.Admin,
		Moniker:             spec.moniker(),
		ValidatorPubKeyB64:  pubKey,
		ValidatorPubKeyType: pubKeyType,
	}); patchErr != nil {
		return nil, patchErr
	}

	// 4. Fail fast on a malformed genesis before we pay for a start + readiness timeout.
	if validateErr := runCmd(ctx, bin, "genesis", "validate", "--home", spec.HomeDir); validateErr != nil {
		return nil, validateErr
	}

	// 5. Patch config.toml (fast blocks, pprof off) and app.toml (0 min gas price).
	configDir := filepath.Join(spec.HomeDir, "config")
	if patchErr := patchConfigTOML(filepath.Join(configDir, "config.toml")); patchErr != nil {
		return nil, patchErr
	}
	if patchErr := patchAppTOML(filepath.Join(spec.HomeDir, "config", "app.toml"), Denom); patchErr != nil {
		return nil, patchErr
	}

	// 6. Every listener gets a distinct dynamic port so multiple nodes coexist. Ports not needed are
	// disabled via flags (grpc/grpc-web/api) or config (pprof); the four always-on listeners bind below, plus
	// a fifth for the gRPC query server when Spec.EnableGRPC is set (the cosmos facet).
	nPorts := 4
	if spec.EnableGRPC {
		nPorts = 5
	}
	port, err := allocatePorts(nPorts)
	if err != nil {
		return nil, fmt.Errorf("sandboxd chain %s: %w", spec.ID, err)
	}
	cometRPCPort, p2pPort, jsonRPCPort, jsonRPCWSPort := port[0], port[1], port[2], port[3]

	cometRPCURL := fmt.Sprintf("http://127.0.0.1:%d", cometRPCPort)
	jsonRPCURL := fmt.Sprintf("http://127.0.0.1:%d", jsonRPCPort)

	args := []string{
		"start",
		"--home", spec.HomeDir,
		"--log_level", logLevel,
		"--rpc.laddr", fmt.Sprintf("tcp://127.0.0.1:%d", cometRPCPort),
		"--p2p.laddr", fmt.Sprintf("tcp://127.0.0.1:%d", p2pPort),
		"--grpc-web.enable=false",
		"--api.enable=false", // the REST (LCD) API server was retired; queries go over typed gRPC
		"--json-rpc.enable=true",
		"--json-rpc.address", fmt.Sprintf("127.0.0.1:%d", jsonRPCPort),
		"--json-rpc.ws-address", fmt.Sprintf("127.0.0.1:%d", jsonRPCWSPort),
		"--json-rpc.api", jsonRPCNamespaces,
		"--evm.evm-chain-id", strconv.FormatUint(spec.EVMChainID, 10),
	}
	var grpcURL string
	if spec.EnableGRPC {
		grpcPort := port[4]
		grpcURL = fmt.Sprintf("127.0.0.1:%d", grpcPort)
		// Both enabling AND the listen address are `start` flags (--grpc.enable / --grpc.address), so no
		// app.toml patch is needed — the dynamic port keeps coexisting cosmos nodes off the shared 9090.
		args = append(args, "--grpc.enable=true", "--grpc.address", grpcURL)
	} else {
		args = append(args, "--grpc.enable=false")
	}
	p, err := proc.Start(proc.Spec{Name: bin, Args: args, LogPath: spec.LogPath})
	if err != nil {
		return nil, fmt.Errorf("sandboxd chain %s: start: %w", spec.ID, err)
	}

	// 7. Semantic readiness: CometBFT has committed a block and finished catching up, AND the eth
	// JSON-RPC (which only comes up after the node is producing blocks) reports a matching height.
	client, err := ethclient.DialContext(ctx, jsonRPCURL)
	if err != nil {
		_ = p.Stop(stopGrace)
		return nil, fmt.Errorf("sandboxd chain %s: dial json-rpc: %w", spec.ID, err)
	}
	// The CometBFT RPC probe reuses one official rpc/client/http client across ticks (New only constructs —
	// it never dials until Start, which we never call — so the poll stays connection-cheap).
	cometClient, err := rpchttp.New(cometRPCURL, "/websocket")
	if err != nil {
		client.Close()
		_ = p.Stop(stopGrace)
		return nil, fmt.Errorf("sandboxd chain %s: build comet rpc client: %w", spec.ID, err)
	}
	// When the gRPC query server is opted in (cosmos facet), a persistent client conn folds a gRPC liveness
	// check into readiness so the node is never handed back before its typed bank/auth queries can be served.
	var grpcConn *grpc.ClientConn
	if grpcURL != "" {
		grpcConn, err = grpc.NewClient(grpcURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			client.Close()
			_ = p.Stop(stopGrace)
			return nil, fmt.Errorf("sandboxd chain %s: build grpc client: %w", spec.ID, err)
		}
	}
	probe := func(ctx context.Context) error {
		if err := cometReady(ctx, cometClient); err != nil {
			return err
		}
		h, err := client.BlockNumber(ctx)
		if err != nil {
			return fmt.Errorf("eth_blockNumber: %w", err)
		}
		if h < 1 {
			return fmt.Errorf("eth block height %d < 1", h)
		}
		if grpcConn != nil {
			if err := grpcReady(grpcConn); err != nil {
				return err
			}
		}
		return nil
	}
	if err := p.WaitReady(ctx, probe, readyTimeout); err != nil {
		client.Close()
		if grpcConn != nil {
			_ = grpcConn.Close()
		}
		_ = p.Stop(stopGrace)
		return nil, fmt.Errorf("sandboxd chain %s readiness: %w", spec.ID, err)
	}
	client.Close() // the wrapper dials its own client; readiness only needed a probe connection.
	if grpcConn != nil {
		_ = grpcConn.Close() // the harness/stub dial their own gRPC conns; readiness only needed a probe.
	}

	return &Node{id: spec.ID, proc: p, jsonRPCURL: jsonRPCURL, cometRPCURL: cometRPCURL, grpcURL: grpcURL}, nil
}

// grpcReady kicks the (lazy) client conn into connecting and reports nil only once it reaches the Ready
// state — the cosmos gRPC server registers its query services before it begins serving, so a Ready transport
// means the bank/auth queries are answerable. A non-Ready state is returned as an error so WaitReady keeps
// polling within its budget (this is a single-shot state read, never a blocking wait, so the poll cadence is
// preserved).
func grpcReady(conn *grpc.ClientConn) error {
	conn.Connect()
	if s := conn.GetState(); s != connectivity.Ready {
		return fmt.Errorf("grpc query server not ready (state %s)", s)
	}
	return nil
}

// readValidatorPubKey reads the base64 ed25519 consensus pubkey (and its proto type URL) from the
// priv_validator_key.json that `init` generated, so the genesis POA validator matches the key the node
// signs blocks with.
func readValidatorPubKey(homeDir string) (key, typeURL string, err error) {
	path := filepath.Join(homeDir, "config", "priv_validator_key.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("sandboxd: read %s: %w", path, err)
	}
	var pv struct {
		PubKey struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"pub_key"`
	}
	if err := json.Unmarshal(data, &pv); err != nil {
		return "", "", fmt.Errorf("sandboxd: parse %s: %w", path, err)
	}
	if pv.PubKey.Value == "" {
		return "", "", fmt.Errorf("sandboxd: %s has no pub_key.value", path)
	}
	// `init` always generates an ed25519 consensus key. Anything else means the tool changed under us; fail
	// loudly rather than guess a proto type that would silently produce a wrong genesis validator.
	if pv.PubKey.Type != "tendermint/PubKeyEd25519" {
		return "", "", fmt.Errorf(
			"sandboxd: %s consensus key type %q is not tendermint/PubKeyEd25519",
			path,
			pv.PubKey.Type,
		)
	}
	return pv.PubKey.Value, ed25519PubKeyType, nil
}

// cometReady returns nil once CometBFT's /status reports a committed block (height >= 1) and the node is
// no longer catching up. Any transport error or an unmet condition is returned so WaitReady keeps polling.
func cometReady(ctx context.Context, c *rpchttp.HTTP) error {
	status, err := c.Status(ctx)
	if err != nil {
		return fmt.Errorf("comet /status: %w", err)
	}
	if status.SyncInfo.LatestBlockHeight < 1 {
		return fmt.Errorf("comet height %d < 1", status.SyncInfo.LatestBlockHeight)
	}
	if status.SyncInfo.CatchingUp {
		return fmt.Errorf("comet still catching up")
	}
	return nil
}

// allocatePorts returns n distinct free TCP ports. It loops FreePort until it has n unique values so two
// listeners of the same node never collide (FreePort itself only guarantees a port is free, not distinct
// across calls).
func allocatePorts(n int) ([]int, error) {
	seen := map[int]bool{}
	out := make([]int, 0, n)
	for len(out) < n {
		p, err := ports.FreePort()
		if err != nil {
			return nil, fmt.Errorf("allocate port: %w", err)
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out, nil
}

// runCmd execs a one-shot sandboxd subcommand, bounded by cmdTimeout. On failure it includes the combined
// output (the benign "cosmos.poa.v1.Msg ... proto annotation" warning appears there on success and is
// harmless). Errors are prefixed with the harness convention.
func runCmd(ctx context.Context, bin string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, cmdTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("sandboxd %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Version returns a concise sandboxd version line for the diagnostics bundle (best-effort; "unknown" on
// any failure so version reporting never blocks or fails a run). The plain `go build` carries no ldflags,
// so the short `version` is empty — `version --long` still reports the cosmos-sdk and Go toolchain, which
// is the useful signal. Its stdout is clean YAML; the benign proto-annotation warning goes to stderr.
func Version(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, resolveBin(), "version", "--long").Output()
	if err != nil {
		return "unknown"
	}
	var sdk, goVer string
	for _, line := range strings.Split(string(out), "\n") {
		if v, ok := strings.CutPrefix(line, "cosmos_sdk_version:"); ok {
			sdk = strings.TrimSpace(v)
		}
		if v, ok := strings.CutPrefix(line, "go:"); ok {
			goVer = strings.TrimSpace(v)
		}
	}
	if sdk == "" {
		return "unknown"
	}
	if goVer != "" {
		return fmt.Sprintf("cosmos-sdk %s, %s", sdk, goVer)
	}
	return "cosmos-sdk " + sdk
}

// ResolvedBin reports which sandboxd binary the harness will exec (SANDBOXD_BIN if set, else the repo's
// bin/sandboxd), for the doctor/diagnostics surface.
func ResolvedBin() string { return resolveBin() }

// resolveBin picks the sandboxd binary: SANDBOXD_BIN if set, else the repo's built bin/sandboxd.
func resolveBin() string {
	if v := os.Getenv(binEnv); v != "" {
		return v
	}
	return defaultBinPath()
}

// defaultBinPath locates bin/sandboxd relative to THIS source file (via runtime.Caller) rather than the
// process working directory, so it resolves the same whether a test runs from e2e/setup or the module root.
func defaultBinPath() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join("bin", "sandboxd")
	}
	// file = <link>/harness/chain/sandboxd/sandboxd.go -> four parents up is the link repo root.
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(file))))
	return filepath.Join(root, "bin", "sandboxd")
}
