// SPDX-License-Identifier: Apache-2.0

package solidityibc

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/besu"
)

// The constructor's own rules (trusted state, trusting period, validator
// set) are left to the contract; validate covers only what it cannot see.
func TestBesuClientConfigValidation(t *testing.T) {
	valid := BesuClientConfig{
		Consensus: besu.IBFT2, ID: "cli-test", CounterpartyClientID: "cli-other",
		CounterpartyRouter: common.HexToAddress("0x02"),
	}
	require.NoError(t, valid.validate())

	for name, tc := range map[string]struct {
		mutate func(*BesuClientConfig)
		want   string
	}{
		"consensus":       {func(c *BesuClientConfig) { c.Consensus = "clique" }, "unknown besu consensus"},
		"client id":       {func(c *BesuClientConfig) { c.ID = "not valid!" }, "not a valid"},
		"counterparty id": {func(c *BesuClientConfig) { c.CounterpartyClientID = "" }, "empty counterparty"},
		"router":          {func(c *BesuClientConfig) { c.CounterpartyRouter = common.Address{} }, "zero counterparty router"},
	} {
		t.Run(name, func(t *testing.T) {
			config := valid
			tc.mutate(&config)
			require.ErrorContains(t, config.validate(), tc.want)
		})
	}
}
