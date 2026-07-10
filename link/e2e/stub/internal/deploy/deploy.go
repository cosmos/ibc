// Package deploy implements `ibc deploy`: it dials each configured chain and atomically deploys the
// test-only fixtures (MockIFT, MockGMP, Counter) the harness asserts against, then emits the
// machine-readable wire.Deployment metadata (per-chain addresses + mock client ids).
//
// These fixtures are trivial mocks the stub compiles itself, not the real Eureka contract stack. The
// relayer is fully stubbed in this POC. Deployment is the real on-chain effect; what it deploys is a
// stand-in.
package deploy

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/cosmos/ibc/link/e2e/stub/internal/cfg"
	"github.com/cosmos/ibc/link/e2e/stub/internal/exitcode"
	"github.com/cosmos/ibc/link/e2e/stub/internal/onchain"
	"github.com/cosmos/ibc/link/e2e/stub/internal/rpcsafe"
	"github.com/cosmos/ibc/link/e2e/stub/internal/store"
	"github.com/cosmos/ibc/link/harness/fixturekeys"
	"github.com/cosmos/ibc/link/harness/fixtures"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/internal/config"
)

// perChainTimeout bounds dialing + deploying all fixtures on a single chain, so a dead RPC fails the
// deploy promptly instead of hanging.
const perChainTimeout = 60 * time.Second

// initialIFTSupply is minted to the faucet by FixtureDeployer so the harness's EVM IFT escrow
// (MockIFT.sendTransfer from the faucet) has tokens to escrow. 1e24 = 1,000,000 tokens at 18 decimals — far more than any POC test
// moves, so balance is never the cause of a failure. The literal is a build-time constant, so a parse
// failure is a programmer error (panic) rather than a swallowed one.
var initialIFTSupply = mustBigInt("1000000000000000000000000")

func mustBigInt(s string) *big.Int {
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic(fmt.Sprintf("deploy: invalid big.Int literal %q", s))
	}
	return v
}

// Command builds the `deploy` command.
func Command(flags *config.FlagSet) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "deploy",
		Short:        "deploy the test-only MockIFT/MockGMP/Counter fixtures to each configured chain",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(cmd, flags)
		},
	}
	return cmd
}

// chainResult carries one chain's deploy outcome from its goroutine back to the deterministic assembly
// step, keyed by chain id so the result never depends on completion order.
type chainResult struct {
	id         string
	deployment wire.ChainDeployment
	hashes     []string
}

func run(cmd *cobra.Command, flags *config.FlagSet) error {
	c, setupErr := cfg.Setup(flags)
	if setupErr != nil {
		return setupErr
	}

	if err := cfg.RequireStore(c); err != nil {
		return exitcode.New(wire.ExitConfigInvalid, err)
	}

	// Per-chain deploys are self-contained (own dial, own faucet nonce, no shared state), so run them
	// concurrently — ~one chain's deploy budget instead of the sum. Each goroutine writes its own result
	// slot (indexed by declaration order), so the assembled Deployment stays deterministic without a lock.
	results := make([]chainResult, len(c.Chains))
	g, gctx := errgroup.WithContext(cmd.Context())
	for i, ch := range c.Chains {
		g.Go(func() error {
			cd, hashes, deployErr := deployChain(gctx, ch)
			if deployErr != nil {
				// Redact any RPC URL a lower-layer transport error embedded: it may carry a resolved ${ENV}
				// secret, and this message reaches stderr. The error chain is terminal here (the process is
				// about to exit), so flattening it to a sanitized string loses nothing useful.
				return exitcode.New(
					wire.ExitDeployFailure,
					fmt.Errorf("deploy to chain %s: %s", ch.ID, rpcsafe.RedactURLs(deployErr.Error())),
				)
			}
			results[i] = chainResult{id: ch.ID, deployment: cd, hashes: hashes}
			return nil
		})
	}
	if waitErr := g.Wait(); waitErr != nil {
		return waitErr
	}

	dep := wire.Deployment{Chains: map[string]wire.ChainDeployment{}}
	for _, r := range results {
		dep.Chains[r.id] = r.deployment
		dep.TxHashes = append(dep.TxHashes, r.hashes...)
	}
	// Persist the deployment so the relay daemon can resolve fixture addresses
	// without re-deploying. It happens before emitting the metadata so a persistence failure surfaces as
	// a failed deploy rather than reported-but-not-saved.
	if err := persist(cmd.Context(), c.DB.URL, dep); err != nil {
		return exitcode.New(wire.ExitInternal, err)
	}

	return config.PrintJSON(dep)
}

