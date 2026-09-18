// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// IBCStoreStorageSlot is the ERC-7201 namespace slot of IBCStoreUpgradeable's
// commitment mapping, keccak256(abi.encode(uint256(keccak256("ibc.storage.IBCStore")) - 1)) & ~0xff.
var IBCStoreStorageSlot = common.HexToHash("0x1260944489272988d9df285149b5aa1b0f48f2136d6f416159f840a3e0747600")

// CommitmentSlot is the storage slot holding the commitment for a raw IBC
// path: keccak256(abi.encode(keccak256(path), IBCStoreStorageSlot)). Pass it
// to eth_getProof as the storage key; the light client hashes it again itself
// to form the trie key.
func CommitmentSlot(path []byte) common.Hash {
	return crypto.Keccak256Hash(crypto.Keccak256(path), IBCStoreStorageSlot.Bytes())
}
