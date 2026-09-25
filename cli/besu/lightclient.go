// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"context"
	"fmt"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besuqbft"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	messageBindings     = besumsgs.NewBindings()
	ibcStoreStorageSlot = common.HexToHash(ics26router.IbcStoreStorageSlot)
)

// EncodeUpdateClient encodes the updateClient payload.
func EncodeUpdateClient(msg besumsgs.IBesuLightClientMsgsMsgUpdateClient) ([]byte, error) {
	data, err := messageBindings.TryPackUpdateClient(msg)
	if err != nil {
		return nil, fmt.Errorf("encode update client: %w", err)
	}

	return data[4:], nil
}

// EncodeMembershipProof encodes the proof bytes for verifyMembership and
// verifyNonMembership. AccountProofNodes may be empty when an earlier call in
// the same transaction proved the same height.
func EncodeMembershipProof(proof besumsgs.IBesuLightClientMsgsMembershipProof) ([]byte, error) {
	data, err := messageBindings.TryPackMembershipProof(proof)
	if err != nil {
		return nil, fmt.Errorf("encode membership proof: %w", err)
	}

	return data[4:], nil
}

// CommitmentSlot is the storage slot holding the commitment for a raw IBC
// path: keccak256(abi.encode(keccak256(path), IbcStoreStorageSlot)). Pass it
// to eth_getProof as the storage key; the light client hashes it again itself
// to form the trie key.
func CommitmentSlot(path []byte) common.Hash {
	return crypto.Keccak256Hash(crypto.Keccak256(path), ibcStoreStorageSlot.Bytes())
}

// ReadClientState reads the state of the Besu QBFT light client at address.
func ReadClientState(
	ctx context.Context,
	caller bind.ContractCaller,
	address common.Address,
) (besumsgs.IBesuLightClientMsgsClientState, error) {
	lightClient, err := besuqbft.NewContractCaller(address, caller)
	if err != nil {
		return besumsgs.IBesuLightClientMsgsClientState{}, err
	}

	raw, err := lightClient.GetClientState(&bind.CallOpts{Context: ctx})
	if err != nil {
		return besumsgs.IBesuLightClientMsgsClientState{}, fmt.Errorf("querying client state: %w", err)
	}

	state, err := messageBindings.UnpackClientState(raw)
	if err != nil {
		return besumsgs.IBesuLightClientMsgsClientState{}, fmt.Errorf(
			"%s is not a besu-qbft light client: %w", address, err,
		)
	}

	return state, nil
}
