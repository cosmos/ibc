package stub

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

	"github.com/cosmos/ibc/link/cmd/configcmd"
	"github.com/cosmos/ibc/link/cmd/testappcmd"
	"github.com/cosmos/ibc/link/testappbindings"

	internalconfig "github.com/cosmos/ibc/link/internal/config"
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

// TestAppsDeploy returns the temporary synthetic test-app deployment handler.
func TestAppsDeploy(flags *internalconfig.FlagSet) testappcmd.Handler {
	return func(cmd *cobra.Command, _ []string) error {
		return runTestAppsDeploy(cmd, flags)
	}
}

type chainResult struct {
	id         string
	deployment testappcmd.ChainDeployment
}

func runTestAppsDeploy(cmd *cobra.Command, flags *internalconfig.FlagSet) error {
	c, setupErr := setupConfig(flags)
	if setupErr != nil {
		return setupErr
	}

	if err := requireStore(c); err != nil {
		return err
	}
	signerKeys, err := testAppSignerKeys(c)
	if err != nil {
		return err
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
				return fmt.Errorf("deploy test apps to chain %s: %w", ch.ID, deployErr)
			}
			results[i] = chainResult{id: ch.ID, deployment: chainDeployment}
			return nil
		})
	}
	if waitErr := g.Wait(); waitErr != nil {
		// Some chains may already contain durable deployments. Emit every receipt
		// before the coded failure so the driver can account for those effects.
		if printErr := printIndentedJSON(cmd.OutOrStdout(), deployment(results)); printErr != nil {
			return errors.Join(waitErr, printErr)
		}
		return waitErr
	}

	deployed := deployment(results)
	// Persist before stdout JSON so a write failure cannot leave silent address drift.
	if err := persist(cmd.Context(), c.DB.URL, deployed); err != nil {
		// Deployment is already durable even though persistence failed.
		if printErr := printIndentedJSON(cmd.OutOrStdout(), deployed); printErr != nil {
			return errors.Join(err, printErr)
		}
		return err
	}

	return printIndentedJSON(cmd.OutOrStdout(), deployed)
}

func deployment(results []chainResult) testappcmd.Deployment {
	deployed := testappcmd.Deployment{Chains: map[string]testappcmd.ChainDeployment{}}
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
	ch configcmd.Chain,
	signerKey *ecdsa.PrivateKey,
) (testappcmd.ChainDeployment, error) {
	ctx, cancel := context.WithTimeout(ctx, perChainTimeout)
	defer cancel()
	conn, err := connectChain(ctx, ch.RPC.URL)
	if err != nil {
		return testappcmd.ChainDeployment{}, err
	}
	client := conn.Client
	defer client.Close()

	opts, err := newTransactor(ctx, signerKey, conn.ChainID)
	if err != nil {
		return testappcmd.ChainDeployment{}, fmt.Errorf("build test-app deployment transactor: %w", err)
	}

	deployed, txHash, err := deployTestApps(ctx, opts, client)
	if err != nil {
		return testappcmd.ChainDeployment{}, err
	}

	chainDeployment := testappcmd.ChainDeployment{
		MockIFT: deployed.mockIFT.Hex(),
		MockGMP: deployed.mockGMP.Hex(),
		Counter: deployed.counter.Hex(),
		TxHash:  txHash,
	}
	return chainDeployment, nil
}

func testAppSignerKeys(c *configcmd.Config) (map[string]*ecdsa.PrivateKey, error) {
	keys := make(map[string]*ecdsa.PrivateKey, len(c.Chains))
	for i, ch := range c.Chains {
		if ch.Type != configcmd.ChainTypeEVM {
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
		key, err := loadECDSA(c.Signers, ch.TestAppSigner)
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
	deployer, tx, contract, err := testappbindings.DeployTestAppDeployer(
		opts,
		client,
		initialIFTSupply,
	)
	if err != nil {
		return deployedTestApps{}, "", fmt.Errorf("deploy test apps: %w", err)
	}
	rcpt, err := waitMined(ctx, client, tx)
	if err != nil {
		return deployedTestApps{}, "", err
	}
	if rcpt.Status != types.ReceiptStatusSuccessful {
		return deployedTestApps{}, "", fmt.Errorf("test app deployment reverted (tx %s)", tx.Hash().Hex())
	}

	parsed, err := testappbindings.TestAppDeployerMetaData.GetAbi()
	if err != nil {
		return deployedTestApps{}, "", fmt.Errorf("read test app deployer ABI: %w", err)
	}
	topic := parsed.Events["TestAppsDeployed"].ID
	for _, lg := range rcpt.Logs {
		if lg.Address != deployer || len(lg.Topics) == 0 || lg.Topics[0] != topic {
			continue
		}
		event, parseErr := contract.ParseTestAppsDeployed(*lg)
		if parseErr != nil {
			return deployedTestApps{}, "", fmt.Errorf("decode test app deployment event: %w", parseErr)
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

func persist(ctx context.Context, dbPath string, deployment testappcmd.Deployment) error {
	st, err := openStore(dbPath)
	if err != nil {
		return err
	}
	defer st.Close() //nolint:errcheck
	return st.SaveTestApps(ctx, deployment)
}
