// SPDX-License-Identifier: Apache-2.0

package v2

import "github.com/pkg/errors"

// ErrTxRejected marks a broadcast that the node definitely rejected before admission.
// Other broadcast errors may have left the transaction pending or confirmed.
var ErrTxRejected = errors.New("transaction rejected")

// Chain query errors
var (
	ErrTxNotFound                = errors.New("tx not found")
	ErrWriteAckNotFoundForPacket = errors.New("write ack for packet not found in tx")
	ErrWriteAckDecoding          = errors.New("could not decode write ack")
)
