package cosmos

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	sdkbech32 "github.com/cosmos/cosmos-sdk/types/bech32"
)

// testKeyHex is an arbitrary 32-byte scalar used only to derive a syntactically valid address.
const testKeyHex = "0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"

func TestCanonicalAddressRoundTrips(t *testing.T) {
	addr, err := AccountAddressFromKeyHex(testKeyHex)
	require.NoError(t, err)

	got, err := CanonicalAddress(addr)
	require.NoError(t, err)
	require.Equal(t, addr, got)
}

func TestCanonicalAddressFoldsCase(t *testing.T) {
	addr, err := AccountAddressFromKeyHex(testKeyHex)
	require.NoError(t, err)

	// bech32 is case-insensitive as a whole-string encoding; the canonical form is lowercase.
	got, err := CanonicalAddress(strings.ToUpper(addr))
	require.NoError(t, err)
	require.Equal(t, addr, got)
}

func TestCanonicalAddressRejectsWrongPrefix(t *testing.T) {
	osmo, err := sdkbech32.ConvertAndEncode("osmo", make([]byte, 20))
	require.NoError(t, err)

	_, err = CanonicalAddress(osmo)
	require.Error(t, err)
	require.ErrorContains(t, err, `want "cosmos"`)
}

func TestCanonicalAddressRejectsGarbage(t *testing.T) {
	for _, s := range []string{"", "cosmos1", "not-an-address", "0x66aB6D9362d4F35596279692F0251Db635165871"} {
		_, err := CanonicalAddress(s)
		require.Error(t, err, "input %q", s)
	}
}
