package e2e_test

import (
	"context"
	"testing"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/environment/solidityibc/counter"
	"github.com/cosmos/ibc/e2e/internal/harness/environment/solidityibc/testerc20"
)

func TestProtocolStackDeployed(t *testing.T) {
	ctx := t.Context()
	env := e2etest.Start(t, e2etest.SelectedSuite(t))
	signers := e2etest.NewSigners(t)
	_, deployment := e2etest.Deploy(
		t,
		env,
		signers,
		e2etest.AtoB(e2etest.ChainA, e2etest.ChainB),
	)

	require.NotEmpty(t, env.Connections(), "suite must realize at least one IBC Connection")
	for _, connectionID := range env.Connections() {
		connection, err := env.Connection(connectionID)
		require.NoError(t, err)
		require.NotEmpty(t, connection.A().Locator())
		require.NotEmpty(t, connection.B().Locator())
		require.Equal(t, connection.B().Locator(), connection.A().CounterpartyLocator())
		require.Equal(t, connection.A().Locator(), connection.B().CounterpartyLocator())
	}

	for _, id := range []environment.ChainID{e2etest.ChainA, e2etest.ChainB} {
		apps, ok := deployment.Chain(id)
		require.True(t, ok, "deployment must include Chain %s", id)
		require.NotEqual(t, common.Address{}, apps.Token)
		require.NotEqual(t, common.Address{}, apps.Counter)
		require.NotEqual(t, common.Address{}, apps.ICS20Transfer)
		require.NotEqual(t, common.Address{}, apps.ICS27GMP)
		require.NotEqual(t, common.Address{}, apps.ICS26Router)

		chain, err := env.Chain(id)
		require.NoError(t, err)
		evmAccess, err := chain.EVM()
		require.NoError(t, err)

		assertPortsBound(ctx, t, string(id), evmAccess, apps)
		assertTokenMinted(ctx, t, string(id), evmAccess, apps.Token, signers.Addresses().Application)
		assertCounterFresh(ctx, t, string(id), evmAccess, apps.Counter)
	}
}

func assertPortsBound(
	ctx context.Context,
	t *testing.T,
	chainID string,
	client *environment.EVM,
	apps e2etest.ChainDeployment,
) {
	t.Helper()
	require.NoError(t, client.UseContractCaller(func(caller bind.ContractCaller) error {
		router, err := ics26router.NewContractCaller(apps.ICS26Router, caller)
		if err != nil {
			return err
		}
		transferApp, err := router.GetIBCApp(&bind.CallOpts{Context: ctx}, "transfer")
		require.NoError(t, err)
		require.Equal(t, apps.ICS20Transfer, transferApp, "transfer port on Chain %s", chainID)
		gmpApp, err := router.GetIBCApp(&bind.CallOpts{Context: ctx}, "gmpport")
		require.NoError(t, err)
		require.Equal(t, apps.ICS27GMP, gmpApp, "gmpport on Chain %s", chainID)
		return nil
	}))
}

func assertTokenMinted(
	ctx context.Context,
	t *testing.T,
	chainID string,
	client *environment.EVM,
	token common.Address,
	holder common.Address,
) {
	t.Helper()
	code, err := client.CodeAt(ctx, token, nil)
	require.NoError(t, err)
	require.NotEmpty(t, code, "TestERC20 bytecode on Chain %s", chainID)

	var tokenBalanceSign int
	err = client.UseContractCaller(func(caller bind.ContractCaller) error {
		bound, bindErr := testerc20.NewTestERC20Caller(token, caller)
		if bindErr != nil {
			return bindErr
		}
		got, callErr := bound.BalanceOf(&bind.CallOpts{Context: ctx}, holder)
		if callErr == nil {
			tokenBalanceSign = got.Sign()
		}
		return callErr
	})
	require.NoError(t, err, "TestERC20.balanceOf on Chain %s", chainID)
	require.Positive(t, tokenBalanceSign, "application signer must hold TestERC20 on Chain %s", chainID)
}

func assertCounterFresh(
	ctx context.Context,
	t *testing.T,
	chainID string,
	client *environment.EVM,
	counterAddr common.Address,
) {
	t.Helper()
	code, err := client.CodeAt(ctx, counterAddr, nil)
	require.NoError(t, err)
	require.NotEmpty(t, code, "Counter bytecode on Chain %s", chainID)

	var countSign int
	err = client.UseContractCaller(func(caller bind.ContractCaller) error {
		bound, bindErr := counter.NewCounterCaller(counterAddr, caller)
		if bindErr != nil {
			return bindErr
		}
		got, callErr := bound.Count(&bind.CallOpts{Context: ctx})
		if callErr == nil {
			countSign = got.Sign()
		}
		return callErr
	})
	require.NoError(t, err, "Counter.count on Chain %s", chainID)
	require.Zero(t, countSign, "fresh Counter must be 0 on Chain %s", chainID)
}
