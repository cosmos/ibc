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

func accountNodes(t *testing.T, update besutest.UpdateFixture) [][]byte {
	t.Helper()

	nodes, err := update.AccountProofNodes()
	require.NoError(t, err)

	return nodes
}

func proofNodes(t *testing.T, m besutest.MembershipFixture) [][]byte {
	t.Helper()

	nodes, err := m.ProofNodes()
	require.NoError(t, err)

	return nodes
}

// expectSnapshot wires the counterparty reads snapshot(height) performs for a
// fixture update.
func (e *fixtureEnv) expectSnapshot(t *testing.T, update besutest.UpdateFixture) {
	t.Helper()

	e.counterparty.EXPECT().GetHeaderRLP(mock.Anything, update.Height).Return([]byte(update.HeaderRLP), nil)
	e.counterparty.EXPECT().GetRouterProof(mock.Anything, update.Height, mock.Anything).Return(v2.AccountProof{
		StorageRoot:  update.ExpectedStorageRoot,
		AccountProof: accountNodes(t, update),
	}, nil)
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
	env.expectSnapshot(t, update)

	updates, err := env.prepareUpdate(ctx, update.Height)
	require.NoError(t, err)
	require.Len(t, updates, 1)

	decoded, err := besutest.DecodeUpdateClient(updates[0])
	require.NoError(t, err)
	assert.Equal(t, []byte(update.HeaderRLP), decoded.HeaderRLP)
	assert.Equal(t, env.fixture.InitialTrustedHeight, decoded.TrustedHeight)
	assert.Equal(t, env.fixture.InitialConsensusState(), decoded.ConsensusStatePreimage)
	assert.Equal(t, accountNodes(t, update), decoded.AccountProof)

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

func TestPrepareValidatorTurnoverCheckpoints(t *testing.T) {
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
	nodes := accountNodes(t, env.fixture.NonAdjacentUpdate)
	for _, next := range []uint64{12, 14, 15} {
		env.gen.store(height, trusted, true) // simulate confirmation, not just submission
		env.host.EXPECT().GetBesuQBFTClientState(mock.Anything, clientID).Return(env.clientState(height), nil).Once()
		env.counterparty.EXPECT().GetRouterProof(mock.Anything, next, [][32]byte(nil)).
			Return(v2.AccountProof{StorageRoot: common.BigToHash(new(big.Int).SetUint64(next)), AccountProof: nodes}, nil).
			Once()
		updates, err := env.prepareUpdate(t.Context(), 15)
		require.NoError(t, err)
		require.Len(t, updates, 1)
		update, err := besutest.DecodeUpdateClient(updates[0])
		require.NoError(t, err)
		require.Equal(t, height, update.TrustedHeight)
		require.Equal(t, trusted, update.ConsensusStatePreimage)
		require.Equal(t, headers[next].RLP, update.HeaderRLP)
		signers, err := headers[next].Signers()
		require.NoError(t, err)
		require.NoError(t, besu.CheckUpdate(headers[next], signers, trusted))
		height = next
		trusted = besu.ConsensusState{
			Timestamp: headers[next].Timestamp, Validators: headers[next].Validators,
			StorageRoot: common.BigToHash(new(big.Int).SetUint64(next)),
		}
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

func TestPrepareScanBudgetReturnsCheckpoint(t *testing.T) {
	env := newFixtureEnv(t)
	keys := besutest.Keys(2)
	const anchor, target = uint64(10), uint64(10 + maxScan + 1)
	env.gen.store(anchor, besu.ConsensusState{Timestamp: 1700000010, Validators: besutest.Addresses(keys[:1])}, true)
	env.host.EXPECT().GetBesuQBFTClientState(mock.Anything, clientID).Return(env.clientState(anchor), nil).Once()
	final := sealedHeader(t, env.fixture.AdjacentUpdate.HeaderRLP, target, keys[1:])
	env.counterparty.EXPECT().GetHeaderRLP(mock.Anything, target).Return(final.RLP, nil).Once()
	env.counterparty.EXPECT().GetRouterProof(mock.Anything, target-1, [][32]byte(nil)).
		Return(v2.AccountProof{}, nil).Once()
	next := anchor + 1
	env.counterparty.EXPECT().GetHeaderRLP(mock.Anything, mock.MatchedBy(func(height uint64) bool {
		return height > anchor && height < target
	})).RunAndReturn(func(_ context.Context, height uint64) ([]byte, error) {
		require.Equal(t, next, height)
		next++
		return sealedHeader(t, env.fixture.AdjacentUpdate.HeaderRLP, height, keys[:1]).RLP, nil
	}).Times(maxScan)

	updates, err := env.prepareUpdate(context.Background(), target)
	require.NoError(t, err)
	require.Len(t, updates, 1)
	decoded, err := besutest.DecodeUpdateClient(updates[0])
	require.NoError(t, err)
	header, err := besu.ParseHeader(decoded.HeaderRLP)
	require.NoError(t, err)
	require.Equal(t, target-1, header.Height)
	assert.Equal(t, target, next)
}

func TestStateProofTargetAlreadyStored(t *testing.T) {
	ctx := context.Background()
	env := newFixtureEnv(t)
	update := env.fixture.NonAdjacentUpdate

	env.host.EXPECT().GetBesuQBFTClientState(mock.Anything, clientID).Return(env.clientState(update.Height), nil).Once()
	// trusted anchor is the update height itself
	env.expectSnapshot(t, update)
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
			env.expectSnapshot(t, update)

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
	env.expectSnapshot(t, update)
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
	env.expectSnapshot(t, update)
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
		env.expectSnapshot(t, update)
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
		other.expectSnapshot(t, update)
		other.host.EXPECT().GetBesuQBFTConsensusStateHash(mock.Anything, clientID, update.Height).
			Return([32]byte{0xff}, nil).Once()

		_, err := other.gen.preimage(ctx, update.Height)
		require.ErrorContains(t, err, "hashes to")
	})
}

// expectProofAt wires the counterparty reads PacketProofs performs at a
// fixture height, answering each requested slot with valueFor.
func (e *fixtureEnv) expectProofAt(
	update besutest.UpdateFixture,
	valueFor func(slot [32]byte) (*big.Int, [][]byte),
) {
	e.counterparty.EXPECT().GetHeaderRLP(mock.Anything, update.Height).Return([]byte(update.HeaderRLP), nil).Once()
	e.counterparty.EXPECT().GetRouterProof(mock.Anything, update.Height, mock.Anything).RunAndReturn(
		func(_ context.Context, _ uint64, slots [][32]byte) (v2.AccountProof, error) {
			proof := v2.AccountProof{StorageRoot: update.ExpectedStorageRoot}
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
		env.expectProofAt(update, func([32]byte) (*big.Int, [][]byte) { return commitment, membershipNodes })

		proofs, err := env.packetProofs(
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
		assert.Equal(t, membershipNodes, decoded.ProofNodes)
	})

	t.Run("commitment mismatch", func(t *testing.T) {
		env := newFixtureEnv(t)
		env.expectProofAt(update, func([32]byte) (*big.Int, [][]byte) { return big.NewInt(1), membershipNodes })

		_, err := env.packetProofs(ctx, update.Height, v2.ProofKindPacketCommitment, []channeltypesv2.Packet{sent})
		require.ErrorIs(t, err, ErrCommitmentMismatch)
	})

	t.Run("receipt absence uses the fixture exclusion proof", func(t *testing.T) {
		env := newFixtureEnv(t)
		env.expectProofAt(update, func(slot [32]byte) (*big.Int, [][]byte) {
			require.Equal(t, besu.CommitmentSlot(fixture.NonMembership.Path), common.Hash(slot))
			return big.NewInt(0), nonMembershipNodes
		})

		proofs, err := env.packetProofs(
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
		env.expectProofAt(update, func([32]byte) (*big.Int, [][]byte) { return big.NewInt(1), [][]byte{{0x01}} })

		_, err := env.packetProofs(
			ctx,
			update.Height,
			v2.ProofKindReceiptAbsence,
			[]channeltypesv2.Packet{received},
		)
		require.ErrorIs(t, err, ErrReceiptExists)
	})

	t.Run("acknowledgement requires a value", func(t *testing.T) {
		env := newFixtureEnv(t)
		env.expectProofAt(update, func([32]byte) (*big.Int, [][]byte) { return big.NewInt(0), nil })

		_, err := env.packetProofs(
			ctx,
			update.Height,
			v2.ProofKindAcknowledgement,
			[]channeltypesv2.Packet{received},
		)
		require.ErrorIs(t, err, ErrAckMissing)
	})

	t.Run("duplicate slots are requested once", func(t *testing.T) {
		env := newFixtureEnv(t)
		env.expectProofAt(update, func([32]byte) (*big.Int, [][]byte) { return big.NewInt(0), nonMembershipNodes })

		proofs, err := env.packetProofs(
			ctx, update.Height, v2.ProofKindReceiptAbsence, []channeltypesv2.Packet{received, received},
		)
		require.NoError(t, err)
		require.Len(t, proofs, 2)
		assert.Equal(t, proofs[0], proofs[1])
	})
}

func TestLatestProvableHeight(t *testing.T) {
	ctx := context.Background()

	t.Run("head when within drift", func(t *testing.T) {
		env := newFixtureEnv(t)
		env.host.EXPECT().GetBesuQBFTClientState(mock.Anything, clientID).
			Return(env.clientState(env.fixture.InitialTrustedHeight), nil).Once()
		env.expectInitialAnchor(t)
		env.host.EXPECT().GetBlockHeader(mock.Anything, uint64(v2.LatestBlock)).
			Return(v2.BlockHeader{Height: 500, Timestamp: time.Unix(1788192450, 0)}, nil).Once()
		env.counterparty.EXPECT().GetBlockHeader(mock.Anything, uint64(v2.LatestBlock)).
			Return(v2.BlockHeader{Height: 120, Timestamp: time.Unix(1788192460, 0)}, nil).Once()

		height, ts, err := env.gen.LatestProvableHeight(ctx)
		require.NoError(t, err)
		assert.Equal(t, uint64(120), height)
		assert.Equal(t, time.Unix(1788192460, 0), ts)
	})

	t.Run("steps back under clock drift", func(t *testing.T) {
		env := newFixtureEnv(t)
		env.host.EXPECT().GetBesuQBFTClientState(mock.Anything, clientID).
			Return(env.clientState(env.fixture.InitialTrustedHeight), nil).Once()
		env.expectInitialAnchor(t)
		// host is at t=1000, drift 15 -> headers newer than 1015 are inadmissible
		env.host.EXPECT().GetBlockHeader(mock.Anything, uint64(v2.LatestBlock)).
			Return(v2.BlockHeader{Height: 500, Timestamp: time.Unix(1788192400, 0)}, nil).Once()
		env.counterparty.EXPECT().GetBlockHeader(mock.Anything, uint64(v2.LatestBlock)).
			Return(v2.BlockHeader{Height: 120, Timestamp: time.Unix(1788192460, 0)}, nil).Once()
		env.counterparty.EXPECT().GetBlockHeader(mock.Anything, mock.Anything).RunAndReturn(
			func(_ context.Context, height uint64) (v2.BlockHeader, error) {
				ts := int64(1788192415) + (int64(height)-118)*5
				return v2.BlockHeader{Height: height, Timestamp: time.Unix(ts, 0)}, nil
			})

		height, ts, err := env.gen.LatestProvableHeight(ctx)
		require.NoError(t, err)
		assert.Equal(t, uint64(118), height)
		assert.Equal(t, time.Unix(1788192415, 0), ts)
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

func TestStoreBoundsCacheWithoutAnchor(t *testing.T) {
	env := newFixtureEnv(t)
	state := env.fixture.InitialConsensusState()
	for height := uint64(1); height <= 2*maxCached; height++ {
		env.gen.store(height, state, false)
		require.LessOrEqual(t, len(env.gen.cache), maxCached)
	}
	require.Len(t, env.gen.cache, maxCached)
}

func TestStoreBoundsCacheAndRetainsAnchor(t *testing.T) {
	for _, anchor := range []uint64{10, 2000} {
		env := newFixtureEnv(t)
		state := env.fixture.InitialConsensusState()
		env.gen.store(anchor, state, true)
		for height := anchor + 1; height < anchor+2*maxCached; height++ {
			env.gen.store(height, state, false)
			require.LessOrEqual(t, len(env.gen.cache), maxCached)
			require.Equal(t, cacheEntry{state: state, verified: true}, env.gen.cache[anchor])
		}
		require.Len(t, env.gen.cache, maxCached)
		require.NotContains(t, env.gen.cache, anchor+1)
		require.Contains(t, env.gen.cache, anchor+2*maxCached-1)

		// Backfills must not displace the anchor or newer cached heights.
		env.gen.store(anchor-1, state, false)
		require.NotContains(t, env.gen.cache, anchor-1)
		require.Len(t, env.gen.cache, maxCached)

		// Advancing the anchor makes its predecessor eligible for eviction.
		newAnchor := anchor + 2*maxCached
		env.gen.store(newAnchor, state, true)
		require.NotContains(t, env.gen.cache, anchor)
		require.Equal(t, cacheEntry{state: state, verified: true}, env.gen.cache[newAnchor])
		require.Len(t, env.gen.cache, maxCached)
	}
}

// splitPath decomposes a raw commitment path into its client id and sequence,
// asserting the expected kind byte.
func splitPath(t *testing.T, path []byte, kind byte) (string, uint64) {
	t.Helper()
	require.Greater(t, len(path), 9)
	require.Equal(t, kind, path[len(path)-9])

	return string(path[:len(path)-9]), binary.BigEndian.Uint64(path[len(path)-8:])
}

// prepareUpdate isolates the update half of Prepare with an empty packet batch.
func (e *fixtureEnv) prepareUpdate(ctx context.Context, height uint64) ([][]byte, error) {
	timestamp := e.fixture.InitialTrustedTimestamp
	for _, entry := range e.gen.cache {
		timestamp = entry.state.Timestamp
		break
	}
	e.host.EXPECT().GetBlockHeader(mock.Anything, uint64(v2.LatestBlock)).
		Return(v2.BlockHeader{Timestamp: time.Unix(int64(timestamp+15000), 0)}, nil).Once()
	result, err := e.gen.Prepare(ctx, height, v2.ProofKindPacketCommitment, nil)
	if err != nil {
		return nil, err
	}
	if result.Ready == nil {
		return [][]byte{result.Advance}, nil
	}
	if len(result.Ready.Update) == 0 {
		return nil, nil
	}
	return [][]byte{result.Ready.Update}, nil
}

func (e *fixtureEnv) packetProofs(
	ctx context.Context,
	height uint64,
	kind v2.ProofKind,
	packets []channeltypesv2.Packet,
) ([][]byte, error) {
	slots, indices, err := packetSlots(kind, packets)
	if err != nil {
		return nil, err
	}
	snap, err := e.gen.snapshot(ctx, height, slots)
	if err != nil {
		return nil, err
	}
	return packetProofs(snap, kind, packets, indices)
}

func TestPrepareLongHistoryResumesFromConfirmedState(t *testing.T) {
	env := newFixtureEnv(t)
	keys := besutest.Keys(8)
	const anchor = uint64(10)
	const bridge = anchor + maxScan + 1
	const target = bridge + 2
	latest := anchor
	trusted := besu.ConsensusState{Timestamp: 1700000010, Validators: besutest.Addresses(keys[:4])}
	confirmed := map[uint64]besu.ConsensusState{anchor: trusted}
	env.gen.store(anchor, trusted, true)
	env.host.EXPECT().GetBesuQBFTClientState(mock.Anything, clientID).RunAndReturn(
		func(context.Context, string) (besu.ClientState, error) {
			state := env.clientState(latest)
			state.TrustingPeriod = 0
			return state, nil
		})
	env.host.EXPECT().GetBesuQBFTConsensusStateHash(mock.Anything, clientID, mock.Anything).RunAndReturn(
		func(_ context.Context, _ string, h uint64) ([32]byte, error) {
			state, ok := confirmed[h]
			require.True(t, ok, "unconfirmed height became an anchor")
			return mustHash(t, state), nil
		})
	env.host.EXPECT().GetBlockHeader(mock.Anything, uint64(v2.LatestBlock)).
		Return(v2.BlockHeader{Timestamp: time.Unix(1700000000+int64(target), 0)}, nil)
	env.counterparty.EXPECT().GetHeaderRLP(mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, h uint64) ([]byte, error) {
			set := keys[:4]
			if h == bridge {
				set = keys[2:6]
			} else if h > bridge {
				set = keys[4:8]
			}
			return sealedHeader(t, env.fixture.AdjacentUpdate.HeaderRLP, h, set).RLP, nil
		})
	env.counterparty.EXPECT().GetRouterProof(mock.Anything, mock.Anything, [][32]byte(nil)).
		Return(v2.AccountProof{AccountProof: accountNodes(t, env.fixture.NonAdjacentUpdate)}, nil).Times(3)
	for i, want := range []uint64{bridge - 1, bridge, target} {
		result, err := env.gen.Prepare(t.Context(), target, v2.ProofKindPacketCommitment, nil)
		require.NoError(t, err)
		payload := result.Advance
		if i == 2 {
			require.NotNil(t, result.Ready)
			payload = result.Ready.Update
		} else {
			require.Nil(t, result.Ready)
		}
		update, err := besutest.DecodeUpdateClient(payload)
		require.NoError(t, err)
		header, err := besu.ParseHeader(update.HeaderRLP)
		require.NoError(t, err)
		require.Equal(t, want, header.Height)
		require.Equal(t, latest, update.TrustedHeight)
		signers, err := header.Signers()
		require.NoError(t, err)
		require.NoError(t, besu.CheckUpdate(header, signers, confirmed[latest]))
		require.False(t, env.gen.cache[want].verified)
		latest = want // only successful on-chain confirmation changes the source of truth
		confirmed[want] = besu.ConsensusState{Timestamp: header.Timestamp, Validators: header.Validators}
	}
}

func TestPrepareSharesTargetSnapshot(t *testing.T) {
	env := newFixtureEnv(t)
	update := env.fixture.NonAdjacentUpdate
	env.expectInitialAnchor(t)
	env.host.EXPECT().GetBesuQBFTClientState(mock.Anything, clientID).
		Return(env.clientState(env.fixture.InitialTrustedHeight), nil).Once()
	env.host.EXPECT().GetBlockHeader(mock.Anything, uint64(v2.LatestBlock)).
		Return(v2.BlockHeader{Timestamp: time.Unix(int64(update.ExpectedConsensusState().Timestamp), 0)}, nil).Once()
	packet := channeltypesv2.Packet{Sequence: 7, DestinationClient: "dst"}
	slots, _, err := packetSlots(v2.ProofKindReceiptAbsence, []channeltypesv2.Packet{packet, packet})
	require.NoError(t, err)
	require.Len(t, slots, 1)
	env.counterparty.EXPECT().GetHeaderRLP(mock.Anything, update.Height).Return(update.HeaderRLP, nil).Once()
	env.counterparty.EXPECT().GetRouterProof(mock.Anything, update.Height, slots).Return(v2.AccountProof{
		StorageRoot:  update.ExpectedStorageRoot,
		AccountProof: accountNodes(t, update),
		StorageProofs: []v2.StorageProof{
			{Key: slots[0], Value: big.NewInt(0), Proof: proofNodes(t, env.fixture.NonMembership)},
		},
	}, nil).Once()
	result, err := env.gen.Prepare(
		t.Context(),
		update.Height,
		v2.ProofKindReceiptAbsence,
		[]channeltypesv2.Packet{packet, packet},
	)
	require.NoError(t, err)
	require.NoError(t, result.Validate(2))
	require.NotEmpty(t, result.Ready.Update)
	require.Equal(t, result.Ready.PacketProofs[0], result.Ready.PacketProofs[1])
	require.Equal(t, update.ExpectedConsensusState(), env.gen.cache[update.Height].state)
}
