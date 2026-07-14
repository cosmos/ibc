// Package testappdeploy implements `ibc test-apps deploy` for the synthetic
// e2e applications. It does not deploy the real Solidity IBC protocol stack.
package testappdeploy

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/cosmos/ibc/e2e/internal/harness/ibclink/wire"
	"github.com/cosmos/ibc/e2e/internal/testapp/contracts"
	"github.com/cosmos/ibc/e2e/stub/internal/cfg"
	"github.com/cosmos/ibc/e2e/stub/internal/exitcode"
	"github.com/cosmos/ibc/e2e/stub/internal/onchain"
	"github.com/cosmos/ibc/e2e/stub/internal/signing"
	"github.com/cosmos/ibc/e2e/stub/internal/store"
)

const perChainTimeout = 60 * time.Second

var initialIFTSupply = mustBigInt("1000000000000000000000000")

func mustBigInt(s string) *big.Int {
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic(fmt.Sprintf("testappdeploy: invalid big.Int literal %q", s))
	}
	return v
}

func Command(flags *cfg.FlagSet) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "deploy",
		Short:        "deploy the synthetic MockIFT, MockGMP, and Counter test applications",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(cmd, flags)
		},
	}
	return cmd
}

type chainResult struct {
	id         string
	deployment wire.ChainTestAppDeployment
}

func run(cmd *cobra.Command, flags *cfg.FlagSet) error {
	c, setupErr := cfg.Setup(flags)
	if setupErr != nil {
		return setupErr
	}

	if err := cfg.RequireStore(c); err != nil {
		return exitcode.New(wire.ExitConfigInvalid, err)
	}
	signerKeys, err := testAppSignerKeys(c)
	if err != nil {
		return exitcode.New(wire.ExitConfigInvalid, err)
	}

	results := make([]chainResult, len(c.Chains))
	// Do not fail-fast after a sibling error: another Chain may already have
	// broadcast a durable deployment transaction and must be allowed to finish
	// so its receipt can be reported.
	var g errgroup.Group
	for i, ch := range c.Chains {
		g.Go(func() error {
			chainDeployment, deployErr := deployChain(cmd.Context(), ch, signerKeys[ch.ID])
			if deployErr != nil {
				return exitcode.New(
					wire.ExitTestAppDeployFailure,
					fmt.Errorf("deploy test apps to chain %s: %w", ch.ID, deployErr),
				)
			}
			results[i] = chainResult{id: ch.ID, deployment: chainDeployment}
			return nil
		})
	}
	if waitErr := g.Wait(); waitErr != nil {
		// Some chains may already contain durable deployments. Emit every receipt
		// before the coded failure so the driver can account for those effects.
		if printErr := cfg.PrintJSON(deployment(results)); printErr != nil {
			return exitcode.New(wire.ExitInternal, errors.Join(waitErr, printErr))
		}
		return waitErr
	}

	deployed := deployment(results)
	// Persist before stdout JSON so a write failure cannot leave silent address drift.
	if err := persist(cmd.Context(), c.DB.URL, deployed); err != nil {
		// Deployment is already durable even though persistence failed.
		if printErr := cfg.PrintJSON(deployed); printErr != nil {
			return exitcode.New(wire.ExitInternal, errors.Join(err, printErr))
		}
		return exitcode.New(wire.ExitInternal, err)
	}

	return cfg.PrintJSON(deployed)
}

func deployment(results []chainResult) wire.TestAppDeployment {
	deployed := wire.TestAppDeployment{Chains: map[string]wire.ChainTestAppDeployment{}}
	for _, result := range results {
		if result.id == "" {
			continue
		}
		deployed.Chains[result.id] = result.deployment
	}
	return deployed
}

