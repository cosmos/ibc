// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient/gethclient"
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
// height via eth_getProof. The light client verifies the proofs.
func (c *Client) GetRouterProof(ctx context.Context, height uint64, slots []common.Hash) (RouterProof, error) {
	keys := make([]string, len(slots))
	for i, slot := range slots {
		keys[i] = slot.Hex()
	}

	result, err := c.eth.GetProof(ctx, c.routerAddress, keys, heightToBigInt(height))
	if err != nil {
		return RouterProof{}, errors.Wrapf(err, "getting router proof at height %d on chain %s", height, c.chainID)
	}

	proof, err := routerProofFromResult(result, len(slots))
	if err != nil {
		return RouterProof{}, errors.Wrapf(err, "router proof at height %d on chain %s", height, c.chainID)
	}

	return proof, nil
}

// routerProofFromResult decodes an eth_getProof result for the given number of
// requested slots, taking its storage proofs in request order. The light
// client checks each proof against the key it derives from the packet.
func routerProofFromResult(result *gethclient.AccountResult, slotCount int) (RouterProof, error) {
	if len(result.StorageProof) != slotCount {
		return RouterProof{}, errors.Errorf(
			"%d storage proofs returned for %d slots",
			len(result.StorageProof),
			slotCount,
		)
	}

	proofs := make([][][]byte, slotCount)

	for i, storage := range result.StorageProof {
		nodes, err := decodeProofNodes(storage.Proof)
		if err != nil {
			return RouterProof{}, errors.Wrapf(err, "storage proof %d", i)
		}
		proofs[i] = nodes
	}

	accountNodes, err := decodeProofNodes(result.AccountProof)
	if err != nil {
		return RouterProof{}, errors.Wrap(err, "account proof")
	}

	return RouterProof{
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
