// SPDX-License-Identifier: Apache-2.0

package main

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseIFTAmount(t *testing.T) {
	require.Equal(t, big.NewInt(1000000), parseIFTAmount("1000000"))
	require.Equal(t, big.NewInt(0), parseIFTAmount("0"))
	aboveUint64, _ := new(big.Int).SetString("100000000000000000000", 10)
	require.Equal(t, aboveUint64, parseIFTAmount("100000000000000000000"))
	require.Nil(t, parseIFTAmount("notanumber"))
	require.Nil(t, parseIFTAmount(""))
	require.Nil(t, parseIFTAmount("-1.5"))
	require.Nil(t, parseIFTAmount("-1"))
}