func deployChain(
	ctx context.Context,
	ch wire.Chain,
	signerKey *ecdsa.PrivateKey,
) (wire.ChainTestAppDeployment, error) {
	ctx, cancel := context.WithTimeout(ctx, perChainTimeout)
	defer cancel()
	conn, err := onchain.Connect(ctx, ch.RPC.URL)
	if err != nil {
		return wire.ChainTestAppDeployment{}, err
	}
	client := conn.Client
	defer client.Close()

	opts, err := onchain.Transactor(ctx, signerKey, conn.ChainID)
	if err != nil {
		return wire.ChainTestAppDeployment{}, fmt.Errorf("build test-app deployment transactor: %w", err)
	}

	deployed, txHash, err := deployTestApps(ctx, opts, client)
	if err != nil {
		return wire.ChainTestAppDeployment{}, err
	}

	chainDeployment := wire.ChainTestAppDeployment{
		MockIFT: deployed.mockIFT.Hex(),
		MockGMP: deployed.mockGMP.Hex(),
		Counter: deployed.counter.Hex(),
		TxHash:  txHash,
	}
	return chainDeployment, nil
}

func testAppSignerKeys(c *wire.ConfigYAML) (map[string]*ecdsa.PrivateKey, error) {
	keys := make(map[string]*ecdsa.PrivateKey, len(c.Chains))
	for i, ch := range c.Chains {
		if ch.Type != wire.ChainTypeEVM {
			return nil, fmt.Errorf(
				"chains[%d].type: test-app deployment does not support chain type %q",
				i,
				ch.Type,
			)
		}
		path := fmt.Sprintf("chains[%d].testAppSigner", i)
		if ch.TestAppSigner == "" {
			return nil, fmt.Errorf("%s: test-app deployment signer alias is empty", path)
		}
		key, err := signing.LoadECDSA(c.Signers, ch.TestAppSigner)
		if err != nil {
			return nil, fmt.Errorf("%s: test-app deployment signer %q: %w", path, ch.TestAppSigner, err)
		}
		keys[ch.ID] = key
	}
	return keys, nil
}

type deployedTestApps struct {
	mockGMP common.Address
	mockIFT common.Address
	counter common.Address
}

func deployTestApps(
	ctx context.Context,
	opts *bind.TransactOpts,
	client *ethclient.Client,
) (deployedTestApps, string, error) {
	parsed, err := contracts.TestAppDeployer.ParsedABI()
	if err != nil {
		return deployedTestApps{}, "", err
	}
	deployer, tx, _, err := bind.DeployContract(
		opts,
		parsed,
		contracts.TestAppDeployer.Bytecode,
		client,
		initialIFTSupply,
	)
	if err != nil {
		return deployedTestApps{}, "", fmt.Errorf("deploy test apps: %w", err)
	}
	rcpt, err := onchain.WaitMined(ctx, client, tx)
	if err != nil {
		return deployedTestApps{}, "", err
	}
	if rcpt.Status != types.ReceiptStatusSuccessful {
		return deployedTestApps{}, "", fmt.Errorf("test app deployment reverted (tx %s)", tx.Hash().Hex())
	}

	topic := parsed.Events["TestAppsDeployed"].ID
	for _, lg := range rcpt.Logs {
		if lg.Address != deployer || len(lg.Topics) == 0 || lg.Topics[0] != topic {
			continue
		}
		var event struct {
			MockGMP common.Address `abi:"mockGMP"`
			MockIFT common.Address `abi:"mockIFT"`
			Counter common.Address `abi:"counter"`
		}
		if err := parsed.UnpackIntoInterface(&event, "TestAppsDeployed", lg.Data); err != nil {
			return deployedTestApps{}, "", fmt.Errorf("decode test app deployment event: %w", err)
		}
		return deployedTestApps{
			mockGMP: event.MockGMP,
			mockIFT: event.MockIFT,
			counter: event.Counter,
		}, tx.Hash().Hex(), nil
	}
	return deployedTestApps{}, "", fmt.Errorf(
		"test app deployment %s emitted no TestAppsDeployed event",
		tx.Hash().Hex(),
	)
}

func persist(ctx context.Context, dbPath string, deployment wire.TestAppDeployment) error {
	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer st.Close() //nolint:errcheck
	return st.SaveTestApps(ctx, deployment)
}
