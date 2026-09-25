// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
)

// RouterProof is an eth_getProof result for the router at one height: the
// account proof nodes against the block's state root and the storage proof
// nodes of each requested slot, in request order.
type RouterProof struct {
	AccountProof  [][]byte
	StorageProofs [][][]byte
}

// GetRouterProof proves the router account and the requested storage slots at
// height via eth_getProof. The light client verifies the proofs, deriving each
// storage key from the packet itself.
func (c *Client) GetRouterProof(ctx context.Context, height uint64, slots []common.Hash) (RouterProof, error) {
	accountProof, storageProofs, err := c.eth.GetProof(ctx, c.routerAddress, slots, heightToBigInt(height))
	if err != nil {
		return RouterProof{}, errors.Wrapf(err, "getting router proof at height %d on chain %s", height, c.chainID)
	}

	if len(storageProofs) != len(slots) {
		return RouterProof{}, errors.Errorf(
			"router proof at height %d on chain %s: %d storage proofs returned for %d slots",
			height, c.chainID, len(storageProofs), len(slots),
		)
	}

	return RouterProof{AccountProof: accountProof, StorageProofs: storageProofs}, nil
}
