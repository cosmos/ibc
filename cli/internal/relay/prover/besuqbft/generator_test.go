// SPDX-License-Identifier: Apache-2.0

package besuqbft

import (
	"context"
	"crypto/ecdsa"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/cli/besu/besutest"
	"github.com/cosmos/ibc/cli/internal/tests/mocks"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

const clientID = "besu-chain-a"

type fixtureEnv struct {
	fixture      besutest.Fixture
	host         *mocks.MockClient
	counterparty *mocks.MockClient
	gen          *Generator
}

func newFixtureEnv(t *testing.T) *fixtureEnv {
	t.Helper()

	env := &fixtureEnv{
		fixture:      besutest.MustFixture(t),
		host:         mocks.NewMockClient(t),
		counterparty: mocks.NewMockClient(t),
	}
	env.gen = New(env.host, env.counterparty, clientID)

	return env
}

func (e *fixtureEnv) clientState(latest uint64) besu.ClientState {
	return besu.ClientState{
		IBCRouter:      e.fixture.RouterAddress,
		LatestHeight:   latest,
		TrustingPeriod: e.fixture.TrustingPeriod,
		MaxClockDrift:  e.fixture.MaxClockDrift,
	}
}

func mustHash(t *testing.T, state besu.ConsensusState) [32]byte {
	t.Helper()

	hash, err := state.Hash()
	require.NoError(t, err)

	return hash
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

// expectHeader wires the counterparty header read for a fixture update; an
// update needs nothing else from the counterparty.
func (e *fixtureEnv) expectHeader(t *testing.T, update besutest.UpdateFixture) {
	t.Helper()

	e.counterparty.EXPECT().GetHeaderRLP(mock.Anything, update.Height).Return([]byte(update.HeaderRLP), nil)
}

// expectInitialAnchor makes the initial trusted state resolvable: the fixture
// has no header for it, so the cache is seeded as an unverified entry that the
// prover verifies against the stored hash.
func (e *fixtureEnv) expectInitialAnchor(t *testing.T) {
	t.Helper()

	e.gen.store(e.fixture.InitialTrustedHeight, e.fixture.InitialConsensusState(), false)
	e.host.EXPECT().GetBesuQBFTConsensusStateHash(mock.Anything, clientID, e.fixture.InitialTrustedHeight).
		Return(mustHash(t, e.fixture.InitialConsensusState()), nil).Once()
}

func TestStateProofDirectUpdate(t *testing.T) {
	ctx := context.Background()
	env := newFixtureEnv(t)
	update := env.fixture.NonAdjacentUpdate

	env.host.EXPECT().GetBesuQBFTClientState(mock.Anything, clientID).
		Return(env.clientState(env.fixture.InitialTrustedHeight), nil).Once()
	env.expectInitialAnchor(t)
	env.expectHeader(t, update)

	updates, err := env.prepareUpdate(ctx, update.Height)
	require.NoError(t, err)
	require.Len(t, updates, 1)

	decoded, err := besutest.DecodeUpdateClient(updates[0])
	require.NoError(t, err)
	assert.Equal(t, []byte(update.HeaderRLP), decoded.HeaderRLP)
	assert.Equal(t, env.fixture.InitialTrustedHeight, decoded.TrustedHeight)
	assert.Equal(t, env.fixture.InitialConsensusState(), decoded.ConsensusStatePreimage)

	// the installed state is cached unverified for the packet proofs that follow
	entry, ok := env.gen.cache[update.Height]
	require.True(t, ok)
	assert.False(t, entry.verified)
	assert.Equal(t, update.ExpectedConsensusState(), entry.state)
}

func sealedHeader(t *testing.T, template []byte, height uint64, keys []*ecdsa.PrivateKey) *besu.Header {
	t.Helper()

	header, err := besutest.MustBuilder(template).
		SetHeight(height).
		SetTimestamp(1700000000 + height).
		SetValidators(besutest.Addresses(keys)).
		MustSign(keys...).Header()
	require.NoError(t, err)
	return header
}

func TestStateProofValidatorTurnoverChain(t *testing.T) {
	env := newFixtureEnv(t)
	keys := besutest.Keys(10)
	trusted := besu.ConsensusState{Timestamp: 1700000010, Validators: besutest.Addresses(keys[:4])}
	height := uint64(10)
	headers := make(map[uint64]*besu.Header)
	for i, set := range [][]*ecdsa.PrivateKey{keys[2:6], keys[2:6], keys[4:8], keys[4:8], keys[6:10]} {
		h := uint64(i) + 11
		headers[h] = sealedHeader(t, env.fixture.AdjacentUpdate.HeaderRLP, h, set)
	}
	env.counterparty.EXPECT().
		GetHeaderRLP(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, h uint64) ([]byte, error) { return headers[h].RLP, nil })
	env.gen.store(height, trusted, true)
	env.host.EXPECT().GetBesuQBFTClientState(mock.Anything, clientID).Return(env.clientState(height), nil).Once()
	hops := []uint64{12, 14, 15}

	updates, err := env.prepareUpdate(t.Context(), 15)
	require.NoError(t, err)
	require.Len(t, updates, len(hops))
	// Each hop is the furthest header the previous hop's validators accept,
	// and trusts that previous hop rather than the on-chain anchor.
	for i, hop := range hops {
		update, err := besutest.DecodeUpdateClient(updates[i])
		require.NoError(t, err)
		require.Equal(t, height, update.TrustedHeight)
		require.Equal(t, trusted, update.ConsensusStatePreimage)
		require.Equal(t, headers[hop].RLP, update.HeaderRLP)
		signers, err := headers[hop].Signers()
		require.NoError(t, err)
		require.NoError(t, besu.CheckUpdate(headers[hop], signers, trusted))
		height = hop
		trusted = consensusOf(headers[hop])
	}
	require.False(t, env.gen.cache[15].verified)
	_, cached := env.gen.cache[12]
	require.False(t, cached, "intermediate hops are not anchors until the client stores them")
}

// sealedHeaderBy builds a header whose validator set and commit sealers differ.
func sealedHeaderBy(t *testing.T, template []byte, height uint64, validators, sealers []*ecdsa.PrivateKey) *besu.Header {
	t.Helper()

	header, err := besutest.MustBuilder(template).
		SetHeight(height).
		SetTimestamp(1700000000 + height).
		SetValidators(besutest.Addresses(validators)).
		MustSign(sealers...).Header()
	require.NoError(t, err)
	return header
}

func TestStateProofBridgesThinlySealedBlock(t *testing.T) {
	env := newFixtureEnv(t)
	keys := besutest.Keys(8)
	trusted := besu.ConsensusState{Timestamp: 1700000010, Validators: besutest.Addresses(keys[:4])}
	env.gen.store(10, trusted, true)
	env.host.EXPECT().GetBesuQBFTClientState(mock.Anything, clientID).Return(env.clientState(10), nil).Once()
	template := env.fixture.AdjacentUpdate.HeaderRLP
	headers := map[uint64]*besu.Header{
		// Same new set at 11 and 12, but only one trusted validator sealed 11.
		11: sealedHeaderBy(t, template, 11, keys[2:6], keys[3:6]),
		12: sealedHeaderBy(t, template, 12, keys[2:6], keys[2:5]),
		13: sealedHeaderBy(t, template, 13, keys[4:8], keys[4:8]),
	}
	env.counterparty.EXPECT().GetHeaderRLP(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, h uint64) ([]byte, error) { return headers[h].RLP, nil })
	updates, err := env.prepareUpdate(t.Context(), 13)
	require.NoError(t, err)
	require.Len(t, updates, 2)
	for i, want := range []uint64{12, 13} {
		update, err := besutest.DecodeUpdateClient(updates[i])
		require.NoError(t, err)
		require.Equal(t, headers[want].RLP, update.HeaderRLP)
	}
}

