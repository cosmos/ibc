package e2e_test

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/synthetic"
	"github.com/cosmos/ibc/link/cmd/testappcmd"

	bindings "github.com/cosmos/ibc/link/testappbindings"
)

func TestTestAppDeployment(t *testing.T) {
	ctx := t.Context()
	env := e2etest.Start(t, e2etest.SelectedSuite(t))
	signers := synthetic.NewSigners(t)
	_, deployment := synthetic.Deploy(
		t,
		env,
		signers,
		synthetic.AtoB(e2etest.ChainA, e2etest.ChainB),
	)

	chainA, err := env.Chain(e2etest.ChainA)
	require.NoError(t, err)
	a, err := chainA.EVM()
	require.NoError(t, err)

	chainB, err := env.Chain(e2etest.ChainB)
	require.NoError(t, err)
	b, err := chainB.EVM()
	require.NoError(t, err)

	for _, tc := range []struct {
		id     string
		client *environment.EVM
	}{{string(e2etest.ChainA), a}, {string(e2etest.ChainB), b}} {
		apps, ok := deployment.Chain(tc.id)
		require.True(t, ok, "test-app deployment must include Chain %s", tc.id)

		assertTestAppsOnChain(ctx, t, tc.id, tc.client, apps, signers.Addresses().Application)
	}
}

func assertTestAppsOnChain(
	ctx context.Context,
	t *testing.T,
	chainID string,
	client *environment.EVM,
	apps testappcmd.ChainDeployment,
	application common.Address,
) {
	t.Helper()
	require.NotEmpty(t, apps.TxHash, "test-app deployment transaction on Chain %s", chainID)
	for name, hexAddress := range map[string]string{
		"MockIFT": apps.MockIFT,
		"MockGMP": apps.MockGMP,
		"Counter": apps.Counter,
	} {
		require.True(t, common.IsHexAddress(hexAddress), "%s address on Chain %s", name, chainID)
		address := common.HexToAddress(hexAddress)
		code, err := client.CodeAt(ctx, address, nil)
		require.NoError(t, err, "CodeAt(%s) on Chain %s", name, chainID)
		require.NotEmpty(t, code, "%s at %s must have bytecode on Chain %s", name, address, chainID)
	}

	counterAddress := common.HexToAddress(apps.Counter)
	var countSign int
	err := client.UseContractCaller(func(caller bind.ContractCaller) error {
		counter, err := bindings.NewCounterCaller(counterAddress, caller)
		if err != nil {
			return err
		}
		count, err := counter.Count(&bind.CallOpts{Context: ctx})
		if err == nil {
			countSign = count.Sign()
		}
		return err
	})
	require.NoError(t, err, "call Counter.count() on Chain %s", chainID)
	require.Zero(t, countSign, "freshly deployed Counter.count() must be 0 on Chain %s", chainID)

	iftAddress := common.HexToAddress(apps.MockIFT)
	var balanceSign int
	err = client.UseContractCaller(func(caller bind.ContractCaller) error {
		ift, bindErr := bindings.NewMockIFTCaller(iftAddress, caller)
		if bindErr != nil {
			return bindErr
		}
		balance, callErr := ift.BalanceOf(&bind.CallOpts{Context: ctx}, application)
		if callErr == nil {
			balanceSign = balance.Sign()
		}
		return callErr
	})
	require.NoError(t, err, "call MockIFT.balanceOf(application signer) on Chain %s", chainID)
	require.Positive(
		t,
		balanceSign,
		"test-app deployer must mint the application signer's initial IFT supply on Chain %s",
		chainID,
	)
}
