// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var ibcStoreStorageSlot = common.HexToHash(ics26router.IbcStoreStorageSlot)

// CommitmentSlot is the storage slot holding the commitment for a raw IBC
// path: keccak256(abi.encode(keccak256(path), IbcStoreStorageSlot)). Pass it
// to eth_getProof as the storage key; the light client hashes it again itself
// to form the trie key.
func CommitmentSlot(path []byte) common.Hash {
	return crypto.Keccak256Hash(crypto.Keccak256(path), ibcStoreStorageSlot.Bytes())
}
