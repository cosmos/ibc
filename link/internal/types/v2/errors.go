// SPDX-License-Identifier: Apache-2.0

package v2

import "github.com/pkg/errors"

// Chain query errors
var (
	ErrTxNotFound                = errors.New("tx not found")
	ErrWriteAckNotFoundForPacket = errors.New("write ack for packet not found in tx")
	ErrWriteAckDecoding          = errors.New("could not decode write ack")
)