func TestStateProofUnbridgeableTurnover(t *testing.T) {
	env := newFixtureEnv(t)
	keys := besutest.Keys(8)
	env.gen.store(10, besu.ConsensusState{Timestamp: 1700000010, Validators: besutest.Addresses(keys[:4])}, true)
	env.host.EXPECT().GetBesuQBFTClientState(mock.Anything, clientID).Return(env.clientState(10), nil).Once()
	for _, height := range []uint64{11, 12} {
		header := sealedHeader(t, env.fixture.AdjacentUpdate.HeaderRLP, height, keys[4:])
		env.counterparty.EXPECT().GetHeaderRLP(mock.Anything, height).Return(header.RLP, nil).Once()
	}

	updates, err := env.prepareUpdate(context.Background(), 12)
	require.ErrorIs(t, err, besu.ErrInsufficientOverlap)
	require.ErrorContains(t, err, "no header after trusted height 10")
	assert.Empty(t, updates)
}

func TestStateProofTargetAlreadyStored(t *testing.T) {
	ctx := context.Background()
	env := newFixtureEnv(t)
	update := env.fixture.NonAdjacentUpdate

	env.host.EXPECT().GetBesuQBFTClientState(mock.Anything, clientID).Return(env.clientState(update.Height), nil).Once()
	// trusted anchor is the update height itself
	env.expectHeader(t, update)
	env.host.EXPECT().GetBesuQBFTConsensusStateHash(mock.Anything, clientID, update.Height).
		Return(mustHash(t, update.ExpectedConsensusState()), nil).Once()

	updates, err := env.prepareUpdate(ctx, update.Height)
	require.NoError(t, err)
	assert.Nil(t, updates, "no update needed")
}

