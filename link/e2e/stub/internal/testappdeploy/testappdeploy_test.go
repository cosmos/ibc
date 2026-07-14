package testappdeploy

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

const missingTestAppSignerAlias = "missing"

func TestDeploymentRetainsOnlyCompletedChainReceipts(t *testing.T) {
	got := deployment([]chainResult{
		{
			id: "chain-a",
			deployment: wire.ChainTestAppDeployment{
				Counter: "0x1", TxHash: "0xreceipt",
			},
		},
		{},
	})

	require.Equal(t, "0xreceipt", got.Chains["chain-a"].TxHash)
	require.Equal(t, "0x1", got.Chains["chain-a"].Counter)
	_, incomplete := got.Chains[""]
	require.False(t, incomplete)
}

func TestTestAppSignerKeysRequireEveryEVMChain(t *testing.T) {
	c := &wire.ConfigYAML{Chains: []wire.Chain{{ID: "evm", Type: wire.ChainTypeEVM}}}

	_, err := testAppSignerKeys(c)
	require.ErrorContains(t, err, "chains[0].testAppSigner: test-app deployment signer alias is empty")
}

func TestTestAppSignerKeysIdentifyUnknownAlias(t *testing.T) {
	c := &wire.ConfigYAML{Chains: []wire.Chain{{
		ID: "evm", Type: wire.ChainTypeEVM, TestAppSigner: missingTestAppSignerAlias,
	}}}

	_, err := testAppSignerKeys(c)
	require.ErrorContains(t, err, `chains[0].testAppSigner: test-app deployment signer "missing"`)
	require.ErrorContains(t, err, `signer alias "missing" is not configured`)
}
