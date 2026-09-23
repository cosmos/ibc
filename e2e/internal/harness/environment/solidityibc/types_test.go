// SPDX-License-Identifier: Apache-2.0

package solidityibc

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

// The constructor's own rules (trusted state, trusting period, validator
// set) are left to the contract; validate covers only what it cannot see.
func TestBesuQBFTClientConfigValidation(t *testing.T) {
	valid := BesuQBFTClientConfig{
		ID: "cli-test", CounterpartyClientID: "cli-other",
		CounterpartyRouter: common.HexToAddress("0x02"),
	}
	require.NoError(t, valid.validate())

	for name, tc := range map[string]struct {
		mutate func(*BesuQBFTClientConfig)
		want   string
	}{
		"client id":       {func(c *BesuQBFTClientConfig) { c.ID = "not valid!" }, "not a valid"},
		"counterparty id": {func(c *BesuQBFTClientConfig) { c.CounterpartyClientID = "" }, "empty counterparty"},
		"router":          {func(c *BesuQBFTClientConfig) { c.CounterpartyRouter = common.Address{} }, "zero counterparty router"},
	} {
		t.Run(name, func(t *testing.T) {
			config := valid
			tc.mutate(&config)
			require.ErrorContains(t, config.validate(), tc.want)
		})
	}
}
