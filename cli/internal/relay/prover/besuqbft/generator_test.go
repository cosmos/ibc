// SPDX-License-Identifier: Apache-2.0

package besuqbft

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/cli/besu/besutest"
	"github.com/cosmos/ibc/cli/internal/chains/evm"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

const clientID = "besu-chain-a"

type fakeChain struct {
	id             string
	router         common.Address
	latest         *v2.BlockHeader
	sealed         map[uint64]*besu.ParsedHeader
	clientState    besumsgs.IBesuLightClientMsgsClientState
	clientStateErr error
	proof          func(height uint64, slots [][32]byte) (evm.RouterProof, error)
}

func (f *fakeChain) ChainID() string { return f.id }

func (f *fakeChain) RouterAddress() common.Address { return f.router }

func (f *fakeChain) GetBlockHeader(_ context.Context, height uint64) (v2.BlockHeader, error) {
	if height == v2.LatestBlock {
		if f.latest != nil {
			return *f.latest, nil
		}
		return v2.BlockHeader{}, errors.New("no latest header")
	}
	return v2.BlockHeader{}, fmt.Errorf("no block header %d", height)
}

func (f *fakeChain) SealedHeader(_ context.Context, height uint64) (*besu.ParsedHeader, error) {
	h, ok := f.sealed[height]
	if !ok {
		return nil, fmt.Errorf("no sealed header %d", height)
	}
	return h, nil
}

func (f *fakeChain) GetRouterProof(_ context.Context, height uint64, slots [][32]byte) (evm.RouterProof, error) {
	if f.proof == nil {
		return evm.RouterProof{}, fmt.Errorf("no router proof at %d", height)
	}
	return f.proof(height, slots)
}

func (f *fakeChain) BesuQBFTClientState(context.Context, string) (besumsgs.IBesuLightClientMsgsClientState, error) {
	return f.clientState, f.clientStateErr
}

type fixtureEnv struct {
	fixture      besutest.Fixture
	host         *fakeChain
	counterparty *fakeChain
	gen          *Generator
}

func newFixtureEnv(t *testing.T) *fixtureEnv {
	t.Helper()

	env := &fixtureEnv{
		fixture:      besutest.MustFixture(t),
		host:         &fakeChain{id: "host", sealed: map[uint64]*besu.ParsedHeader{}},
		counterparty: &fakeChain{id: "besu-b", sealed: map[uint64]*besu.ParsedHeader{}},
	}
	env.counterparty.router = env.fixture.RouterAddress
	env.gen = &Generator{host: env.host, counterparty: env.counterparty, clientID: clientID}
	return env
}

func (e *fixtureEnv) clientState(latest uint64) besumsgs.IBesuLightClientMsgsClientState {
	return besumsgs.IBesuLightClientMsgsClientState{
		IbcRouter:      e.fixture.RouterAddress,
		LatestHeight:   besumsgs.IICS02ClientMsgsHeight{RevisionHeight: latest},
		TrustingPeriod: e.fixture.TrustingPeriod,
		MaxClockDrift:  e.fixture.MaxClockDrift,
	}
}

func accountNodes(t *testing.T, m besutest.MembershipFixture) [][]byte {
	t.Helper()
	nodes, err := m.AccountProofNodes()
	require.NoError(t, err)
	return nodes
}

func proofNodes(t *testing.T, m besutest.MembershipFixture) [][]byte {
	t.Helper()
	nodes, err := m.ProofNodes()
	require.NoError(t, err)
	return nodes
}

func consensusHeader(height uint64, state besumsgs.IBesuLightClientMsgsConsensusState) *besu.ParsedHeader {
	return &besu.ParsedHeader{
		Height:     height,
		Timestamp:  state.Timestamp,
		StateRoot:  state.StateRoot,
		Validators: state.Validators,
	}
}

func parsedUpdate(t *testing.T, update besutest.UpdateFixture) *besu.ParsedHeader {
	t.Helper()
	return besutest.ParseHeader(t, update.HeaderRLP)
}

func (e *fixtureEnv) setAnchor(t *testing.T, height uint64, state besumsgs.IBesuLightClientMsgsConsensusState) {
	t.Helper()
	e.host.clientState = e.clientState(height)
	e.counterparty.sealed[height] = consensusHeader(height, state)
}

func (e *fixtureEnv) expectInitialAnchor(t *testing.T) {
	t.Helper()
	e.setAnchor(t, e.fixture.InitialTrustedHeight, e.fixture.InitialConsensusState())
}

