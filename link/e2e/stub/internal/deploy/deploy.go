// Package deploy implements `ibc deploy` with test-only mock fixtures (not the real Eureka stack).
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

const perChainTimeout = 60 * time.Second

var initialIFTSupply = mustBigInt("1000000000000000000000000")

func mustBigInt(s string) *big.Int {
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic(fmt.Sprintf("deploy: invalid big.Int literal %q", s))
	}
	return v
}

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

	results := make([]chainResult, len(c.Chains))
	g, gctx := errgroup.WithContext(cmd.Context())
	for i, ch := range c.Chains {
		g.Go(func() error {
			cd, hashes, deployErr := deployChain(gctx, ch)
			if deployErr != nil {
				// Flatten dial errors so resolved ${ENV} credentials in RPC URLs never reach stderr.
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
	// Persist before stdout JSON so a write failure fails deploy, not silent drift.
	if err := persist(cmd.Context(), c.DB.URL, dep); err != nil {
		return exitcode.New(wire.ExitInternal, err)
	}

	return config.PrintJSON(dep)
}

func deployChain(ctx context.Context, ch wire.Chain) (wire.ChainDeployment, []string, error) {
	ctx, cancel := context.WithTimeout(ctx, perChainTimeout)
	defer cancel()
	return deployEVMChain(ctx, ch)
}

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
			fixturekeys.MockIFT:   deployed.mockIFT.Hex(),
			fixturekeys.MockGMP:   deployed.mockGMP.Hex(),
			fixturekeys.Counter:   deployed.counter.Hex(),
			fixturekeys.IFTFaucet: onchain.FaucetAddress().Hex(),
		},
		ClientID: "client-" + ch.ID,
	}
	return cd, []string{txHash}, nil
}

type deployedFixtures struct {
	mockGMP common.Address
	mockIFT common.Address
	counter common.Address
}

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

func persist(ctx context.Context, dbPath string, dep wire.Deployment) error {
	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer st.Close() //nolint:errcheck
	return st.SaveDeployment(ctx, dep)
}
