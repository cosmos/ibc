package setup_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/e2e/e2etest"
	"github.com/cosmos/ibc/link/e2e/internal/synthetic"
	"github.com/cosmos/ibc/link/e2e/internal/testapp/contracts"
	"github.com/cosmos/ibc/link/harness/environment"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"

	ethereum "github.com/ethereum/go-ethereum"
)

func TestTestApps_SelectedSuite_VerifiesDeploymentOnChain(t *testing.T) {
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
	apps wire.ChainTestAppDeployment,
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

	counterABI, err := contracts.Counter.ParsedABI()
	require.NoError(t, err)
	callData, err := counterABI.Pack("count")
	require.NoError(t, err)

	counterAddress := common.HexToAddress(apps.Counter)
	result, err := client.CallContract(
		ctx,
		ethereum.CallMsg{To: &counterAddress, Data: callData},
		nil,
	)
	require.NoError(t, err, "call Counter.count() on Chain %s", chainID)

	values, err := counterABI.Unpack("count", result)
	require.NoError(t, err)
	require.Len(t, values, 1)
	count, ok := values[0].(*big.Int)
	require.True(t, ok, "count() returns a uint256")
	require.Zero(t, count.Sign(), "freshly deployed Counter.count() must be 0 on Chain %s", chainID)

	iftABI, err := contracts.MockIFT.ParsedABI()
	require.NoError(t, err)
	callData, err = iftABI.Pack("balanceOf", application)
	require.NoError(t, err)
	iftAddress := common.HexToAddress(apps.MockIFT)
	result, err = client.CallContract(
		ctx,
		ethereum.CallMsg{To: &iftAddress, Data: callData},
		nil,
	)
	require.NoError(t, err, "call MockIFT.balanceOf(application signer) on Chain %s", chainID)
	values, err = iftABI.Unpack("balanceOf", result)
	require.NoError(t, err)
	require.Len(t, values, 1)
	balance, ok := values[0].(*big.Int)
	require.True(t, ok, "balanceOf returns a uint256")
	require.Positive(
		t,
		balance.Sign(),
		"test-app deployer must mint the application signer's initial IFT supply on Chain %s",
		chainID,
	)
}