func TestStateProofTargetEqualsVerifiedAnchor(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(fmt.Sprintf("conflict=%t", conflict), func(t *testing.T) {
			env := newFixtureEnv(t)
			update := env.fixture.NonAdjacentUpdate
			trusted := update.ExpectedConsensusState()
			if conflict {
				trusted.Timestamp++
			}
			env.gen.store(update.Height, trusted, true)
			env.host.EXPECT().
				GetBesuQBFTClientState(mock.Anything, clientID).
				Return(env.clientState(update.Height), nil).
				Once()
			env.expectHeader(t, update)

			updates, err := env.prepareUpdate(context.Background(), update.Height)
			if conflict {
				require.ErrorIs(t, err, ErrConflictingConsensusState)
			} else {
				require.NoError(t, err)
			}
			require.Empty(t, updates)
			require.Equal(t, trusted, env.gen.cache[update.Height].state)
			// No consensus hash RPC is needed for an already verified anchor.
		})
	}
}

func TestStateProofTargetConflicts(t *testing.T) {
	ctx := context.Background()
	env := newFixtureEnv(t)
	update := env.fixture.NonAdjacentUpdate
	tampered := update.ExpectedConsensusState()
	tampered.Timestamp++

	env.host.EXPECT().
		GetBesuQBFTClientState(mock.Anything, clientID).
		Return(env.clientState(update.Height+1), nil).
		Once()
	env.gen.store(update.Height+1, tampered, true) // some verified anchor above the target
	env.expectHeader(t, update)
	env.host.EXPECT().GetBesuQBFTConsensusStateHash(mock.Anything, clientID, update.Height).
		Return(mustHash(t, tampered), nil).Once()

	_, err := env.prepareUpdate(ctx, update.Height)
	require.ErrorIs(t, err, ErrConflictingConsensusState)
}

