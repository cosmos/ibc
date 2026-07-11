package reader

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/onchain"
)

// canonicalAddrReader builds a reader without a live client — CanonicalAddr is pure string handling.
func canonicalAddrReader() onchain.Reader {
	return New(nil, "chain-a", wire.ChainDeployment{}, onchain.Budget{})
}

func TestCanonicalAddrFoldsCasing(t *testing.T) {
	r := canonicalAddrReader()

	lower, err := r.CanonicalAddr("0x66ab6d9362d4f35596279692f0251db635165871")
	require.NoError(t, err)
	upper, err := r.CanonicalAddr("0x66AB6D9362D4F35596279692F0251DB635165871")
	require.NoError(t, err)
	require.Equal(t, lower, upper, "casing variants of one address share a canonical form")
	require.Equal(t, "0x66aB6D9362d4F35596279692F0251Db635165871", lower, "canonical form is EIP-55")
}

func TestCanonicalAddrRejectsMalformed(t *testing.T) {
	r := canonicalAddrReader()
	for _, s := range []string{"", "0x123", "not-an-address"} {
		_, err := r.CanonicalAddr(s)
		require.Error(t, err, "input %q", s)
		require.ErrorContains(t, err, "not a valid EVM address")
	}
}
