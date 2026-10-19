// SPDX-License-Identifier: Apache-2.0

// Package besutest provides the Besu QBFT and IBFT2 fixtures from
// ibc-contracts for tests in the cli module.
package besutest

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/besumsgs"
	"github.com/cosmos/ibc/cli/besu"

	_ "embed"
)

// fixtureJSON holds ibc-solidity/test/besu-bft/fixtures/<consensus>.json from
// ibc-contracts at the go-abigen commit (make sync-besu-fixture): headers and
// proofs, plus synthetic updates for the negative Solidity cases.
//
//go:embed testdata/qbft.json
var qbftFixtureJSON []byte

//go:embed testdata/ibft2.json
var ibft2FixtureJSON []byte

var fixtureJSON = map[besu.Consensus][]byte{
	besu.QBFT:  qbftFixtureJSON,
	besu.IBFT2: ibft2FixtureJSON,
}

var messageBindings = besumsgs.NewBindings()

// Fixture mirrors the fixture JSON layout.
//
// TODO(FOU-1406): Move fixture generation from ibc-contracts into this repo's
// e2e flow and share these fixture types with the generator.
type Fixture struct {
	// Consensus is the consensus the fixture chain runs.
	Consensus besu.Consensus `json:"-"`

	RouterAddress            common.Address   `json:"routerAddress"`
	InitialTrustedHeight     uint64           `json:"initialTrustedHeight"`
	InitialTrustedTimestamp  uint64           `json:"initialTrustedTimestamp"`
	InitialTrustedStateRoot  common.Hash      `json:"initialTrustedStateRoot"`
	InitialTrustedValidators []common.Address `json:"initialTrustedValidators"`
	TrustingPeriod           uint64           `json:"trustingPeriod"`
	MaxClockDrift            uint64           `json:"maxClockDrift"`

	AdjacentUpdate    UpdateFixture `json:"adjacentUpdate"`
	NonAdjacentUpdate UpdateFixture `json:"nonAdjacentUpdate"`

	Membership    MembershipFixture `json:"membership"`
	NonMembership MembershipFixture `json:"nonMembership"`
}

// UpdateFixture is one header update used by Go tests. The negative Solidity
// cases remain in the fixture JSON but are not decoded here.
type UpdateFixture struct {
	Height             uint64           `json:"height"`
	HeaderRLP          hexutil.Bytes    `json:"headerRlp"`
	TrustedHeight      uint64           `json:"trustedHeight"`
	ExpectedTimestamp  uint64           `json:"expectedTimestamp"`
	ExpectedStateRoot  common.Hash      `json:"expectedStateRoot"`
	ExpectedValidators []common.Address `json:"expectedValidators"`
}

// MembershipFixture is one storage proof with the account proof that anchors
// it to the state root of NonAdjacentUpdate's header, the height both proofs
// were captured at.
type MembershipFixture struct {
	Proof        hexutil.Bytes `json:"proof"`        // abi.encode(bytes[]) of the storage proof nodes
	AccountProof hexutil.Bytes `json:"accountProof"` // abi.encode(bytes[]) of the account proof nodes
	Path         hexutil.Bytes `json:"path"`
}

// LoadFixture decodes the embedded fixture of a chain running consensus.
func LoadFixture(consensus besu.Consensus) (Fixture, error) {
	raw, ok := fixtureJSON[consensus]
	if !ok {
		return Fixture{}, fmt.Errorf("no fixture for besu consensus %q", consensus)
	}

	fixture := Fixture{Consensus: consensus}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		return Fixture{}, fmt.Errorf("decode %s fixture: %w", consensus, err)
	}

	return fixture, nil
}

// MustFixture is LoadFixture for tests.
func MustFixture(tb testing.TB, consensus besu.Consensus) Fixture {
	tb.Helper()

	fixture, err := LoadFixture(consensus)
	if err != nil {
		tb.Fatal(err)
	}

	return fixture
}

// InitialConsensusState is the consensus state the fixture client is deployed with.
func (f Fixture) InitialConsensusState() besumsgs.IBesuLightClientMsgsConsensusState {
	return besumsgs.IBesuLightClientMsgsConsensusState{
		Timestamp:  f.InitialTrustedTimestamp,
		StateRoot:  f.InitialTrustedStateRoot,
		Validators: f.InitialTrustedValidators,
	}
}

// ExpectedConsensusState is the consensus state the update installs.
func (u UpdateFixture) ExpectedConsensusState() besumsgs.IBesuLightClientMsgsConsensusState {
	return besumsgs.IBesuLightClientMsgsConsensusState{
		Timestamp:  u.ExpectedTimestamp,
		StateRoot:  u.ExpectedStateRoot,
		Validators: u.ExpectedValidators,
	}
}

// ProofNodes unwraps the abi.encode(bytes[]) storage proof.
func (m MembershipFixture) ProofNodes() ([][]byte, error) {
	return messageBindings.UnpackProofNodes(m.Proof)
}

// AccountProofNodes unwraps the abi.encode(bytes[]) account proof.
func (m MembershipFixture) AccountProofNodes() ([][]byte, error) {
	return messageBindings.UnpackProofNodes(m.AccountProof)
}

// HashConsensusState is keccak256(abi.encode(state)), the hash the light client
// stores per height.
func HashConsensusState(state besumsgs.IBesuLightClientMsgsConsensusState) (common.Hash, error) {
	data, err := messageBindings.TryPackConsensusState(state)
	if err != nil {
		return common.Hash{}, fmt.Errorf("encode consensus state: %w", err)
	}

	return crypto.Keccak256Hash(data[4:]), nil
}

// ParseHeader parses a header RLP of the fixture chain the way the relayer
// parses a node's header.
func (f Fixture) ParseHeader(tb testing.TB, headerRLP []byte) *besu.ParsedHeader {
	tb.Helper()

	var header types.Header
	if err := rlp.DecodeBytes(headerRLP, &header); err != nil {
		tb.Fatalf("decode header: %v", err)
	}

	parsed, err := besu.ParseSealedHeader(&header, f.Consensus)
	if err != nil {
		tb.Fatalf("parse header: %v", err)
	}

	return parsed
}