// deployChain bounds and deploys one EVM chain.
func deployChain(ctx context.Context, ch wire.Chain) (wire.ChainDeployment, []string, error) {
	ctx, cancel := context.WithTimeout(ctx, perChainTimeout)
	defer cancel()
	return deployEVMChain(ctx, ch)
}

// deployEVMChain dials one EVM chain and atomically creates all fixtures plus the faucet's initial MockIFT
// balance, then returns their addresses, mock client id, and bootstrap transaction hash.
func deployEVMChain(ctx context.Context, ch wire.Chain) (wire.ChainDeployment, []string, error) {
	conn, err := onchain.Connect(ctx, ch.RPC.URL)
	if err != nil {
		return wire.ChainDeployment{}, nil, err
	}
	client := conn.Client
	defer client.Close()

	opts, err := conn.FaucetTransactor(ctx)
	if err != nil {
		return wire.ChainDeployment{}, nil, err
	}

	deployed, txHash, err := deployFixtures(ctx, opts, client)
	if err != nil {
		return wire.ChainDeployment{}, nil, err
	}

	cd := wire.ChainDeployment{
		Fixtures: map[string]string{
			fixturekeys.MockIFT: deployed.mockIFT.Hex(),
			fixturekeys.MockGMP: deployed.mockGMP.Hex(),
			fixturekeys.Counter: deployed.counter.Hex(),
			// The IFT source holder on an EVM chain is the dev faucet the IFT escrow debits (the account
			// minted the initial supply above), recorded so the harness reads the same holder's balance to
			// assert the source escrow — one family-agnostic fixture, no EVM assumption in the harness.
			fixturekeys.IFTFaucet: onchain.FaucetAddress().Hex(),
		},
		ClientID: "client-" + ch.ID, // mock id; the real ibc link binary assigns light-client ids per pairing
	}
	return cd, []string{txHash}, nil
}

type deployedFixtures struct {
	mockGMP common.Address
	mockIFT common.Address
	counter common.Address
}

// deployFixtures sends the single atomic fixture bootstrap and decodes its child addresses.
func deployFixtures(
	ctx context.Context,
	opts *bind.TransactOpts,
	client *ethclient.Client,
) (deployedFixtures, string, error) {
	parsed, err := fixtures.FixtureDeployer.ParsedABI()
	if err != nil {
		return deployedFixtures{}, "", err
	}
	deployer, tx, _, err := bind.DeployContract(
		opts,
		parsed,
		fixtures.FixtureDeployer.Bytecode,
		client,
		initialIFTSupply,
	)
	if err != nil {
		return deployedFixtures{}, "", fmt.Errorf("deploy fixtures: %w", err)
	}
	rcpt, err := onchain.WaitMined(ctx, client, tx)
	if err != nil {
		return deployedFixtures{}, "", err
	}
	if rcpt.Status != types.ReceiptStatusSuccessful {
		return deployedFixtures{}, "", fmt.Errorf("fixture deployment reverted (tx %s)", tx.Hash().Hex())
	}

	topic := parsed.Events["FixturesDeployed"].ID
	for _, lg := range rcpt.Logs {
		if lg.Address != deployer || len(lg.Topics) == 0 || lg.Topics[0] != topic {
			continue
		}
		var event struct {
			MockGMP common.Address `abi:"mockGMP"`
			MockIFT common.Address `abi:"mockIFT"`
			Counter common.Address `abi:"counter"`
		}
		if err := parsed.UnpackIntoInterface(&event, "FixturesDeployed", lg.Data); err != nil {
			return deployedFixtures{}, "", fmt.Errorf("decode fixture deployment: %w", err)
		}
		return deployedFixtures{
			mockGMP: event.MockGMP,
			mockIFT: event.MockIFT,
			counter: event.Counter,
		}, tx.Hash().Hex(), nil
	}
	return deployedFixtures{}, "", fmt.Errorf("fixture deployment tx %s emitted no FixturesDeployed", tx.Hash().Hex())
}

// persist saves the deployment to the relayer store.
func persist(ctx context.Context, dbPath string, dep wire.Deployment) error {
	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer st.Close() //nolint:errcheck
	return st.SaveDeployment(ctx, dep)
}
