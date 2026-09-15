// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"errors"
	"regexp"

	"github.com/ethereum/go-ethereum/rpc"
)

// These complete message formats come from geth's core/txpool/validation.go.
var gethValidationRejection = regexp.MustCompile(`^(intrinsic gas too low: gas [0-9]+, minimum needed [0-9]+|` +
	`transaction gas price below minimum: gas tip cap [0-9]+, minimum needed [0-9]+|` +
	`insufficient funds for gas \* price \+ value: balance [0-9]+, (tx cost [0-9]+|` +
	`queued cost [0-9]+, tx (cost|bumped) [0-9]+), overshot [0-9]+)$`)

func isRejectedBroadcast(err error) bool {
	var rpcErr rpc.Error
	if !errors.As(err, &rpcErr) {
		return false
	}

	// Require both the code and complete message: these codes also describe
	// unrelated failures. Unknown, internal, nonce and known-tx errors stay ambiguous.
	// Besu pairs: ethereum/api/.../response/RpcErrorType.java (25.4.0).
	switch rpcErr.ErrorCode() {
	case -32000:
		switch rpcErr.Error() {
		case "exceeds block gas limit", "transaction underpriced",
			"insufficient funds for gas * price + value",
			"max priority fee per gas higher than max fee per gas",
			"Wrong chainId", "ChainId not supported", "ChainId is required",
			"Transaction fee cap exceeded", "Max priority fee per gas exceeds max fee per gas":
			return true
		}
		return gethValidationRejection.MatchString(rpcErr.Error())
	case -32002:
		return rpcErr.Error() == "Invalid signature"
	case -32003:
		return rpcErr.Error() == "Intrinsic gas exceeds gas limit"
	case -32004:
		return rpcErr.Error() == "Upfront cost exceeds account balance"
	case -32005:
		return rpcErr.Error() == "Transaction gas limit exceeds block gas limit"
	case -32007:
		return rpcErr.Error() == "Sender account not authorized to send transactions"
	case -32009:
		return rpcErr.Error() == "Gas price below configured minimum gas price" ||
			rpcErr.Error() == "Gas price below current base fee"
	default:
		return false
	}
}