func TestClientUpdatePayloadDirectUpdate(t *testing.T) {
	ctx := context.Background()
	env := newFixtureEnv(t)
	update := env.fixture.NonAdjacentUpdate
	env.expectInitialAnchor(t)
	env.counterparty.sealed[update.Height] = parsedUpdate(t, update)

	payloads, err := env.gen.ClientUpdatePayloads(ctx, update.Height)
	require.NoError(t, err)
	require.Len(t, payloads, 1)

	decoded, err := besumsgs.NewBindings().UnpackUpdateClient(payloads[0])
	require.NoError(t, err)
	assert.Equal(t, []byte(update.HeaderRLP), decoded.HeaderRlp)
	assert.Equal(
		t,
		besumsgs.IICS02ClientMsgsHeight{RevisionHeight: env.fixture.InitialTrustedHeight},
		decoded.TrustedHeight,
	)
	assert.Equal(t, env.fixture.InitialConsensusState(), decoded.ConsensusStatePreimage)
}

// The generator sees the header's validators but leaves the overlap rule to
// the contract: a target sealed by a disjoint validator set is still encoded.
func TestClientUpdatePayloadDefersOverlapValidationToContract(t *testing.T) {
	env := newFixtureEnv(t)
	trusted := besumsgs.IBesuLightClientMsgsConsensusState{
		Timestamp:  1700000010,
		Validators: []common.Address{common.HexToAddress("0x01"), common.HexToAddress("0x02")},
	}
	env.setAnchor(t, 10, trusted)
	target := parsedUpdate(t, env.fixture.AdjacentUpdate)
	target.Height, target.Timestamp = 12, 1700000012
	target.Validators = []common.Address{common.HexToAddress("0x03"), common.HexToAddress("0x04")}
	env.counterparty.sealed[12] = target

	payloads, err := env.gen.ClientUpdatePayloads(t.Context(), 12)
	require.NoError(t, err)
	require.Len(t, payloads, 1)
	decoded, err := besumsgs.NewBindings().UnpackUpdateClient(payloads[0])
	require.NoError(t, err)
	require.Equal(t, target.RLP, decoded.HeaderRlp)
}

func TestClientUpdatePayloadTargetIsLatest(t *testing.T) {
	env := newFixtureEnv(t)
	update := env.fixture.NonAdjacentUpdate
	env.setAnchor(t, update.Height, update.ExpectedConsensusState())
	env.counterparty.sealed[update.Height] = parsedUpdate(t, update)

	payloads, err := env.gen.ClientUpdatePayloads(t.Context(), update.Height)
	require.NoError(t, err)
	assert.Empty(t, payloads, "no update needed")
}

// The contract stores a consensus state at the header's own height and only
// raises latestHeight when the header is newer, so a height below the latest
// one is updated from the latest trusted state.
func TestClientUpdatePayloadBackfillBelowTrusted(t *testing.T) {
	env := newFixtureEnv(t)
	update := env.fixture.NonAdjacentUpdate
	anchor := update.Height + 5
	env.setAnchor(t, anchor, env.fixture.InitialConsensusState())
	env.counterparty.sealed[update.Height] = parsedUpdate(t, update)

	payloads, err := env.gen.ClientUpdatePayloads(t.Context(), update.Height)
	require.NoError(t, err)
	require.Len(t, payloads, 1)

	decoded, err := besumsgs.NewBindings().UnpackUpdateClient(payloads[0])
	require.NoError(t, err)
	assert.Equal(t, besumsgs.IICS02ClientMsgsHeight{RevisionHeight: anchor}, decoded.TrustedHeight)
}

func (e *fixtureEnv) expectProofAt(
	t *testing.T,
	update besutest.UpdateFixture,
	nodesFor func(slot [32]byte) [][]byte,
) {
	t.Helper()
	e.counterparty.sealed[update.Height] = parsedUpdate(t, update)
	e.counterparty.proof = func(_ uint64, slots [][32]byte) (evm.RouterProof, error) {
		proof := evm.RouterProof{AccountProof: accountNodes(t, e.fixture.Membership)}
		for _, slot := range slots {
			proof.StorageProofs = append(proof.StorageProofs, nodesFor(slot))
		}
		return proof, nil
	}
}

