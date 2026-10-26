// SPDX-License-Identifier: Apache-2.0

package besubft

import (
	"bytes"
	"fmt"
	"log/slog"
	"math/big"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/besumsgs"
	"github.com/cosmos/ibc/cli/besu"
)

var (
	oneThird  = besumsgs.IBesuLightClientMsgsTrustThreshold{Numerator: 1, Denominator: 3}
	oneHalf   = besumsgs.IBesuLightClientMsgsTrustThreshold{Numerator: 1, Denominator: 2}
	twoThirds = besumsgs.IBesuLightClientMsgsTrustThreshold{Numerator: 2, Denominator: 3}
)

// addrs returns validator addresses in ascending order, as headers list them.
func addrs(ids ...int64) []common.Address {
	out := make([]common.Address, len(ids))
	for i, id := range ids {
		out[i] = common.BigToAddress(big.NewInt(id))
	}
	return out
}

// era is a run of headers from height on with one validator set, each sealed
// by signers.
type era struct {
	from       uint64
	validators []int64
	signers    []int64
}

// syntheticEnv is a chain of headers from trusted to head, with the client
// trusting trusted.
func syntheticEnv(
	t *testing.T,
	level besumsgs.IBesuLightClientMsgsTrustThreshold,
	trusted, head uint64,
	eras ...era,
) (*Generator, *fakeChain, *bytes.Buffer) {
	t.Helper()

	counterparty := &fakeChain{id: "besu-b", head: head, sealed: map[uint64]*besu.ParsedHeader{}}
	current := 0
	for height := trusted; height <= head; height++ {
		for current+1 < len(eras) && eras[current+1].from <= height {
			current++
		}
		e := eras[current]
		counterparty.sealed[height] = &besu.ParsedHeader{
			RLP:        fmt.Appendf(nil, "header-%d", height),
			Height:     height,
			Timestamp:  1000 + height,
			StateRoot:  common.BigToHash(new(big.Int).SetUint64(height)),
			Validators: addrs(e.validators...),
			Signers:    addrs(e.signers...),
		}
	}

	host := &fakeChain{id: "host", clientState: besumsgs.IBesuLightClientMsgsClientState{
		LatestHeight: besumsgs.IICS02ClientMsgsHeight{RevisionHeight: trusted},
		TrustLevel:   level,
	}}
	logs := &bytes.Buffer{}
	gen := &Generator{
		host:         host,
		counterparty: counterparty,
		clientID:     clientID,
		logger:       slog.New(slog.NewTextHandler(logs, nil)),
	}
	return gen, counterparty, logs
}

// requireUpdates checks payloads update along heights, each trusting the
// height before, starting from trusted.
func requireUpdates(t *testing.T, chain *fakeChain, payloads [][]byte, trusted uint64, heights ...uint64) {
	t.Helper()

	require.Len(t, payloads, len(heights))
	for i, payload := range payloads {
		decoded, err := besumsgs.NewBindings().UnpackUpdateClient(payload)
		require.NoError(t, err)
		assert.Equal(t, chain.sealed[heights[i]].RLP, decoded.HeaderRlp, "update %d", i)
		assert.Equal(t, trusted, decoded.TrustedHeight.RevisionHeight, "update %d", i)
		assert.Equal(t, chain.sealed[trusted].ConsensusState(), decoded.ConsensusStatePreimage, "update %d", i)
		trusted = heights[i]
	}
}

// Several set changes between relays leave the head with too few trusted
// signers. The client steps to the furthest header it can verify, then on.
func TestClientUpdatePayloadsCatchUp(t *testing.T) {
	gen, chain, _ := syntheticEnv(t, oneThird, 10, 100,
		era{from: 10, validators: []int64{1, 2, 3, 4}, signers: []int64{1, 2, 3}},
		era{from: 30, validators: []int64{2, 3, 4, 5}, signers: []int64{3, 4, 5}},
		era{from: 60, validators: []int64{3, 4, 5, 6}, signers: []int64{4, 5, 6}},
		era{from: 80, validators: []int64{4, 5, 6, 7}, signers: []int64{5, 6, 7}},
	)

	height, _, err := gen.LatestProvableHeight(t.Context())
	require.NoError(t, err)
	require.Equal(t, uint64(100), height)

	payloads, err := gen.ClientUpdatePayloads(t.Context(), height)
	require.NoError(t, err)
	requireUpdates(t, chain, payloads, 10, 59, 79, 100)
}

