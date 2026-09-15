// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

type broadcastRPCError struct {
	code    int
	message string
}

func (e broadcastRPCError) Error() string  { return e.message }
func (e broadcastRPCError) ErrorCode() int { return e.code }

func TestRejectedBroadcast(t *testing.T) {
	for _, rpcErr := range []broadcastRPCError{
		{-32000, "exceeds block gas limit"},
		{-32000, "transaction underpriced"},
		{-32000, "insufficient funds for gas * price + value"},
		{-32000, "max priority fee per gas higher than max fee per gas"},
		{-32000, "intrinsic gas too low: gas 21000, minimum needed 24000"},
		{-32000, "transaction gas price below minimum: gas tip cap 1, minimum needed 7"},
		{-32000, "insufficient funds for gas * price + value: balance 0, tx cost 21000, overshot 21000"},
		{-32000, "insufficient funds for gas * price + value: balance 1, queued cost 2, tx cost 3, overshot 4"},
		{-32000, "insufficient funds for gas * price + value: balance 1, queued cost 2, tx bumped 3, overshot 4"},
		{-32000, "Wrong chainId"},
		{-32000, "ChainId not supported"},
		{-32000, "ChainId is required"},
		{-32000, "Transaction fee cap exceeded"},
		{-32000, "Max priority fee per gas exceeds max fee per gas"},
		{-32002, "Invalid signature"},
		{-32003, "Intrinsic gas exceeds gas limit"},
		{-32004, "Upfront cost exceeds account balance"},
		{-32005, "Transaction gas limit exceeds block gas limit"},
		{-32007, "Sender account not authorized to send transactions"},
		{-32009, "Gas price below configured minimum gas price"},
		{-32009, "Gas price below current base fee"},
	} {
		t.Run(rpcErr.Error(), func(t *testing.T) {
			require.True(t, isRejectedBroadcast(rpcErr))
			require.True(t, isRejectedBroadcast(fmt.Errorf("wrapped: %w", rpcErr)))
			require.False(t, isRejectedBroadcast(broadcastRPCError{-32603, rpcErr.message}))
			require.False(t, isRejectedBroadcast(broadcastRPCError{rpcErr.code, "proxy: " + rpcErr.message}))
			require.False(t, isRejectedBroadcast(broadcastRPCError{rpcErr.code, rpcErr.message + "; timed out"}))
		})
	}
}

func TestAmbiguousBroadcast(t *testing.T) {
	for _, err := range []error{
		context.Canceled,
		context.DeadlineExceeded,
		io.ErrUnexpectedEOF,
		fmt.Errorf("Upfront cost exceeds account balance"),
		rpc.HTTPError{StatusCode: 400, Status: "400 Bad Request", Body: []byte("exceeds block gas limit")},
		rpc.HTTPError{StatusCode: 429, Status: "429 Too Many Requests"},
		broadcastRPCError{-32603, "Internal error"},
		broadcastRPCError{-32000, "Execution reverted"},
		broadcastRPCError{-32000, "nonce too low: next nonce 8, tx nonce 7"},
		broadcastRPCError{-32001, "Nonce too low"},
		broadcastRPCError{-32000, "already known"},
		broadcastRPCError{-32000, "Known transaction"},
		broadcastRPCError{-32000, "replacement transaction underpriced"},
		broadcastRPCError{-32000, "Replacement transaction underpriced"},
		broadcastRPCError{-32003, "Transaction rejected"},
		broadcastRPCError{-32005, "Number of requests exceeds max batch size"},
		broadcastRPCError{-32000, "intrinsic gas too low: gas ?, minimum needed ?"},
	} {
		t.Run(err.Error(), func(t *testing.T) {
			require.False(t, isRejectedBroadcast(err))
		})
	}
}
