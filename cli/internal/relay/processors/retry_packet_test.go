// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/internal/store"
	"github.com/cosmos/ibc/cli/internal/tests/mocks"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

func TestRetryPacketChecksPendingSubmission(t *testing.T) {
	for _, operationType := range []v2.OperationType{v2.OperationTypeRecv, v2.OperationTypeAck, v2.OperationTypeTimeout} {
		for _, pending := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/pending=%t", operationType, pending), func(t *testing.T) {
				submitter := mocks.NewMockTxSubmitter(t)
				tr := &Transfer{Packet: store.Packet{
					SourceChainID: "1", PacketSourceClientID: "source", PacketSequenceNumber: 42,
				}}
				op := v2.OperationMetadata{
					SourceChainID:  "1",
					SourceClientID: "source",
					Sequence:       42,
					Type:           operationType,
				}
				submitter.EXPECT().HasPending(mock.Anything, op).Return(pending, nil).Once()
				_, err := NewRetryPacket(submitter, operationType).Process(t.Context(), tr)
				if pending {
					require.ErrorIs(t, err, ErrSubmissionPending)
				} else {
					require.NoError(t, err)
				}
			})
		}
	}
}
