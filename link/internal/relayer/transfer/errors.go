package transfer

import "github.com/pkg/errors"

// Retryable pipeline conditions
var (
	ErrSendNotFinalized     = errors.New("send tx not finalized")
	ErrTimeoutNotFinalized  = errors.New("timeout timestamp not finalized")
	ErrWriteAckNotFinalized = errors.New("write ack tx not finalized")

	ErrRetryingRecvPacket    = errors.New("retrying recv packet")
	ErrRetryingAckPacket     = errors.New("retrying ack packet")
	ErrRetryingTimeoutPacket = errors.New("retrying timeout packet")
)