// A set that changes and changes back is passed by the headers with the
// original set, though the search first lands on the other.
func TestClientUpdatePayloadsSetChangesBack(t *testing.T) {
	gen, chain, _ := syntheticEnv(t, oneThird, 10, 100,
		era{from: 10, validators: []int64{1, 2, 3, 4}, signers: []int64{1, 2, 3}},
		era{from: 40, validators: []int64{11, 12, 13, 14}, signers: []int64{11, 12, 13}},
		era{from: 60, validators: []int64{1, 2, 3, 4}, signers: []int64{1, 2, 3}},
		era{from: 80, validators: []int64{2, 3, 4, 5}, signers: []int64{3, 4, 5}},
		era{from: 90, validators: []int64{3, 4, 5, 6}, signers: []int64{4, 5, 6}},
	)

	payloads, err := gen.ClientUpdatePayloads(t.Context(), 100)
	require.NoError(t, err)
	requireUpdates(t, chain, payloads, 10, 39, 89, 100)
}

// An override to a set the client cannot follow does not stop it when a later
// set keeps enough of the trusted validators.
func TestClientUpdatePayloadsBridgesOverride(t *testing.T) {
	gen, chain, _ := syntheticEnv(t, oneThird, 10, 100,
		era{from: 10, validators: []int64{1, 2, 3, 4}, signers: []int64{1, 2, 3}},
		era{from: 40, validators: []int64{11, 12, 13, 14}, signers: []int64{11, 12, 13}},
		era{from: 60, validators: []int64{3, 4, 5, 6}, signers: []int64{3, 4, 5}},
		era{from: 100, validators: []int64{4, 5, 6, 7}, signers: []int64{5, 6, 7}},
	)

	payloads, err := gen.ClientUpdatePayloads(t.Context(), 100)
	require.NoError(t, err)
	requireUpdates(t, chain, payloads, 10, 39, 99, 100)
}

// An override that replaces part of the set at once is crossed by a header
// enough trusted validators sealed, though not every header after it is.
func TestClientUpdatePayloadsOverlappingOverride(t *testing.T) {
	gen, chain, _ := syntheticEnv(t, twoThirds, 10, 100,
		era{from: 10, validators: []int64{1, 2, 3, 4}, signers: []int64{1, 2, 3}},
		era{from: 50, validators: []int64{1, 2, 3, 9}, signers: []int64{1, 2, 9}},
	)
	chain.sealed[52].Signers = addrs(1, 2, 3)

	payloads, err := gen.ClientUpdatePayloads(t.Context(), 100)
	require.NoError(t, err)
	requireUpdates(t, chain, payloads, 10, 52, 100)
}

// A set the client cannot follow stops it at the header before. The client
// relays up to there, the stop is logged once, and an update past it fails
// saying where.
func TestClientStuckAtSetChange(t *testing.T) {
	gen, chain, logs := syntheticEnv(t, oneThird, 10, 100,
		era{from: 10, validators: []int64{1, 2, 3, 4}, signers: []int64{1, 2, 3}},
		era{from: 30, validators: []int64{2, 3, 4, 5}, signers: []int64{3, 4, 5}},
		era{from: 60, validators: []int64{11, 12, 13, 14}, signers: []int64{11, 12, 13}},
	)

	for range 2 {
		height, ts, err := gen.LatestProvableHeight(t.Context())
		require.NoError(t, err)
		require.Equal(t, uint64(59), height)
		assert.Equal(t, int64(1059), ts.Unix())
	}
	assert.Equal(t, 1, bytes.Count(logs.Bytes(), []byte("cannot follow a validator set change")), logs.String())
	assert.Contains(t, logs.String(), "reachedHeight=59")

	payloads, err := gen.ClientUpdatePayloads(t.Context(), 59)
	require.NoError(t, err)
	requireUpdates(t, chain, payloads, 10, 59)

	_, err = gen.ClientUpdatePayloads(t.Context(), 100)
	var stuck *stuckError
	require.ErrorAs(t, err, &stuck)
	assert.Equal(t, stuckError{trusted: 10, reached: chain.sealed[59], signed: 0, required: 2}, *stuck)
	assert.EqualError(t, err, "client at height 10 cannot follow the validator set past height 59: "+
		"header 60 is sealed by 0 of the 4 validators trusted at height 59, it needs 2")
}