func TestPacketProofs(t *testing.T) {
	ctx := context.Background()
	fixture := besutest.MustFixture(t)
	update := fixture.NonAdjacentUpdate
	preimage := update.ExpectedConsensusState()

	commitmentClient, commitmentSeq := splitPath(t, fixture.Membership.Path, 0x01)
	receiptClient, receiptSeq := splitPath(t, fixture.NonMembership.Path, 0x02)

	sent := channeltypesv2.Packet{
		Sequence:          commitmentSeq,
		SourceClient:      commitmentClient,
		DestinationClient: "some-other-client",
		TimeoutTimestamp:  1800000000,
		Payloads: []channeltypesv2.Payload{{
			SourcePort: "transfer", DestinationPort: "transfer", Version: "ics20-1",
			Encoding: "application/x-solidity-abi", Value: []byte{0xde, 0xad},
		}},
	}
	received := channeltypesv2.Packet{
		Sequence:          receiptSeq,
		SourceClient:      "some-other-client",
		DestinationClient: receiptClient,
	}
	membershipNodes := proofNodes(t, fixture.Membership)
	nonMembershipNodes := proofNodes(t, fixture.NonMembership)

	t.Run("membership wraps the storage proof with the proof-height preimage", func(t *testing.T) {
		env := newFixtureEnv(t)
		env.expectProofAt(t, update, func([32]byte) [][]byte { return membershipNodes })

		proofs, err := env.gen.PacketProofs(
			ctx,
			update.Height,
			v2.ProofKindPacketCommitment,
			[]channeltypesv2.Packet{sent},
		)
		require.NoError(t, err)
		require.Len(t, proofs, 1)

		decoded, err := besumsgs.NewBindings().UnpackMembershipProof(proofs[0])
		require.NoError(t, err)
		assert.Equal(t, preimage, decoded.ConsensusStatePreimage)
		assert.Equal(t, accountNodes(t, fixture.Membership), decoded.AccountProofNodes)
		assert.Equal(t, membershipNodes, decoded.ProofNodes)
	})

	t.Run("only the first proof carries the account proof", func(t *testing.T) {
		env := newFixtureEnv(t)
		env.expectProofAt(t, update, func([32]byte) [][]byte { return membershipNodes })
		next := sent
		next.Sequence++

		proofs, err := env.gen.PacketProofs(
			ctx,
			update.Height,
			v2.ProofKindPacketCommitment,
			[]channeltypesv2.Packet{sent, next},
		)
		require.NoError(t, err)
		require.Len(t, proofs, 2)

		first, err := besumsgs.NewBindings().UnpackMembershipProof(proofs[0])
		require.NoError(t, err)
		assert.NotEmpty(t, first.AccountProofNodes)
		second, err := besumsgs.NewBindings().UnpackMembershipProof(proofs[1])
		require.NoError(t, err)
		assert.Empty(t, second.AccountProofNodes)
		assert.Equal(t, preimage, second.ConsensusStatePreimage)
	})

	t.Run("receipt absence uses the fixture exclusion proof", func(t *testing.T) {
		env := newFixtureEnv(t)
		env.expectProofAt(t, update, func(slot [32]byte) [][]byte {
			require.Equal(t, besu.CommitmentSlot(fixture.NonMembership.Path), common.Hash(slot))
			return nonMembershipNodes
		})

		proofs, err := env.gen.PacketProofs(
			ctx,
			update.Height,
			v2.ProofKindReceiptAbsence,
			[]channeltypesv2.Packet{received},
		)
		require.NoError(t, err)
		require.Len(t, proofs, 1)

		decoded, err := besumsgs.NewBindings().UnpackMembershipProof(proofs[0])
		require.NoError(t, err)
		assert.Equal(t, preimage, decoded.ConsensusStatePreimage)
		assert.Equal(t, nonMembershipNodes, decoded.ProofNodes)
	})
}

// The head is returned even when it is ahead of the host clock: the contract
// rejects a header beyond maxClockDrift at simulation.
func TestLatestProvableHeightReturnsHead(t *testing.T) {
	env := newFixtureEnv(t)
	head := time.Unix(int64(env.fixture.InitialTrustedTimestamp), 0).UTC() //nolint:gosec // fixture timestamp
	env.counterparty.latest = &v2.BlockHeader{Height: env.fixture.InitialTrustedHeight + 50, Timestamp: head}

	height, ts, err := env.gen.LatestProvableHeight(t.Context())
	require.NoError(t, err)
	assert.Equal(t, env.fixture.InitialTrustedHeight+50, height)
	assert.Equal(t, head, ts)
}

func TestNewGeneratorChecksRouter(t *testing.T) {
	ctx := context.Background()

	t.Run("router mismatch", func(t *testing.T) {
		env := newFixtureEnv(t)
		env.host.clientState = env.clientState(env.fixture.InitialTrustedHeight)
		env.counterparty.router = common.HexToAddress("0x1234")
		_, err := NewGenerator(ctx, clientID, env.host, env.counterparty)
		require.ErrorContains(t, err, "configured with router")
	})

	t.Run("client state error", func(t *testing.T) {
		env := newFixtureEnv(t)
		env.host.clientStateErr = errors.New("execution reverted")
		_, err := NewGenerator(ctx, clientID, env.host, env.counterparty)
		require.ErrorContains(t, err, "execution reverted")
	})

	t.Run("matching router", func(t *testing.T) {
		env := newFixtureEnv(t)
		env.host.clientState = env.clientState(env.fixture.InitialTrustedHeight)
		_, err := NewGenerator(ctx, clientID, env.host, env.counterparty)
		require.NoError(t, err)
	})
}

func splitPath(t *testing.T, path []byte, kind byte) (string, uint64) {
	t.Helper()
	require.Greater(t, len(path), 9)
	require.Equal(t, kind, path[len(path)-9])
	return string(path[:len(path)-9]), binary.BigEndian.Uint64(path[len(path)-8:])
}