func TestStateProofBackfillBelowTrusted(t *testing.T) {
	ctx := context.Background()
	env := newFixtureEnv(t)
	update := env.fixture.NonAdjacentUpdate
	anchor := update.Height + 5

	env.host.EXPECT().GetBesuQBFTClientState(mock.Anything, clientID).Return(env.clientState(anchor), nil).Once()
	env.gen.store(anchor, env.fixture.InitialConsensusState(), true)
	env.expectHeader(t, update)
	env.host.EXPECT().GetBesuQBFTConsensusStateHash(mock.Anything, clientID, update.Height).
		Return([32]byte{}, v2.ErrConsensusStateNotFound).Once()

	updates, err := env.prepareUpdate(ctx, update.Height)
	require.NoError(t, err)
	require.Len(t, updates, 1)

	decoded, err := besutest.DecodeUpdateClient(updates[0])
	require.NoError(t, err)
	assert.Equal(t, anchor, decoded.TrustedHeight)
}

func TestPreimageVerifiesOnceAndRejectsMismatch(t *testing.T) {
	ctx := context.Background()
	env := newFixtureEnv(t)
	update := env.fixture.NonAdjacentUpdate

	t.Run("rebuild then cache", func(t *testing.T) {
		env.expectHeader(t, update)
		env.host.EXPECT().GetBesuQBFTConsensusStateHash(mock.Anything, clientID, update.Height).
			Return(mustHash(t, update.ExpectedConsensusState()), nil).Once()

		state, err := env.gen.preimage(ctx, update.Height)
		require.NoError(t, err)
		assert.Equal(t, update.ExpectedConsensusState(), state)

		// second call is served from the verified cache: no further expectations
		again, err := env.gen.preimage(ctx, update.Height)
		require.NoError(t, err)
		assert.Equal(t, state, again)
	})

	t.Run("mismatch", func(t *testing.T) {
		other := newFixtureEnv(t)
		other.expectHeader(t, update)
		other.host.EXPECT().GetBesuQBFTConsensusStateHash(mock.Anything, clientID, update.Height).
			Return([32]byte{0xff}, nil).Once()

		_, err := other.gen.preimage(ctx, update.Height)
		require.ErrorContains(t, err, "hashes to")
	})
}

func TestPreimageRecoversFromStaleUnverifiedCache(t *testing.T) {
	env := newFixtureEnv(t)
	update := env.fixture.NonAdjacentUpdate
	want := update.ExpectedConsensusState()
	stale := want
	stale.StateRoot = common.Hash{0xff}
	env.gen.store(update.Height, stale, false)
	env.host.EXPECT().GetBesuQBFTConsensusStateHash(mock.Anything, clientID, update.Height).
		Return(mustHash(t, want), nil).Twice()

	_, err := env.gen.preimage(t.Context(), update.Height)
	require.ErrorContains(t, err, "hashes to")
	require.NotContains(t, env.gen.cache, update.Height)

	env.expectHeader(t, update)
	state, err := env.gen.preimage(t.Context(), update.Height)
	require.NoError(t, err)
	require.Equal(t, want, state)
	require.True(t, env.gen.cache[update.Height].verified)

	state, err = env.gen.preimage(t.Context(), update.Height)
	require.NoError(t, err)
	require.Equal(t, want, state)
}

// expectProofAt wires the counterparty reads PacketProofs performs at a
// fixture height, answering each requested slot with valueFor.
func (e *fixtureEnv) expectProofAt(
	t *testing.T,
	update besutest.UpdateFixture,
	valueFor func(slot [32]byte) (*big.Int, [][]byte),
) {
	e.counterparty.EXPECT().GetHeaderRLP(mock.Anything, update.Height).Return([]byte(update.HeaderRLP), nil).Once()
	e.counterparty.EXPECT().GetRouterProof(mock.Anything, update.Height, mock.Anything).RunAndReturn(
		func(_ context.Context, _ uint64, slots [][32]byte) (v2.AccountProof, error) {
			proof := v2.AccountProof{AccountProof: accountNodes(t, e.fixture.Membership)}
			for _, slot := range slots {
				value, nodes := valueFor(slot)
				proof.StorageProofs = append(
					proof.StorageProofs,
					v2.StorageProof{Key: slot, Value: value, Proof: nodes},
				)
			}

			return proof, nil
		}).Once()
}

