// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

type bootstrapQBFTAPI struct {
	requestedBlock string
	validators     []common.Address
	err            error
}

func (api *bootstrapQBFTAPI) GetValidatorsByBlockNumber(block string) ([]common.Address, error) {
	api.requestedBlock = block
	return api.validators, api.err
}

func TestBesuQBFTValidatorsUsesTrustedHeight(t *testing.T) {
	for _, tc := range []struct {
		name       string
		validators []common.Address
		err        error
		wantError  string
	}{
		{name: "validators", validators: []common.Address{common.HexToAddress("0x01")}},
		{name: "unknown block", wantError: "no Besu QBFT validators at height 42"},
		{name: "rpc error", err: errors.New("QBFT unavailable"), wantError: "QBFT unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &bootstrapQBFTAPI{validators: tc.validators, err: tc.err}
			server := rpc.NewServer()
			t.Cleanup(server.Stop)
			require.NoError(t, server.RegisterName("qbft", api))
			client := rpc.DialInProc(server)
			t.Cleanup(client.Close)
			validators, err := besuQBFTValidators(t.Context(), client, 42)
			require.Equal(t, "0x2a", api.requestedBlock)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.validators, validators)
			}
		})
	}
}
