// SPDX-License-Identifier: Apache-2.0

package solidityibc

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestBesuQBFTClientConfigValidation(t *testing.T) {
	validator := common.HexToAddress("0x01")
	config := BesuQBFTClientConfig{
		ID: "cli-test", CounterpartyClientID: "cli-other",
		CounterpartyRouter: common.HexToAddress("0x02"),
		InitialHeight:      1, InitialTimestamp: 1, TrustingPeriod: 1,
	}
	for _, tc := range []struct {
		name       string
		validators []common.Address
	}{
		{"empty", nil},
		{"zero", []common.Address{{}}},
		{"duplicate", []common.Address{validator, validator}},
		{"one validator", []common.Address{validator}},
		{"distinct validators", []common.Address{validator, common.HexToAddress("0x03")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config.InitialValidators = tc.validators
			require.NoError(t, config.validate())
		})
	}
	config.InitialValidators = []common.Address{validator}
	config.TrustingPeriod = 0
	require.ErrorContains(t, config.validate(), "trusting period must be positive")
}
