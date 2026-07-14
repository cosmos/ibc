package evm

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestRequireEOARejectsZeroBeforeRPC(t *testing.T) {
	err := (&EVMClient{}).RequireEOA(t.Context(), common.Address{})
	require.ErrorContains(t, err, "EOA address is zero")
}
