// SPDX-License-Identifier: Apache-2.0

package besu

import "github.com/ethereum/go-ethereum/common"

// Header contains the fields needed to construct light-client payloads.
// RLP is preserved exactly; consensus validity is checked by the contract.
type Header struct {
	RLP        []byte
	Height     uint64
	Timestamp  uint64
	StateRoot  common.Hash
	Validators []common.Address
}