func TestPacketProofs(t *testing.T) {
	ctx := context.Background()
	fixture := besutest.MustFixture(t)
	update := fixture.NonAdjacentUpdate
	preimage := update.ExpectedConsensusState()

	// Derive the packets from the fixture paths (clientId || kind || be64(seq))
	// so the proofs the mocks return are the ones a relayer would ask for.
	commitmentClient, commitmentSeq := splitPath(t, fixture.Membership.Path, 0x01)
	receiptClient, receiptSeq := splitPath(t, fixture.NonMembership.Path, 0x02)

	// commitments live under the sending client on the counterparty
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
	// receipts and acks live under the receiving client on the counterparty
	received := channeltypesv2.Packet{
		Sequence:          receiptSeq,
		SourceClient:      "some-other-client",
		DestinationClient: receiptClient,
	}
	commitment := new(big.Int).SetBytes(channeltypesv2.CommitPacket(sent))
	membershipNodes := proofNodes(t, fixture.Membership)
	nonMembershipNodes := proofNodes(t, fixture.NonMembership)

	t.Run("membership wraps the storage proof with the proof-height preimage", func(t *testing.T) {
		env := newFixtureEnv(t)
		env.expectProofAt(t, update, func([32]byte) (*big.Int, [][]byte) { return commitment, membershipNodes })

		proofs, err := env.gen.PacketProofs(
			ctx,
			update.Height,
			v2.ProofKindPacketCommitment,
			[]channeltypesv2.Packet{sent},
		)
		require.NoError(t, err)
		require.Len(t, proofs, 1)

		decoded, err := besutest.DecodeMembershipProof(proofs[0])
		require.NoError(t, err)
		assert.Equal(t, preimage, decoded.ConsensusStatePreimage)
		assert.Equal(t, accountNodes(t, fixture.Membership), decoded.AccountProofNodes)
		assert.Equal(t, membershipNodes, decoded.ProofNodes)
	})

	t.Run("commitment mismatch", func(t *testing.T) {
		env := newFixtureEnv(t)
		env.expectProofAt(t, update, func([32]byte) (*big.Int, [][]byte) { return big.NewInt(1), membershipNodes })

		_, err := env.gen.PacketProofs(ctx, update.Height, v2.ProofKindPacketCommitment, []channeltypesv2.Packet{sent})
		require.ErrorIs(t, err, ErrCommitmentMismatch)
	})

	t.Run("receipt absence uses the fixture exclusion proof", func(t *testing.T) {
		env := newFixtureEnv(t)
		env.expectProofAt(t, update, func(slot [32]byte) (*big.Int, [][]byte) {
			require.Equal(t, besu.CommitmentSlot(fixture.NonMembership.Path), common.Hash(slot))
			return big.NewInt(0), nonMembershipNodes
		})

		proofs, err := env.gen.PacketProofs(
			ctx,
			update.Height,
			v2.ProofKindReceiptAbsence,
			[]channeltypesv2.Packet{received},
		)
		require.NoError(t, err)
		require.Len(t, proofs, 1)

		decoded, err := besutest.DecodeMembershipProof(proofs[0])
		require.NoError(t, err)
		assert.Equal(t, preimage, decoded.ConsensusStatePreimage)
		assert.Equal(t, nonMembershipNodes, decoded.ProofNodes)
	})

	t.Run("receipt present fails absence", func(t *testing.T) {
		env := newFixtureEnv(t)
		env.expectProofAt(t, update, func([32]byte) (*big.Int, [][]byte) { return big.NewInt(1), [][]byte{{0x01}} })

		_, err := env.gen.PacketProofs(
			ctx,
			update.Height,
			v2.ProofKindReceiptAbsence,
			[]channeltypesv2.Packet{received},
		)
		require.ErrorIs(t, err, ErrReceiptExists)
	})

	t.Run("acknowledgement requires a value", func(t *testing.T) {
		env := newFixtureEnv(t)
		env.expectProofAt(t, update, func([32]byte) (*big.Int, [][]byte) { return big.NewInt(0), nil })

		_, err := env.gen.PacketProofs(
			ctx,
			update.Height,
			v2.ProofKindAcknowledgement,
			[]channeltypesv2.Packet{received},
		)
		require.ErrorIs(t, err, ErrAckMissing)
	})
}

