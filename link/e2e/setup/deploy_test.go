package setup_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/e2e/e2etest"
	"github.com/cosmos/ibc/link/harness/chain/evm"
	"github.com/cosmos/ibc/link/harness/fixturekeys"
	"github.com/cosmos/ibc/link/harness/fixtures"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/topology"

	ethereum "github.com/ethereum/go-ethereum"
)

func TestDeploy_SelectedTopology_VerifiesFixturesOnChain(t *testing.T) {
	ctx := t.Context()
	h := e2etest.StartHarness(t, e2etest.SelectedTopology(t))
	link := h.IBCLink()

	a, err := h.Chains().EVM(topology.ChainA)
	require.NoError(t, err)
	b, err := h.Chains().EVM(topology.ChainB)
	require.NoError(t, err)

	require.NoError(t, link.MigrateUp(ctx))
	dep, err := link.Deploy(ctx)
	require.NoError(t, err)
	require.NotNil(t, dep)
	require.Len(t, dep.TxHashes, len(dep.Chains), "atomic fixture setup emits one transaction per EVM chain")

	for _, tc := range []struct {
		id     string
		client *evm.EVMClient
	}{{topology.ChainA, a}, {topology.ChainB, b}} {
		cd, ok := dep.Chain(tc.id)
		require.True(t, ok, "deploy must include chain %s", tc.id)

		assertFixturesOnChain(ctx, t, tc.id, tc.client, cd)
	}
}

func assertFixturesOnChain(
	ctx context.Context,
	t *testing.T,
	chainID string,
	c *evm.EVMClient,
	cd wire.ChainDeployment,
) {
	t.Helper()
	client := c.Client()

	for _, name := range []string{fixturekeys.MockIFT, fixturekeys.MockGMP, fixturekeys.Counter} {
		hexAddr, err := cd.Fixture(name)
		require.NoError(t, err, "%s fixture on %s", name, chainID)
		addr := common.HexToAddress(hexAddr)
		code, err := client.CodeAt(ctx, addr, nil)
		require.NoError(t, err, "CodeAt(%s) on chain %s", name, chainID)
		require.NotEmpty(t, code, "%s at %s must have bytecode on chain %s", name, addr, chainID)
	}

	counterABI, err := fixtures.Counter.ParsedABI()
	require.NoError(t, err)
	callData, err := counterABI.Pack("count")
	require.NoError(t, err)

	counterHex, err := cd.Fixture(fixturekeys.Counter)
	require.NoError(t, err)
	counterAddr := common.HexToAddress(counterHex)
	ret, err := client.CallContract(ctx, ethereum.CallMsg{To: &counterAddr, Data: callData}, nil)
	require.NoError(t, err, "call Counter.count() on chain %s", chainID)

	vals, err := counterABI.Unpack("count", ret)
	require.NoError(t, err)
	require.Len(t, vals, 1)
	count, ok := vals[0].(*big.Int)
	require.True(t, ok, "count() returns a uint256")
	require.Zero(t, count.Sign(), "freshly deployed Counter.count() must be 0 on chain %s", chainID)

	iftABI, err := fixtures.MockIFT.ParsedABI()
	require.NoError(t, err)
	faucetHex, err := cd.Fixture(fixturekeys.IFTFaucet)
	require.NoError(t, err)
	iftHex, err := cd.Fixture(fixturekeys.MockIFT)
	require.NoError(t, err)
	callData, err = iftABI.Pack("balanceOf", common.HexToAddress(faucetHex))
	require.NoError(t, err)
	iftAddr := common.HexToAddress(iftHex)
	ret, err = client.CallContract(ctx, ethereum.CallMsg{To: &iftAddr, Data: callData}, nil)
	require.NoError(t, err, "call MockIFT.balanceOf(faucet) on chain %s", chainID)
	vals, err = iftABI.Unpack("balanceOf", ret)
	require.NoError(t, err)
	require.Len(t, vals, 1)
	balance, ok := vals[0].(*big.Int)
	require.True(t, ok, "balanceOf returns a uint256")
	require.Positive(t, balance.Sign(), "fixture deployer must mint the faucet's initial IFT supply on %s", chainID)
}
