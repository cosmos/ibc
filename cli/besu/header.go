// SPDX-License-Identifier: Apache-2.0

package besu

import "github.com/ethereum/go-ethereum/common"

// ParsedHeader is a Besu header's exact RLP plus the few fields decoded from
// it that light-client payloads need; consensus validity is checked by the
// contract.
type ParsedHeader struct {
	RLP        []byte
	Height     uint64
	Timestamp  uint64
	StateRoot  common.Hash
	Validators []common.Address
}