func TestLatestProvableHeight(t *testing.T) {
	ctx := context.Background()

	t.Run("head when within drift", func(t *testing.T) {
		env := newFixtureEnv(t)
		base := int64(env.fixture.InitialTrustedTimestamp) //nolint:gosec // fixture timestamp
		env.host.EXPECT().GetBesuQBFTClientState(mock.Anything, clientID).
			Return(env.clientState(env.fixture.InitialTrustedHeight), nil).Once()
		env.expectInitialAnchor(t)
		env.host.EXPECT().GetBlockHeader(mock.Anything, uint64(v2.LatestBlock)).
			Return(v2.BlockHeader{Height: 500, Timestamp: time.Unix(base+35, 0)}, nil).Once()
		env.counterparty.EXPECT().GetBlockHeader(mock.Anything, uint64(v2.LatestBlock)).
			Return(v2.BlockHeader{Height: 120, Timestamp: time.Unix(base+45, 0)}, nil).Once()

		height, ts, err := env.gen.LatestProvableHeight(ctx)
		require.NoError(t, err)
		assert.Equal(t, uint64(120), height)
		assert.Equal(t, time.Unix(base+45, 0), ts)
	})

	t.Run("steps back under clock drift", func(t *testing.T) {
		env := newFixtureEnv(t)
		anchor := env.fixture.InitialTrustedHeight
		base := int64(env.fixture.InitialTrustedTimestamp) //nolint:gosec // fixture timestamp
		env.host.EXPECT().GetBesuQBFTClientState(mock.Anything, clientID).
			Return(env.clientState(anchor), nil).Once()
		env.expectInitialAnchor(t)
		// host is 15s behind the anchor with drift 15 -> only the anchor itself is admissible
		env.host.EXPECT().GetBlockHeader(mock.Anything, uint64(v2.LatestBlock)).
			Return(v2.BlockHeader{Height: 500, Timestamp: time.Unix(base-15, 0)}, nil).Once()
		env.counterparty.EXPECT().GetBlockHeader(mock.Anything, uint64(v2.LatestBlock)).
			Return(v2.BlockHeader{Height: anchor + 2, Timestamp: time.Unix(base+45, 0)}, nil).Once()
		env.counterparty.EXPECT().GetBlockHeader(mock.Anything, mock.Anything).RunAndReturn(
			func(_ context.Context, height uint64) (v2.BlockHeader, error) {
				ts := base + (int64(height)-int64(anchor))*5 //nolint:gosec // small test heights
				return v2.BlockHeader{Height: height, Timestamp: time.Unix(ts, 0)}, nil
			})

		height, ts, err := env.gen.LatestProvableHeight(ctx)
		require.NoError(t, err)
		assert.Equal(t, anchor, height)
		assert.Equal(t, time.Unix(base, 0).UTC(), ts)
	})
}

