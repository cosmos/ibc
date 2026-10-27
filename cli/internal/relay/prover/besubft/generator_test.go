// SPDX-License-Identifier: Apache-2.0

package besubft

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/besumsgs"
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
	head           uint64
	sealed         map[uint64]*besu.ParsedHeader
	reads          []uint64
	clientState    besumsgs.IBesuLightClientMsgsClientState
	clientStateErr error
	stored         map[uint64]common.Hash
	proof          func(height uint64, slots []common.Hash) (evm.RouterProof, error)
}

func (f *fakeChain) ChainID() string { return f.id }

func (f *fakeChain) RouterAddress() common.Address { return f.router }

func (f *fakeChain) SealedHeader(_ context.Context, height uint64) (*besu.ParsedHeader, error) {
	if height == v2.LatestBlock {
		height = f.head
	}
	f.reads = append(f.reads, height)
	h, ok := f.sealed[height]
	if !ok {
		return nil, fmt.Errorf("no sealed header %d", height)
	}
	return h, nil
}

func (f *fakeChain) GetRouterProof(_ context.Context, height uint64, slots []common.Hash) (evm.RouterProof, error) {
	if f.proof == nil {
		return evm.RouterProof{}, fmt.Errorf("no router proof at %d", height)
	}
	return f.proof(height, slots)
}

func (f *fakeChain) BesuClientState(context.Context, string) (besumsgs.IBesuLightClientMsgsClientState, error) {
	return f.clientState, f.clientStateErr
}

func (f *fakeChain) BesuConsensusStateHash(_ context.Context, _ string, height uint64) (common.Hash, error) {
	hash, ok := f.stored[height]
	if !ok {
		return common.Hash{}, fmt.Errorf("execution reverted: ConsensusStateNotFound(%d)", height)
	}
	return hash, nil
}

type fixtureEnv struct {
	fixture      besutest.Fixture
	host         *fakeChain
	counterparty *fakeChain
	gen          *Generator
}

func newFixtureEnv(t *testing.T, consensus besu.Consensus) *fixtureEnv {
	t.Helper()

	env := &fixtureEnv{
		fixture:      besutest.MustFixture(t, consensus),
		host:         &fakeChain{id: "host", sealed: map[uint64]*besu.ParsedHeader{}},
		counterparty: &fakeChain{id: "besu-b", sealed: map[uint64]*besu.ParsedHeader{}},
	}
	env.counterparty.router = env.fixture.RouterAddress
	env.gen = &Generator{
		host:         env.host,
		counterparty: env.counterparty,
		clientID:     clientID,
		logger:       slog.New(slog.DiscardHandler),
	}
	return env
}

