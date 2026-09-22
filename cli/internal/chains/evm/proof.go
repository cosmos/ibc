// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient/gethclient"
	"github.com/pkg/errors"
)

// AccountProof is an eth_getProof result for one account at one height: the
// account proof nodes against the block's state root and one storage proof
// per requested slot, in request order.
type AccountProof struct {
	AccountProof  [][]byte
	StorageProofs []StorageProof
}

// StorageProof is one storage slot's value and trie proof nodes from
// eth_getProof. Value is zero for an absent slot.
type StorageProof struct {
	Value *big.Int
	Proof [][]byte
}

// GetRouterProof proves the router account and the requested storage slots at
// height via eth_getProof. The light client verifies the proofs.
func (c *Client) GetRouterProof(ctx context.Context, height uint64, slots [][32]byte) (AccountProof, error) {
	keys := make([]string, len(slots))
	for i, slot := range slots {
		keys[i] = common.Hash(slot).Hex()
	}

	result, err := c.eth.GetProof(ctx, c.routerAddress, keys, heightToBigInt(height))
	if err != nil {
		return AccountProof{}, errors.Wrapf(err, "getting router proof at height %d on chain %s", height, c.chainID)
	}

	proof, err := accountProofFromResult(result, len(slots))
	if err != nil {
		return AccountProof{}, errors.Wrapf(err, "router proof at height %d on chain %s", height, c.chainID)
	}

	return proof, nil
}

// accountProofFromResult decodes an eth_getProof result whose storage proofs
// are in request order.
func accountProofFromResult(result *gethclient.AccountResult, slots int) (AccountProof, error) {
	if len(result.StorageProof) != slots {
		return AccountProof{}, errors.Errorf("%d storage proofs returned for %d slots", len(result.StorageProof), slots)
	}

	proofs := make([]StorageProof, slots)

	for i, storage := range result.StorageProof {
		if storage.Value == nil {
			return AccountProof{}, errors.Errorf("storage proof %d has no value", i)
		}

		nodes, err := decodeProofNodes(storage.Proof)
		if err != nil {
			return AccountProof{}, errors.Wrapf(err, "storage proof %d", i)
		}
		proofs[i] = StorageProof{Value: storage.Value, Proof: nodes}
	}

	accountNodes, err := decodeProofNodes(result.AccountProof)
	if err != nil {
		return AccountProof{}, errors.Wrap(err, "account proof")
	}

	return AccountProof{
		AccountProof:  accountNodes,
		StorageProofs: proofs,
	}, nil
}

func decodeProofNodes(nodes []string) ([][]byte, error) {
	out := make([][]byte, len(nodes))
	for i, node := range nodes {
		decoded, err := hexutil.Decode(node)
		if err != nil {
			return nil, errors.Wrapf(err, "node %d", i)
		}
		out[i] = decoded
	}

	return out, nil
}
