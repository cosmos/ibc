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

type hashResult struct {
	hash [32]byte
	err  error
}

type fakeChain struct {
	id             string
	latest         *v2.BlockHeader
	headers        map[uint64]v2.BlockHeader
	sealed         map[uint64]*besu.Header
	clientState    besumsgs.IBesuLightClientMsgsClientState
	clientStateErr error
	hashes         map[uint64]hashResult
	proof          func(height uint64, slots [][32]byte) (evm.AccountProof, error)
}

func (f *fakeChain) ChainID() string { return f.id }

func (f *fakeChain) GetBlockHeader(_ context.Context, height uint64) (v2.BlockHeader, error) {
	if height == v2.LatestBlock {
		if f.latest != nil {
			return *f.latest, nil
		}
		return v2.BlockHeader{}, errors.New("no latest header")
	}
	if h, ok := f.headers[height]; ok {
		return h, nil
	}
	if s, ok := f.sealed[height]; ok {
		return v2.BlockHeader{Height: s.Height, Timestamp: time.Unix(int64(s.Timestamp), 0).UTC()}, nil
	}
	return v2.BlockHeader{}, fmt.Errorf("no block header %d", height)
}

func (f *fakeChain) SealedHeader(_ context.Context, height uint64) (*besu.Header, error) {
	h, ok := f.sealed[height]
	if !ok {
		return nil, fmt.Errorf("no sealed header %d", height)
	}
	return h, nil
}

func (f *fakeChain) GetRouterProof(_ context.Context, height uint64, slots [][32]byte) (evm.AccountProof, error) {
	if f.proof == nil {
		return evm.AccountProof{}, fmt.Errorf("no router proof at %d", height)
	}
	return f.proof(height, slots)
}

func (f *fakeChain) GetBesuQBFTClientState(context.Context, string) (besumsgs.IBesuLightClientMsgsClientState, error) {
	return f.clientState, f.clientStateErr
}

func (f *fakeChain) GetBesuQBFTConsensusStateHash(_ context.Context, _ string, height uint64) ([32]byte, error) {
	r, ok := f.hashes[height]
	if !ok {
		return [32]byte{}, fmt.Errorf("no consensus hash at %d", height)
	}
	return r.hash, r.err
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
		host:         &fakeChain{id: "host", sealed: map[uint64]*besu.Header{}, hashes: map[uint64]hashResult{}},
		counterparty: &fakeChain{id: "besu-b", sealed: map[uint64]*besu.Header{}, hashes: map[uint64]hashResult{}},
	}
	env.gen = New(env.host, env.counterparty, clientID)
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