func (e *fixtureEnv) clientState(latest uint64) besumsgs.IBesuLightClientMsgsClientState {
	return besumsgs.IBesuLightClientMsgsClientState{
		IbcRouter:      e.fixture.RouterAddress,
		LatestHeight:   besumsgs.IICS02ClientMsgsHeight{RevisionHeight: latest},
		TrustingPeriod: e.fixture.TrustingPeriod,
		MaxClockDrift:  e.fixture.MaxClockDrift,
		TrustLevel:     besu.DefaultTrustLevel,
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
	for _, consensus := range besu.Consensuses {
		t.Run(string(consensus), func(t *testing.T) {
			ctx := context.Background()
			env := newFixtureEnv(t, consensus)
			update := env.fixture.NonAdjacentUpdate
			env.expectInitialAnchor(t)
			env.counterparty.sealed[update.Height] = besutest.ParseHeader(t, update.HeaderRLP)

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
		})
	}
}

func TestClientUpdatePayloadTargetIsLatest(t *testing.T) {
	env := newFixtureEnv(t, besu.QBFT)
	update := env.fixture.NonAdjacentUpdate
	env.setAnchor(t, update.Height, update.ExpectedConsensusState())
	env.counterparty.sealed[update.Height] = besutest.ParseHeader(t, update.HeaderRLP)

	payloads, err := env.gen.ClientUpdatePayloads(t.Context(), update.Height)
	require.NoError(t, err)
	assert.Empty(t, payloads, "no update needed")
}

// A concurrent update can leave the target below the latest height. The
// contract only accepts headers above the trusted height, so no update is
// sent when the client already stores the target.
func TestClientUpdatePayloadBelowLatestStored(t *testing.T) {
	env := newFixtureEnv(t, besu.QBFT)
	update := env.fixture.NonAdjacentUpdate
	env.setAnchor(t, update.Height+5, env.fixture.InitialConsensusState())
	env.host.stored = map[uint64]common.Hash{update.Height: common.HexToHash("0x01")}

	payloads, err := env.gen.ClientUpdatePayloads(t.Context(), update.Height)
	require.NoError(t, err)
	assert.Empty(t, payloads)
}

// When the client does not store a target below its latest height, proofs at
// target would revert, so the update fails with the reason instead.
func TestClientUpdatePayloadBelowLatestNotStored(t *testing.T) {
	env := newFixtureEnv(t, besu.QBFT)
	update := env.fixture.NonAdjacentUpdate
	latest := update.Height + 5
	env.setAnchor(t, latest, env.fixture.InitialConsensusState())

	_, err := env.gen.ClientUpdatePayloads(t.Context(), update.Height)
	require.ErrorContains(t, err, fmt.Sprintf("client advanced to height %d past target %d", latest, update.Height))
	require.ErrorContains(t, err, "ConsensusStateNotFound")
}

func (e *fixtureEnv) expectProofAt(
	t *testing.T,
	update besutest.UpdateFixture,
	nodesFor func(slot common.Hash) [][]byte,
) {
	t.Helper()
	e.counterparty.sealed[update.Height] = besutest.ParseHeader(t, update.HeaderRLP)
	e.counterparty.proof = func(height uint64, slots []common.Hash) (evm.RouterProof, error) {
		if height != update.Height {
			return evm.RouterProof{}, fmt.Errorf("router proof at %d, want %d", height, update.Height)
		}
		proof := evm.RouterProof{AccountProof: accountNodes(t, e.fixture.Membership)}
		for _, slot := range slots {
			proof.StorageProofs = append(proof.StorageProofs, nodesFor(slot))
		}
		return proof, nil
	}
}

func TestPacketProofs(t *testing.T) {
	for _, consensus := range besu.Consensuses {
		t.Run(string(consensus), func(t *testing.T) {
			testPacketProofs(t, consensus)
		})
	}
}

func testPacketProofs(t *testing.T, consensus besu.Consensus) {
	ctx := context.Background()
	fixture := besutest.MustFixture(t, consensus)
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
		env := newFixtureEnv(t, consensus)
		env.expectProofAt(t, update, func(common.Hash) [][]byte { return membershipNodes })

		proofs, err := env.gen.PacketProofs(
			ctx,
			update.Height,
			v2.ProofKindPacketCommitment,
			[]channeltypesv2.Packet{sent},
			nil,
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
		env := newFixtureEnv(t, consensus)
		env.expectProofAt(t, update, func(common.Hash) [][]byte { return membershipNodes })
		next := sent
		next.Sequence++

		proofs, err := env.gen.PacketProofs(
			ctx,
			update.Height,
			v2.ProofKindPacketCommitment,
			[]channeltypesv2.Packet{sent, next},
			nil,
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
		env := newFixtureEnv(t, consensus)
		env.expectProofAt(t, update, func(slot common.Hash) [][]byte {
			require.Equal(t, besu.CommitmentSlot(fixture.NonMembership.Path), slot)
			return nonMembershipNodes
		})

		proofs, err := env.gen.PacketProofs(
			ctx,
			update.Height,
			v2.ProofKindReceiptAbsence,
			[]channeltypesv2.Packet{received},
			nil,
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
	env := newFixtureEnv(t, besu.QBFT)
	update := env.fixture.NonAdjacentUpdate
	env.expectInitialAnchor(t)
	head := besutest.ParseHeader(t, update.HeaderRLP)
	head.Timestamp = uint64(time.Now().Add(time.Hour).Unix()) //nolint:gosec // test clock is positive
	env.counterparty.sealed[update.Height] = head
	env.counterparty.head = update.Height

	height, ts, err := env.gen.LatestProvableHeight(t.Context())
	require.NoError(t, err)
	assert.Equal(t, update.Height, height)
	assert.Equal(t, time.Unix(int64(head.Timestamp), 0).UTC(), ts) //nolint:gosec // test clock is positive
}

func TestNewGeneratorChecksRouter(t *testing.T) {
	ctx := context.Background()

	t.Run("router mismatch", func(t *testing.T) {
		env := newFixtureEnv(t, besu.QBFT)
		env.host.clientState = env.clientState(env.fixture.InitialTrustedHeight)
		env.counterparty.router = common.HexToAddress("0x1234")
		_, err := NewGenerator(ctx, clientID, env.host, env.counterparty, slog.New(slog.DiscardHandler))
		require.ErrorContains(t, err, "configured with router")
	})

	t.Run("client state error", func(t *testing.T) {
		env := newFixtureEnv(t, besu.QBFT)
		env.host.clientStateErr = errors.New("execution reverted")
		_, err := NewGenerator(ctx, clientID, env.host, env.counterparty, slog.New(slog.DiscardHandler))
		require.ErrorContains(t, err, "execution reverted")
	})

	t.Run("matching router", func(t *testing.T) {
		env := newFixtureEnv(t, besu.QBFT)
		env.host.clientState = env.clientState(env.fixture.InitialTrustedHeight)
		_, err := NewGenerator(ctx, clientID, env.host, env.counterparty, slog.New(slog.DiscardHandler))
		require.NoError(t, err)
	})
}

func splitPath(t *testing.T, path []byte, kind byte) (string, uint64) {
	t.Helper()
	require.Greater(t, len(path), 9)
	require.Equal(t, kind, path[len(path)-9])
	return string(path[:len(path)-9]), binary.BigEndian.Uint64(path[len(path)-8:])
}