// Above one half, a single removal can stop the client: its quorum need not
// carry enough of the validators trusted before.
func TestClientStuckAtRemovalAboveOneHalf(t *testing.T) {
	gen, _, _ := syntheticEnv(t, twoThirds, 10, 100,
		era{from: 10, validators: []int64{1, 2, 3, 4}, signers: []int64{1, 2, 3}},
		era{from: 50, validators: []int64{1, 2, 3}, signers: []int64{1, 2}},
	)

	height, _, err := gen.LatestProvableHeight(t.Context())
	require.NoError(t, err)
	assert.Equal(t, uint64(49), height)

	_, err = gen.ClientUpdatePayloads(t.Context(), 100)
	assert.EqualError(t, err, "client at height 10 cannot follow the validator set past height 49: "+
		"header 50 is sealed by 2 of the 4 validators trusted at height 49, it needs 3")
}

// Whether a target verifies depends on the target alone, not on an earlier
// one the client could not reach.
func TestClientUpdatePayloadsAfterStuck(t *testing.T) {
	gen, chain, _ := syntheticEnv(t, twoThirds, 10, 120,
		era{from: 10, validators: []int64{1, 2, 3, 4}, signers: []int64{1, 2, 3}},
		era{from: 50, validators: []int64{1, 2, 3, 9}, signers: []int64{1, 2, 9}},
	)
	chain.sealed[120].Signers = addrs(1, 2, 3)

	_, err := gen.ClientUpdatePayloads(t.Context(), 100)
	var stuck *stuckError
	require.ErrorAs(t, err, &stuck)
	assert.Equal(t, uint64(49), stuck.reached.Height)

	payloads, err := gen.ClientUpdatePayloads(t.Context(), 120)
	require.NoError(t, err)
	requireUpdates(t, chain, payloads, 10, 120)
}

// A header that cannot be read is an error, not a set change the client
// cannot follow.
func TestPathHeaderUnavailable(t *testing.T) {
	gen, chain, _ := syntheticEnv(t, oneThird, 10, 100,
		era{from: 10, validators: []int64{1, 2, 3, 4}, signers: []int64{1, 2, 3}},
		era{from: 80, validators: []int64{3, 4, 5, 6}, signers: []int64{4, 5, 6}},
	)
	delete(chain.sealed, 55)

	_, _, err := gen.LatestProvableHeight(t.Context())
	require.ErrorContains(t, err, "reading header 55")
	var stuck *stuckError
	assert.NotErrorAs(t, err, &stuck)
}

// The trusted header is read once while the client stays at it.
func TestTrustedHeaderReadOnce(t *testing.T) {
	gen, chain, _ := syntheticEnv(t, oneThird, 10, 100,
		era{from: 10, validators: []int64{1, 2, 3, 4}, signers: []int64{1, 2, 3}},
	)

	height, _, err := gen.LatestProvableHeight(t.Context())
	require.NoError(t, err)
	_, err = gen.ClientUpdatePayloads(t.Context(), height)
	require.NoError(t, err)
	assert.Equal(t, []uint64{100, 10, 100}, chain.reads)
}

// A node behind the client reports its head without reading the trusted
// header it does not have yet.
func TestLatestProvableHeightNodeBehindClient(t *testing.T) {
	gen, chain, _ := syntheticEnv(t, oneThird, 10, 100,
		era{from: 10, validators: []int64{1, 2, 3, 4}, signers: []int64{1, 2, 3}},
	)
	behind := *chain.sealed[10]
	behind.Height = 9
	chain.head, chain.sealed[9] = 9, &behind

	height, _, err := gen.LatestProvableHeight(t.Context())
	require.NoError(t, err)
	assert.Equal(t, uint64(9), height)
	assert.Equal(t, []uint64{9}, chain.reads)
}

