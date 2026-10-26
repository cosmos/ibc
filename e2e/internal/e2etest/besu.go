// SPDX-License-Identifier: Apache-2.0

package e2etest

import (
	"crypto/ecdsa"
	"math/big"
	"slices"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
)

// BesuHeaders reads a managed Besu Chain's headers and seals headers it never
// produced with its validator keys.
type BesuHeaders struct {
	t         testing.TB
	chain     *environment.Chain
	evm       *environment.EVM
	consensus besu.Consensus
	keys      []*ecdsa.PrivateKey
}

func NewBesuHeaders(
	t testing.TB,
	env *environment.Environment,
	id environment.ChainID,
	consensus besu.Consensus,
) BesuHeaders {
	t.Helper()
	chain, err := env.Chain(id)
	require.NoError(t, err)
	evm, err := chain.EVM()
	require.NoError(t, err)
	validators, err := chain.Validators()
	require.NoError(t, err)
	return BesuHeaders{t: t, chain: chain, evm: evm, consensus: consensus, keys: validators.Keys()}
}

// Quorum is the fewest validator keys whose seals pass the client's quorum
// and trusted-overlap checks: two thirds, rounded up.
func (h BesuHeaders) Quorum() []*ecdsa.PrivateKey {
	return slices.Clone(h.keys[:(2*len(h.keys)+2)/3])
}

// Raw is the header at height as the node returns it, waiting for the Chain
// to produce it.
func (h BesuHeaders) Raw(height uint64) *types.Header {
	h.t.Helper()
	timing := h.chain.Timing()
	var header *types.Header
	require.Eventually(h.t, func() bool {
		var err error
		header, err = h.evm.HeaderByNumber(h.t.Context(), new(big.Int).SetUint64(height))
		return err == nil && header != nil
	}, timing.CompletionBudget, timing.PollInterval, "besu header %d", height)
	return header
}

// Sealed is the header at height with its seals in the form the client
// accepts.
func (h BesuHeaders) Sealed(height uint64) *besu.ParsedHeader {
	h.t.Helper()
	header, err := besu.ParseSealedHeader(h.Raw(height))
	require.NoError(h.t, err)
	return header
}

// Seal sets header's validators and seals it by signers.
func (h BesuHeaders) Seal(
	header *types.Header,
	validators []common.Address,
	signers []*ecdsa.PrivateKey,
) *besu.ParsedHeader {
	h.t.Helper()
	sealed, err := besu.SealHeader(header, h.consensus, validators, signers)
	require.NoError(h.t, err)
	return sealed
}
