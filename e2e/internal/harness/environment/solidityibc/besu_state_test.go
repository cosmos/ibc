// SPDX-License-Identifier: Apache-2.0

package solidityibc

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestValidateBesuClientState(t *testing.T) {
	valid := make([]byte, 5*32)
	valid[31] = 1   // router
	valid[95] = 7   // revision height
	valid[159] = 60 // clock drift; trusting period zero is valid
	require.NoError(t, validateBesuClientState(valid))

	for _, tc := range []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{"truncated", func(raw []byte) []byte { return raw[:len(raw)-1] }},
		{"trailing word", func(raw []byte) []byte { return append(raw, make([]byte, 32)...) }},
		{"nonzero revision", func(raw []byte) []byte { raw[63] = 1; return raw }},
		{"zero height", func(raw []byte) []byte { raw[95] = 0; return raw }},
		{"overflow height", func(raw []byte) []byte { raw[64] = 1; return raw }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, validateBesuClientState(tc.mutate(append([]byte(nil), valid...))))
		})
	}
}

func TestBesuQBFTClientConfigValidators(t *testing.T) {
	validator := common.HexToAddress("0x01")
	config := BesuQBFTClientConfig{
		ID: "cli-test", CounterpartyClientID: "cli-other",
		CounterpartyRouter: common.HexToAddress("0x02"),
		InitialHeight:      1, InitialTimestamp: 1,
	}
	for _, tc := range []struct {
		name       string
		validators []common.Address
		valid      bool
	}{
		{"empty", nil, false},
		{"zero", []common.Address{{}}, false},
		{"duplicate", []common.Address{validator, validator}, false},
		{"one validator", []common.Address{validator}, true},
		{"distinct validators", []common.Address{validator, common.HexToAddress("0x03")}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config.InitialValidators = tc.validators
			if tc.valid {
				require.NoError(t, config.validate())
			} else {
				require.Error(t, config.validate())
			}
		})
	}
}