func mustHash(t *testing.T, state besumsgs.IBesuLightClientMsgsConsensusState) [32]byte {
	t.Helper()
	hash, err := besu.HashConsensusState(state)
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

func consensusHeader(height uint64, state besumsgs.IBesuLightClientMsgsConsensusState) *besu.Header {
	return &besu.Header{
		Height:     height,
		Timestamp:  state.Timestamp,
		StateRoot:  state.StateRoot,
		Validators: state.Validators,
	}
}

func parsedUpdate(t *testing.T, update besutest.UpdateFixture) *besu.Header {
	t.Helper()
	header, err := besu.ParseHeader(update.HeaderRLP)
	require.NoError(t, err)
	return header
}

func (e *fixtureEnv) setAnchor(t *testing.T, height uint64, state besumsgs.IBesuLightClientMsgsConsensusState) {
	t.Helper()
	e.host.clientState = e.clientState(height)
	e.counterparty.sealed[height] = consensusHeader(height, state)
	e.host.hashes[height] = hashResult{hash: mustHash(t, state)}
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

	proof, err := env.prepareUpdate(ctx, update.Height)
	require.NoError(t, err)
	require.NotEmpty(t, proof)

	decoded, err := besumsgs.NewBindings().UnpackUpdateClient(proof)
	require.NoError(t, err)
	assert.Equal(t, []byte(update.HeaderRLP), decoded.HeaderRlp)
	assert.Equal(
		t,
		besumsgs.IICS02ClientMsgsHeight{RevisionHeight: env.fixture.InitialTrustedHeight},
		decoded.TrustedHeight,
	)
	assert.Equal(t, env.fixture.InitialConsensusState(), decoded.ConsensusStatePreimage)
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

func TestClientUpdatePayloadRejectsValidatorTurnoverRequiringIntermediateUpdates(t *testing.T) {
	env := newFixtureEnv(t)
	keys := besutest.Keys(8)
	trusted := besumsgs.IBesuLightClientMsgsConsensusState{
		Timestamp:  1700000010,
		Validators: besutest.Addresses(keys[:4]),
	}
	env.setAnchor(t, 10, trusted)
	env.counterparty.sealed[12] = sealedHeader(t, env.fixture.AdjacentUpdate.HeaderRLP, 12, keys[4:])

	update, err := env.prepareUpdate(t.Context(), 12)
	require.ErrorIs(t, err, besu.ErrInsufficientOverlap)
	require.ErrorContains(t, err, "intermediate updates are not supported")
	require.Empty(t, update)
}

func TestClientUpdatePayloadValidatorTurnoverWithSufficientOverlap(t *testing.T) {
	env := newFixtureEnv(t)
	keys := besutest.Keys(6)
	trusted := besumsgs.IBesuLightClientMsgsConsensusState{
		Timestamp:  1700000010,
		Validators: besutest.Addresses(keys[:4]),
	}
	env.setAnchor(t, 10, trusted)
	header := sealedHeader(t, env.fixture.AdjacentUpdate.HeaderRLP, 12, keys[2:])
	env.counterparty.sealed[12] = header

	proof, err := env.prepareUpdate(t.Context(), 12)
	require.NoError(t, err)
	update, err := besumsgs.NewBindings().UnpackUpdateClient(proof)
	require.NoError(t, err)
	require.Equal(t, besumsgs.IICS02ClientMsgsHeight{RevisionHeight: uint64(10)}, update.TrustedHeight)
	require.Equal(t, trusted, update.ConsensusStatePreimage)
	require.Equal(t, header.RLP, update.HeaderRlp)
}

func TestClientUpdatePayloadTargetAlreadyStored(t *testing.T) {
	env := newFixtureEnv(t)
	update := env.fixture.NonAdjacentUpdate
	env.setAnchor(t, update.Height, update.ExpectedConsensusState())
	env.counterparty.sealed[update.Height] = parsedUpdate(t, update)

	proof, err := env.prepareUpdate(t.Context(), update.Height)
	require.NoError(t, err)
	assert.Nil(t, proof, "no update needed")
}

func TestClientUpdatePayloadTargetConflicts(t *testing.T) {
	env := newFixtureEnv(t)
	update := env.fixture.NonAdjacentUpdate
	tampered := update.ExpectedConsensusState()
	tampered.Timestamp++
	env.setAnchor(t, update.Height+1, tampered)
	env.counterparty.sealed[update.Height] = parsedUpdate(t, update)
	env.host.hashes[update.Height] = hashResult{hash: mustHash(t, tampered)}

	_, err := env.prepareUpdate(t.Context(), update.Height)
	require.ErrorIs(t, err, ErrConflictingConsensusState)
}

func TestClientUpdatePayloadBackfillBelowTrusted(t *testing.T) {
	env := newFixtureEnv(t)
	update := env.fixture.NonAdjacentUpdate
	anchor := update.Height + 5
	env.setAnchor(t, anchor, env.fixture.InitialConsensusState())
	env.counterparty.sealed[update.Height] = parsedUpdate(t, update)
	env.host.hashes[update.Height] = hashResult{err: evm.ErrConsensusStateNotFound}

	proof, err := env.prepareUpdate(t.Context(), update.Height)
	require.NoError(t, err)
	require.NotEmpty(t, proof)

	decoded, err := besumsgs.NewBindings().UnpackUpdateClient(proof)
	require.NoError(t, err)
	assert.Equal(t, besumsgs.IICS02ClientMsgsHeight{RevisionHeight: anchor}, decoded.TrustedHeight)
}

func TestPreimageRejectsMismatch(t *testing.T) {
	env := newFixtureEnv(t)
	update := env.fixture.NonAdjacentUpdate
	env.counterparty.sealed[update.Height] = parsedUpdate(t, update)
	env.host.hashes[update.Height] = hashResult{hash: [32]byte{0xff}}

	_, err := env.gen.preimage(t.Context(), update.Height)
	require.ErrorContains(t, err, "hashes to")
}

func (e *fixtureEnv) expectProofAt(
	t *testing.T,
	update besutest.UpdateFixture,
	valueFor func(slot [32]byte) (*big.Int, [][]byte),
) {
	t.Helper()
	e.counterparty.sealed[update.Height] = parsedUpdate(t, update)
	e.counterparty.proof = func(_ uint64, slots [][32]byte) (evm.AccountProof, error) {
		proof := evm.AccountProof{AccountProof: accountNodes(t, e.fixture.Membership)}
		for _, slot := range slots {
			value, nodes := valueFor(slot)
			proof.StorageProofs = append(proof.StorageProofs, evm.StorageProof{Key: slot, Value: value, Proof: nodes})
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

		decoded, err := besumsgs.NewBindings().UnpackMembershipProof(proofs[0])
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

		decoded, err := besumsgs.NewBindings().UnpackMembershipProof(proofs[0])
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
		env.expectInitialAnchor(t)
		env.host.latest = &v2.BlockHeader{Height: 500, Timestamp: time.Unix(base+35, 0)}
		env.counterparty.latest = &v2.BlockHeader{Height: 120, Timestamp: time.Unix(base+45, 0)}

		height, ts, err := env.gen.LatestProvableHeight(ctx)
		require.NoError(t, err)
		assert.Equal(t, uint64(120), height)
		assert.Equal(t, time.Unix(base+45, 0), ts)
	})

	t.Run("steps back under clock drift", func(t *testing.T) {
		env := newFixtureEnv(t)
		anchor := env.fixture.InitialTrustedHeight
		base := int64(env.fixture.InitialTrustedTimestamp) //nolint:gosec // fixture timestamp
		env.expectInitialAnchor(t)
		env.host.latest = &v2.BlockHeader{Height: 500, Timestamp: time.Unix(base-15, 0)}
		env.counterparty.latest = &v2.BlockHeader{Height: anchor + 2, Timestamp: time.Unix(base+45, 0)}
		env.counterparty.headers = map[uint64]v2.BlockHeader{}
		for height := anchor; height <= anchor+2; height++ {
			ts := base + (int64(height)-int64(anchor))*5 //nolint:gosec // small test heights
			env.counterparty.headers[height] = v2.BlockHeader{Height: height, Timestamp: time.Unix(ts, 0)}
		}

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
			env.setAnchor(t, state.LatestHeight.RevisionHeight, trusted)
			env.host.clientState = state
			env.host.latest = &v2.BlockHeader{Height: 500, Timestamp: hostTime}
			if !tc.expired {
				env.counterparty.latest = &v2.BlockHeader{Height: 120, Timestamp: hostTime}
			}
			height, timestamp, err := env.gen.LatestProvableHeight(context.Background())
			if tc.expired {
				require.NoError(t, err)
				assert.Equal(t, state.LatestHeight.RevisionHeight, height)
				assert.Equal(t, time.Unix(int64(trusted.Timestamp), 0).UTC(), timestamp)
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
		env.host.clientState = env.clientState(env.fixture.InitialTrustedHeight)
		err := env.gen.resolve(ctx, common.HexToAddress("0x1234").Hex())
		require.ErrorContains(t, err, "configured with router")
	})

	t.Run("not a besu client", func(t *testing.T) {
		env := newFixtureEnv(t)
		env.host.clientStateErr = errors.New("execution reverted")
		err := env.gen.resolve(ctx, env.fixture.RouterAddress.Hex())
		require.ErrorContains(t, err, "not a besu-qbft light client")
	})

	t.Run("warms the anchor", func(t *testing.T) {
		env := newFixtureEnv(t)
		env.expectInitialAnchor(t)
		require.NoError(t, env.gen.resolve(ctx, env.fixture.RouterAddress.Hex()))
	})
}

func splitPath(t *testing.T, path []byte, kind byte) (string, uint64) {
	t.Helper()
	require.Greater(t, len(path), 9)
	require.Equal(t, kind, path[len(path)-9])
	return string(path[:len(path)-9]), binary.BigEndian.Uint64(path[len(path)-8:])
}

func (e *fixtureEnv) prepareUpdate(ctx context.Context, height uint64) ([]byte, error) {
	timestamp := e.fixture.InitialTrustedTimestamp
	if sealed, ok := e.counterparty.sealed[e.host.clientState.LatestHeight.RevisionHeight]; ok {
		timestamp = sealed.Timestamp
	}
	e.host.latest = &v2.BlockHeader{Timestamp: time.Unix(int64(timestamp+15000), 0)} //nolint:gosec // test offset
	return e.gen.ClientUpdatePayload(ctx, height)
}

func TestPacketProofsShareSlot(t *testing.T) {
	env := newFixtureEnv(t)
	update := env.fixture.NonAdjacentUpdate
	packet := channeltypesv2.Packet{Sequence: 7, DestinationClient: "dst"}
	slots, _, err := packetSlots(v2.ProofKindReceiptAbsence, []channeltypesv2.Packet{packet, packet})
	require.NoError(t, err)
	require.Len(t, slots, 1)
	env.counterparty.sealed[update.Height] = parsedUpdate(t, update)
	env.counterparty.proof = func(_ uint64, got [][32]byte) (evm.AccountProof, error) {
		require.Equal(t, slots, got)
		return evm.AccountProof{
			AccountProof: accountNodes(t, env.fixture.NonMembership),
			StorageProofs: []evm.StorageProof{
				{Key: slots[0], Value: big.NewInt(0), Proof: proofNodes(t, env.fixture.NonMembership)},
			},
		}, nil
	}

	proofs, err := env.gen.PacketProofs(
		t.Context(), update.Height, v2.ProofKindReceiptAbsence, []channeltypesv2.Packet{packet, packet},
	)
	require.NoError(t, err)
	require.Len(t, proofs, 2)
	first, err := besumsgs.NewBindings().UnpackMembershipProof(proofs[0])
	require.NoError(t, err)
	second, err := besumsgs.NewBindings().UnpackMembershipProof(proofs[1])
	require.NoError(t, err)
	require.Equal(t, accountNodes(t, env.fixture.NonMembership), first.AccountProofNodes)
	require.Empty(t, second.AccountProofNodes)
	require.Equal(t, first.ProofNodes, second.ProofNodes)
	require.Equal(t, first.ConsensusStatePreimage, second.ConsensusStatePreimage)
}

func TestExpiredClientStoredTargets(t *testing.T) {
	for _, target := range []uint64{112, 113, 114} {
		t.Run(fmt.Sprint(target), func(t *testing.T) {
			env := newFixtureEnv(t)
			env.setAnchor(t, 112, env.fixture.InitialConsensusState())
			env.host.clientState.LatestHeight.RevisionHeight = 113
			env.counterparty.sealed[113] = consensusHeader(113, env.fixture.InitialConsensusState())
			env.host.hashes[113] = env.host.hashes[112]
			env.counterparty.sealed[114] = consensusHeader(114, env.fixture.InitialConsensusState())
			env.host.latest = &v2.BlockHeader{
				Timestamp: time.Unix(int64(env.fixture.InitialTrustedTimestamp+env.fixture.TrustingPeriod), 0),
			}
			proof, err := env.gen.ClientUpdatePayload(t.Context(), target)
			if target == 114 {
				require.ErrorIs(t, err, ErrClientExpired)
			} else {
				require.NoError(t, err)
			}
			require.Empty(t, proof)
		})
	}
}
