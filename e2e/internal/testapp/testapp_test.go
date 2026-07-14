package testapp

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidAmountOwnsInput(t *testing.T) {
	amount := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	owned, err := validAmount(amount)
	require.NoError(t, err)

	amount.SetInt64(3)
	require.NotEqual(t, amount, owned)
	require.Equal(t, 256, owned.BitLen())
}

func TestValidAmountRejectsNonUint256(t *testing.T) {
	tooLarge := new(big.Int).Lsh(big.NewInt(1), 256)
	for _, amount := range []*big.Int{nil, new(big.Int), big.NewInt(-1), tooLarge} {
		_, err := validAmount(amount)
		require.Error(t, err)
	}
}

func TestAddressRejectsInvalidInput(t *testing.T) {
	_, err := address("target", "not-an-address")
	require.ErrorContains(t, err, "not a valid EVM address")
}
