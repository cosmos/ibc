package stub

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/cmd/configcmd"
	"github.com/cosmos/ibc/link/cmd/testappcmd"
)

const missingTestAppSignerAlias = "missing"

func TestDeploymentRetainsOnlyCompletedChainReceipts(t *testing.T) {
	got := deployment([]chainResult{
		{
			id: testChainA,
			deployment: testappcmd.ChainDeployment{
				Counter: "0x1", TxHash: testReceiptHash,
			},
		},
		{},
	})

	require.Equal(t, testReceiptHash, got.Chains[testChainA].TxHash)
	require.Equal(t, "0x1", got.Chains[testChainA].Counter)
	_, incomplete := got.Chains[""]
	require.False(t, incomplete)
}

func TestTestAppSignerKeysRequireEveryEVMChain(t *testing.T) {
	c := &configcmd.Config{Chains: []configcmd.Chain{{ID: "evm", Type: configcmd.ChainTypeEVM}}}

	_, err := testAppSignerKeys(c)
	require.ErrorContains(t, err, "chains[0].testAppSigner: test-app deployment signer alias is empty")
}

func TestTestAppSignerKeysIdentifyUnknownAlias(t *testing.T) {
	c := &configcmd.Config{Chains: []configcmd.Chain{{
		ID: "evm", Type: configcmd.ChainTypeEVM, TestAppSigner: missingTestAppSignerAlias,
	}}}

	_, err := testAppSignerKeys(c)
	require.ErrorContains(t, err, `chains[0].testAppSigner: test-app deployment signer "missing"`)
	require.ErrorContains(t, err, `signer alias "missing" is not configured`)
}
