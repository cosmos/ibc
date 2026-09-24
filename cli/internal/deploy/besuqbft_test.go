// SPDX-License-Identifier: Apache-2.0

package deploy

import (
	"context"
	"testing"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

// trustedTimestamp is the consensus-state timestamp besuSource serves.
const trustedTimestamp = 1788200000

type besuSource struct {
	Target
	head   uint64
	height uint64
}

func (s *besuSource) Head(context.Context) (uint64, uint64, error) {
	return s.head, 0, nil
}

func (s *besuSource) BesuQBFTConsensusState(
	_ context.Context,
	height uint64,
) (besumsgs.IBesuLightClientMsgsConsensusState, error) {
	s.height = height
	return besumsgs.IBesuLightClientMsgsConsensusState{Timestamp: trustedTimestamp}, nil
}

// hostChain is a chain without the client whose head sits at timestamp.
type hostChain struct {
	Target
	timestamp uint64
}

func (h *hostChain) Head(context.Context) (uint64, uint64, error) {
	return 0, h.timestamp, nil
}

func TestBesuQBFTBootstrapParams(t *testing.T) {
	router := common.HexToAddress("0x00000000000000000000000000000000000000bb")
	for _, tc := range []struct {
		name       string
		height     uint64
		head       uint64
		hostTime   uint64
		source     Target
		wantHeight uint64
		wantErr    string
	}{
		{name: "explicit height", height: 7, head: 20, hostTime: trustedTimestamp + 100, wantHeight: 7},
		{name: "head by default", head: 20, hostTime: trustedTimestamp + 100, wantHeight: 20},
		{name: "genesis head", hostTime: trustedTimestamp + 100, wantErr: "no block past genesis"},
		{
			name: "not a besu source", height: 7, hostTime: trustedTimestamp + 100,
			source: &hostChain{}, wantErr: "cannot serve a besu-qbft trusted state",
		},
		{name: "expired here", height: 7, hostTime: trustedTimestamp + 3600, wantErr: "already older than the trusting period"},
		{name: "one second from expiry", height: 7, hostTime: trustedTimestamp + 3599, wantHeight: 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := &besuSource{head: tc.head}
			var counterparty Target = source
			if tc.source != nil {
				counterparty = tc.source
			}
			params, err := BesuQBFTBootstrap{
				Host:           &hostChain{timestamp: tc.hostTime},
				Counterparty:   counterparty,
				IBCRouter:      router,
				Height:         tc.height,
				TrustingPeriod: 3600,
				MaxClockDrift:  60,
			}.Params(t.Context())
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantHeight, source.height)
			require.Equal(t, BesuQBFTParams{
				IBCRouter:             router,
				InitialHeight:         tc.wantHeight,
				InitialConsensusState: besumsgs.IBesuLightClientMsgsConsensusState{Timestamp: trustedTimestamp},
				TrustingPeriod:        3600,
				MaxClockDrift:         60,
			}, params)
		})
	}
}