// randomEras is a chain from height 1 to head whose set changes by one
// validator on a third of blocks, and by two at once on one in overrideEvery
// when that is positive, each header sealed by a random quorum.
func randomEras(rng *rand.Rand, head uint64, overrideEvery int) []era {
	validators, next := []int64{1, 2, 3, 4}, int64(5)
	eras := make([]era, 0, head)
	for height := uint64(1); height <= head; height++ {
		switch {
		case height == 1:
		case overrideEvery > 0 && rng.IntN(overrideEvery) == 0:
			for range 2 {
				i := rng.IntN(len(validators))
				validators, next = append(slices.Delete(slices.Clone(validators), i, i+1), next), next+1
			}
		case rng.IntN(3) > 0:
		case len(validators) > 1 && rng.IntN(2) == 0:
			i := rng.IntN(len(validators))
			validators = slices.Delete(slices.Clone(validators), i, i+1)
		default:
			validators, next = append(slices.Clone(validators), next), next+1
		}
		signers := slices.Clone(validators)
		rng.Shuffle(len(signers), func(i, j int) { signers[i], signers[j] = signers[j], signers[i] })
		signers = signers[:(2*len(validators)+2)/3]
		slices.Sort(signers)
		eras = append(eras, era{from: height, validators: validators, signers: signers})
	}
	return eras
}

// requireVerifiedUpdates checks payloads take the client from trusted to
// target, each passing the contract's overlap and quorum rules against the
// header before.
func requireVerifiedUpdates(
	t *testing.T,
	chain *fakeChain,
	payloads [][]byte,
	level besumsgs.IBesuLightClientMsgsTrustThreshold,
	trusted, target uint64,
) {
	t.Helper()

	from := chain.sealed[trusted]
	for _, payload := range payloads {
		decoded, err := besumsgs.NewBindings().UnpackUpdateClient(payload)
		require.NoError(t, err)
		require.Equal(t, from.Height, decoded.TrustedHeight.RevisionHeight)
		var height uint64
		_, err = fmt.Sscanf(string(decoded.HeaderRlp), "header-%d", &height)
		require.NoError(t, err)
		header := chain.sealed[height]

		overlap := 0
		for _, signer := range header.Signers {
			if slices.Contains(from.Validators, signer) {
				overlap++
			}
		}
		require.GreaterOrEqual(t, overlap*int(level.Denominator), len(from.Validators)*int(level.Numerator),
			"update to %d trusting %d", height, from.Height)
		require.GreaterOrEqual(t, 3*len(header.Signers), 2*len(header.Validators), "quorum at %d", height)
		from = header
	}
	require.Equal(t, target, from.Height)
}

// Header voting changes at most one validator a block, so at a trust level of
// at most one half the client follows any such chain, however far behind it is.
func TestPathFollowsHeaderVoting(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // deterministic test chains
	const head = 300
	for _, level := range []besumsgs.IBesuLightClientMsgsTrustThreshold{oneThird, oneHalf} {
		for range 100 {
			gen, chain, _ := syntheticEnv(t, level, 1, head, randomEras(rng, head, 0)...)

			height, _, err := gen.LatestProvableHeight(t.Context())
			require.NoError(t, err)
			require.Equal(t, uint64(head), height)

			payloads, err := gen.ClientUpdatePayloads(t.Context(), head)
			require.NoError(t, err)
			requireVerifiedUpdates(t, chain, payloads, level, 1, head)
		}
	}
}

// Overrides can stop the client short of the head, but the height reported is
// one the updates reach.
func TestLatestProvableHeightIsReachable(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 9)) //nolint:gosec // deterministic test chains
	const head = 120
	for _, level := range []besumsgs.IBesuLightClientMsgsTrustThreshold{oneThird, oneHalf, twoThirds} {
		stopped := 0
		for range 1000 {
			gen, chain, _ := syntheticEnv(t, level, 1, head, randomEras(rng, head, 20)...)

			height, _, err := gen.LatestProvableHeight(t.Context())
			require.NoError(t, err)
			if height < head {
				stopped++
			}

			payloads, err := gen.ClientUpdatePayloads(t.Context(), height)
			require.NoError(t, err, "reported height %d", height)
			requireVerifiedUpdates(t, chain, payloads, level, 1, height)
		}
		require.Positive(t, stopped)
	}
}