func TestLatestProvableHeightTrustingPeriod(t *testing.T) {
	for _, tc := range []struct {
		name        string
		hostOffset  time.Duration
		age, period uint64
		expired     bool
	}{
		{name: "host behind wall clock", hostOffset: -time.Hour, age: 119, period: 120},
		{name: "host ahead of wall clock", hostOffset: time.Hour, age: 120, period: 120, expired: true},
		{name: "one second before expiry", age: 119, period: 120},
		{name: "at expiry", age: 120, period: 120, expired: true},
		{name: "never expires", age: 3600},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newFixtureEnv(t)
			hostTime := time.Now().Add(tc.hostOffset).Truncate(time.Second)
			state := env.clientState(env.fixture.InitialTrustedHeight)
			state.TrustingPeriod = tc.period
			trusted := env.fixture.InitialConsensusState()
			trusted.Timestamp = uint64(hostTime.Unix()) - tc.age //nolint:gosec // current epoch seconds
			env.gen.store(state.LatestHeight, trusted, true)
			env.host.EXPECT().GetBesuQBFTClientState(mock.Anything, clientID).Return(state, nil).Once()
			env.host.EXPECT().GetBlockHeader(mock.Anything, uint64(v2.LatestBlock)).
				Return(v2.BlockHeader{Height: 500, Timestamp: hostTime}, nil).Once()
			if !tc.expired {
				env.counterparty.EXPECT().GetBlockHeader(mock.Anything, uint64(v2.LatestBlock)).
					Return(v2.BlockHeader{Height: 120, Timestamp: hostTime}, nil).Once()
			}
			height, timestamp, err := env.gen.LatestProvableHeight(context.Background())
			if tc.expired {
				require.ErrorIs(t, err, ErrClientExpired)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, uint64(120), height)
			assert.Equal(t, hostTime, timestamp)
		})
	}
}

func TestResolveChecksRouterAndClientType(t *testing.T) {
	ctx := context.Background()

	t.Run("router mismatch", func(t *testing.T) {
		env := newFixtureEnv(t)
		env.host.EXPECT().GetBesuQBFTClientState(mock.Anything, clientID).
			Return(env.clientState(env.fixture.InitialTrustedHeight), nil).Once()
		env.counterparty.EXPECT().ChainID().Return("besu-b").Maybe()

		err := env.gen.resolve(ctx, common.HexToAddress("0x1234").Hex())
		require.ErrorContains(t, err, "configured with router")
	})

	t.Run("not a besu client", func(t *testing.T) {
		env := newFixtureEnv(t)
		env.host.EXPECT().GetBesuQBFTClientState(mock.Anything, clientID).
			Return(besu.ClientState{}, errors.New("execution reverted")).Once()

		err := env.gen.resolve(ctx, env.fixture.RouterAddress.Hex())
		require.ErrorContains(t, err, "not a besu-qbft light client")
	})

	t.Run("warms the anchor", func(t *testing.T) {
		env := newFixtureEnv(t)
		env.host.EXPECT().GetBesuQBFTClientState(mock.Anything, clientID).
			Return(env.clientState(env.fixture.InitialTrustedHeight), nil).Once()
		env.expectInitialAnchor(t)

		require.NoError(t, env.gen.resolve(ctx, env.fixture.RouterAddress.Hex()))
		assert.True(t, env.gen.cache[env.fixture.InitialTrustedHeight].verified)
	})
}

// splitPath decomposes a raw commitment path into its client id and sequence,
// asserting the expected kind byte.
func splitPath(t *testing.T, path []byte, kind byte) (string, uint64) {
	t.Helper()
	require.Greater(t, len(path), 9)
	require.Equal(t, kind, path[len(path)-9])

	return string(path[:len(path)-9]), binary.BigEndian.Uint64(path[len(path)-8:])
}

// prepareUpdate calls StateProof with a host head shortly after the cached
// anchor's timestamp so trusting period and clock drift checks pass.
func (e *fixtureEnv) prepareUpdate(ctx context.Context, height uint64) ([][]byte, error) {
	timestamp := e.fixture.InitialTrustedTimestamp
	for _, entry := range e.gen.cache {
		timestamp = entry.state.Timestamp
		break
	}
	e.host.EXPECT().GetBlockHeader(mock.Anything, uint64(v2.LatestBlock)).
		Return(v2.BlockHeader{Timestamp: time.Unix(int64(timestamp+15000), 0)}, nil).Once()
	return e.gen.StateProof(ctx, height)
}

