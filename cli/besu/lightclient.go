// SPDX-License-Identifier: Apache-2.0

package besu

import (
	"context"
	"fmt"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besuibft2"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besuqbft"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// DefaultTrustLevel is the fraction of the validators trusted at the trusted height
// that must seal an update. It matches Besu's own two-thirds commit-seal
// quorum but is a separate check, against the trusted validator set.
var DefaultTrustLevel = besumsgs.IBesuLightClientMsgsTrustThreshold{Numerator: 2, Denominator: 3}

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

// DeployLightClient deploys the light client contract that verifies a chain
// running consensus.
func DeployLightClient(
	opts *bind.TransactOpts,
	backend bind.ContractBackend,
	consensus Consensus,
	clientState besumsgs.IBesuLightClientMsgsClientState,
	consensusState besumsgs.IBesuLightClientMsgsConsensusState,
	roleManager common.Address,
) (common.Address, *types.Transaction, error) {
	switch consensus {
	case QBFT:
		addr, tx, _, err := besuqbft.DeployContract(opts, backend, besuqbft.IBesuLightClientMsgsClientState{
			IbcRouter:      clientState.IbcRouter,
			LatestHeight:   besuqbft.IICS02ClientMsgsHeight(clientState.LatestHeight),
			TrustingPeriod: clientState.TrustingPeriod,
			MaxClockDrift:  clientState.MaxClockDrift,
			IsFrozen:       clientState.IsFrozen,
			TrustLevel:     besuqbft.IBesuLightClientMsgsTrustThreshold(clientState.TrustLevel),
		}, besuqbft.IBesuLightClientMsgsConsensusState(consensusState), roleManager)
		return addr, tx, err
	case IBFT2:
		addr, tx, _, err := besuibft2.DeployContract(opts, backend, besuibft2.IBesuLightClientMsgsClientState{
			IbcRouter:      clientState.IbcRouter,
			LatestHeight:   besuibft2.IICS02ClientMsgsHeight(clientState.LatestHeight),
			TrustingPeriod: clientState.TrustingPeriod,
			MaxClockDrift:  clientState.MaxClockDrift,
			IsFrozen:       clientState.IsFrozen,
			TrustLevel:     besuibft2.IBesuLightClientMsgsTrustThreshold(clientState.TrustLevel),
		}, besuibft2.IBesuLightClientMsgsConsensusState(consensusState), roleManager)
		return addr, tx, err
	default:
		return common.Address{}, nil, consensus.Validate()
	}
}

// ReadClientState reads the state of the Besu light client at address. The
// QBFT and IBFT2 clients share their ABI, so either binding reads both.
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
			"%s is not a besu light client: %w", address, err,
		)
	}

	return state, nil
}
