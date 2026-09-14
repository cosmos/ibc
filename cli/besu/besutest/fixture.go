// SPDX-License-Identifier: Apache-2.0

// Package besutest provides the live Besu QBFT fixture captured in
// ibc-contracts and a builder for synthetic sealed headers, for tests in the
// cli and e2e modules.
package besutest

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/cosmos/ibc/cli/besu"

	_ "embed"
)

// qbftFixtureJSON is ibc-solidity/test/besu-bft/fixtures/qbft.json from
// ibc-contracts: real headers, account proofs and storage proofs captured from
// a four-validator Besu QBFT chain during an e2e transfer.
//
//go:embed testdata/qbft.json
var qbftFixtureJSON []byte

// Fixture mirrors the qbft.json layout.
type Fixture struct {
	RouterAddress             common.Address   `json:"routerAddress"`
	InitialTrustedHeight      uint64           `json:"initialTrustedHeight"`
	InitialTrustedTimestamp   uint64           `json:"initialTrustedTimestamp"`
	InitialTrustedStorageRoot common.Hash      `json:"initialTrustedStorageRoot"`
	InitialTrustedValidators  []common.Address `json:"initialTrustedValidators"`
	TrustingPeriod            uint64           `json:"trustingPeriod"`
	MaxClockDrift             uint64           `json:"maxClockDrift"`

	AdjacentUpdate    UpdateFixture `json:"adjacentUpdate"`
	NonAdjacentUpdate UpdateFixture `json:"nonAdjacentUpdate"`
	LowQuorumUpdate   UpdateFixture `json:"lowQuorumUpdate"`
	ConflictingUpdate UpdateFixture `json:"conflictingUpdate"`
	LowOverlapUpdate  UpdateFixture `json:"lowOverlapUpdate"`

	Membership    MembershipFixture `json:"membership"`
	NonMembership MembershipFixture `json:"nonMembership"`
}

// UpdateFixture is one header update. The negative cases carry no expected
// state and an empty account proof.
type UpdateFixture struct {
	Height              uint64           `json:"height"`
	HeaderRLP           hexutil.Bytes    `json:"headerRlp"`
	TrustedHeight       uint64           `json:"trustedHeight"`
	AccountProof        hexutil.Bytes    `json:"accountProof"` // abi.encode(bytes[])
	ExpectedTimestamp   uint64           `json:"expectedTimestamp"`
	ExpectedStorageRoot common.Hash      `json:"expectedStorageRoot"`
	ExpectedValidators  []common.Address `json:"expectedValidators"`
}

// MembershipFixture is one storage proof. Value is empty for non-membership.
type MembershipFixture struct {
	Proof             hexutil.Bytes `json:"proof"` // abi.encode(bytes[]) of the storage proof nodes
	ProofHeight       uint64        `json:"proofHeight"`
	Path              hexutil.Bytes `json:"path"`
	Value             hexutil.Bytes `json:"value"`
	ExpectedTimestamp uint64        `json:"expectedTimestamp"`
}

// LoadFixture decodes the embedded qbft.json.
func LoadFixture() (Fixture, error) {
	var fixture Fixture
	if err := json.Unmarshal(qbftFixtureJSON, &fixture); err != nil {
		return Fixture{}, fmt.Errorf("decode qbft fixture: %w", err)
	}

	return fixture, nil
}

// MustFixture is LoadFixture for tests.
func MustFixture(tb testing.TB) Fixture {
	tb.Helper()

	fixture, err := LoadFixture()
	if err != nil {
		tb.Fatal(err)
	}

	return fixture
}

// InitialConsensusState is the consensus state the fixture client is deployed with.
func (f Fixture) InitialConsensusState() besu.ConsensusState {
	return besu.ConsensusState{
		Timestamp:   f.InitialTrustedTimestamp,
		StorageRoot: f.InitialTrustedStorageRoot,
		Validators:  f.InitialTrustedValidators,
	}
}

// ExpectedConsensusState is the consensus state the update installs.
func (u UpdateFixture) ExpectedConsensusState() besu.ConsensusState {
	return besu.ConsensusState{
		Timestamp:   u.ExpectedTimestamp,
		StorageRoot: u.ExpectedStorageRoot,
		Validators:  u.ExpectedValidators,
	}
}

// AccountProofNodes unwraps the abi.encode(bytes[]) account proof.
func (u UpdateFixture) AccountProofNodes() ([][]byte, error) {
	return DecodeProofNodes(u.AccountProof)
}

// ProofNodes unwraps the abi.encode(bytes[]) storage proof.
func (m MembershipFixture) ProofNodes() ([][]byte, error) {
	return DecodeProofNodes(m.Proof)
}