func TestStateProofLongHistoryBisects(t *testing.T) {
	env := newFixtureEnv(t)
	keys := besutest.Keys(8)
	const anchor = uint64(10)
	const bridge = anchor + 1000
	const target = bridge + 2
	trusted := besu.ConsensusState{Timestamp: 1700000010, Validators: besutest.Addresses(keys[:4])}
	env.gen.store(anchor, trusted, true)
	env.host.EXPECT().GetBesuQBFTClientState(mock.Anything, clientID).RunAndReturn(
		func(context.Context, string) (besu.ClientState, error) {
			state := env.clientState(anchor)
			state.TrustingPeriod = 0
			return state, nil
		}).Once()
	env.host.EXPECT().GetBlockHeader(mock.Anything, uint64(v2.LatestBlock)).
		Return(v2.BlockHeader{Timestamp: time.Unix(1700000000+int64(target), 0)}, nil).Once()
	fetched := 0
	env.counterparty.EXPECT().GetHeaderRLP(mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, h uint64) ([]byte, error) {
			fetched++
			set := keys[:4]
			if h == bridge {
				set = keys[2:6]
			} else if h > bridge {
				set = keys[4:8]
			}
			return sealedHeader(t, env.fixture.AdjacentUpdate.HeaderRLP, h, set).RLP, nil
		})
	updates, err := env.gen.StateProof(t.Context(), target)
	require.NoError(t, err)
	require.Len(t, updates, 2)
	// The bisection finds the newest header the old set still accepts, and
	// that bridge reaches the target directly.
	trustedHeight := anchor
	for i, want := range []uint64{bridge, target} {
		update, err := besutest.DecodeUpdateClient(updates[i])
		require.NoError(t, err)
		header, err := besu.ParseHeader(update.HeaderRLP)
		require.NoError(t, err)
		require.Equal(t, want, header.Height)
		require.Equal(t, trustedHeight, update.TrustedHeight)
		signers, err := header.Signers()
		require.NoError(t, err)
		require.NoError(t, besu.CheckUpdate(header, signers, trusted))
		trustedHeight = want
		trusted = besu.ConsensusState{Timestamp: header.Timestamp, Validators: header.Validators}
	}
	require.Less(t, fetched, 20, "bisection must not walk the history linearly")
}

func TestPacketProofsShareSlot(t *testing.T) {
	env := newFixtureEnv(t)
	update := env.fixture.NonAdjacentUpdate
	packet := channeltypesv2.Packet{Sequence: 7, DestinationClient: "dst"}
	slots, _, err := packetSlots(v2.ProofKindReceiptAbsence, []channeltypesv2.Packet{packet, packet})
	require.NoError(t, err)
	require.Len(t, slots, 1)
	env.counterparty.EXPECT().GetHeaderRLP(mock.Anything, update.Height).Return(update.HeaderRLP, nil).Once()
	env.counterparty.EXPECT().GetRouterProof(mock.Anything, update.Height, slots).Return(v2.AccountProof{
		AccountProof: accountNodes(t, env.fixture.NonMembership),
		StorageProofs: []v2.StorageProof{
			{Key: slots[0], Value: big.NewInt(0), Proof: proofNodes(t, env.fixture.NonMembership)},
		},
	}, nil).Once()

	proofs, err := env.gen.PacketProofs(
		t.Context(), update.Height, v2.ProofKindReceiptAbsence, []channeltypesv2.Packet{packet, packet},
	)
	require.NoError(t, err)
	require.Len(t, proofs, 2)
	// The first proof carries the account proof; the rest reuse the storage
	// root the contract caches for the transaction.
	first, err := besutest.DecodeMembershipProof(proofs[0])
	require.NoError(t, err)
	second, err := besutest.DecodeMembershipProof(proofs[1])
	require.NoError(t, err)
	require.Equal(t, accountNodes(t, env.fixture.NonMembership), first.AccountProofNodes)
	require.Empty(t, second.AccountProofNodes)
	require.Equal(t, first.ProofNodes, second.ProofNodes)
	require.Equal(t, first.ConsensusStatePreimage, second.ConsensusStatePreimage)
}
