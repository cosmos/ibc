// SPDX-License-Identifier: Apache-2.0

package attestation

import (
	"bytes"
	"context"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"

	attestorevm "github.com/cosmos/ibc/cli/attestor/evm"
	"github.com/cosmos/ibc/cli/internal/chains"
	"github.com/cosmos/ibc/cli/internal/service/attestor"
)

// quorumResult the aggregated, quorum-verified attestation for one claim:
// every kept signature attests to the exact same attestationData.
type quorumResult struct {
	AttestationData []byte
	Signatures      [][]byte
}

// aggregationRound keeps the client identity and proof kind with one aggregation attempt.
type aggregationRound struct {
	*Generator
	proofKind string
}

func (g *Generator) newRound(kind string) *aggregationRound {
	return &aggregationRound{Generator: g, proofKind: kind}
}

// queryStateQuorum aggregates a StateAttestation claim across attestors.
func (r *aggregationRound) queryStateQuorum(
	ctx context.Context,
	height uint64,
	expectedData []byte,
) (quorumResult, error) {
	return r.queryQuorum(ctx, attestorevm.TagStateAttestation, expectedData, func(
		ctx context.Context,
		a attestor.Attestor,
	) (attestor.Attestation, error) {
		return a.StateAttestation(ctx, height)
	})
}

// queryPacketQuorum aggregates a PacketAttestation claim across attestors.
func (r *aggregationRound) queryPacketQuorum(
	ctx context.Context,
	packets [][]byte,
	height uint64,
	kind attestor.CommitmentType,
	expectedData []byte,
) (quorumResult, error) {
	return r.queryQuorum(ctx, attestorevm.TagPacketAttestation, expectedData, func(
		ctx context.Context,
		a attestor.Attestor,
	) (attestor.Attestation, error) {
		return a.PacketAttestation(ctx, attestor.PacketAttestationRequest{
			Height:         height,
			Packets:        packets,
			CommitmentType: kind,
		})
	})
}

type attestationQuery func(context.Context, attestor.Attestor) (attestor.Attestation, error)

// quorumResponse one attestor's contribution to a quorum.
type quorumResponse struct {
	name          string
	signer        common.Address
	sig           []byte
	err           error
	failureReason string
}

// queryQuorum collects signatures from distinct signers over the expected claim.
func (r *aggregationRound) queryQuorum(
	ctx context.Context,
	typeTag byte,
	expectedData []byte,
	query attestationQuery,
) (quorumResult, error) {
	if len(r.attestors) == 0 {
		return quorumResult{}, errors.New("no attestors configured")
	}

	responses := make([]quorumResponse, len(r.attestors))

	var wg sync.WaitGroup

	for i, a := range r.attestors {
		wg.Add(1)

		go func(i int, a attestor.Attestor) {
			defer wg.Done()

			responses[i] = r.queryOne(ctx, a, typeTag, expectedData, query)
		}(i, a)
	}

	wg.Wait()

	signatures, err := r.reduceQuorum(ctx, responses)
	if err != nil {
		return quorumResult{}, err
	}
	return quorumResult{AttestationData: expectedData, Signatures: signatures}, nil
}

func (r *aggregationRound) queryOne(
	ctx context.Context,
	a attestor.Attestor,
	typeTag byte,
	expectedData []byte,
	query attestationQuery,
) quorumResponse {
	attestation, err := query(ctx, a)
	if err != nil {
		r.logger.Warn("Attestor query failed", "attestor", a.Name(), "err", err)
		return quorumResponse{
			name:          a.Name(),
			err:           errors.Wrapf(err, "attestor %q", a.Name()),
			failureReason: responseErrorReason(err),
		}
	}

	data := attestation.AttestedData
	if !bytes.Equal(data, expectedData) {
		return quorumResponse{
			name:          a.Name(),
			failureReason: reasonClaimMismatch,
			err:           errors.Errorf("attestor %q: attested data does not match expected claim", a.Name()),
		}
	}

	sig := attestation.Signature

	signer, err := attestorevm.RecoverSigner(attestorevm.Digest(typeTag, data), sig)
	if err != nil {
		r.logger.Warn("Attestor returned an unrecoverable signature", "attestor", a.Name(), "err", err)
		return quorumResponse{
			name:          a.Name(),
			err:           errors.Wrapf(err, "attestor %q", a.Name()),
			failureReason: reasonInvalidSignature,
		}
	}

	return quorumResponse{name: a.Name(), signer: signer, sig: sig}
}

// reduceQuorum counts distinct signers after queryOne has checked the expected claim.
func (r *aggregationRound) reduceQuorum(ctx context.Context, responses []quorumResponse) ([][]byte, error) {
	var signatures [][]byte
	seen := make(map[common.Address]bool)
	for _, resp := range responses {
		reason := resp.failureReason
		if resp.err == nil {
			if seen[resp.signer] {
				reason = reasonDuplicateSigner
			} else {
				reason = reasonNone
				seen[resp.signer] = true
				signatures = append(signatures, resp.sig)
			}
		}
		r.recordResponse(ctx, resp.name, reason)
	}

	if len(signatures) == 0 || len(signatures) < r.threshold {
		return nil, errors.Errorf(
			"quorum not met: got %d of %d required signatures from %d configured attestors (%s)",
			len(signatures), r.threshold, len(responses), joinResponseErrors(responses),
		)
	}
	if len(signatures) < len(responses) {
		r.logger.Warn("Attestation quorum met with some attestors excluded",
			"signatures", len(signatures), "attestors", len(responses),
			"threshold", r.threshold, "reasons", joinResponseErrors(responses))
	} else {
		r.logger.Debug("Attestation quorum met", "signatures", len(signatures), "threshold", r.threshold)
	}
	return signatures, nil
}

// joinResponseErrors summarizes why each non-contributing attestor was
// excluded, so a quorum failure doesn't hide the underlying per-attestor
// errors (network, protocol, or bad signature) behind just a vote count.
func joinResponseErrors(responses []quorumResponse) string {
	var msgs []string

	for _, resp := range responses {
		if resp.err != nil {
			msgs = append(msgs, resp.err.Error())
		}
	}

	if len(msgs) == 0 {
		return "no errors; excluded due to duplicate signer"
	}

	return strings.Join(msgs, "; ")
}

// latestProvableHeight finds a height every attestor in the quorum has. It fans
// LatestHeight out concurrently and takes the minimum among the attestors
// that answered, requiring at least threshold of them to respond.
func latestProvableHeight(
	ctx context.Context,
	logger *slog.Logger,
	attestors []attestor.Attestor,
	threshold int,
	counterpartyChain chains.Client,
) (uint64, time.Time, error) {
	type heightResponse struct {
		height uint64
		err    error
	}

	responses := make([]heightResponse, len(attestors))

	var wg sync.WaitGroup

	for i, a := range attestors {
		wg.Add(1)

		go func(i int, a attestor.Attestor) {
			defer wg.Done()

			height, err := a.LatestHeight(ctx)
			if err != nil {
				responses[i] = heightResponse{err: errors.Wrapf(err, "attestor %q", a.Name())}

				return
			}

			responses[i] = heightResponse{height: height}
		}(i, a)
	}

	wg.Wait()

	var (
		heights []uint64
		errMsgs []string
	)

	for _, resp := range responses {
		if resp.err != nil {
			errMsgs = append(errMsgs, resp.err.Error())

			continue
		}

		heights = append(heights, resp.height)
	}

	if len(heights) < threshold {
		return 0, time.Time{}, errors.Errorf(
			"latest height quorum not met: got %d of %d required responses (%s)",
			len(heights), threshold, strings.Join(errMsgs, "; "),
		)
	}

	if len(errMsgs) > 0 {
		logger.Warn(
			"Some attestors did not report a latest height",
			"responded", len(heights),
			"attestors", len(attestors),
			"threshold", threshold,
			"reasons", strings.Join(errMsgs, "; "),
		)
	}

	// take the highest height at least threshold attestors have reached,
	// not the raw minimum across every responder -- otherwise a single
	// healthy but lagging attestor would drag the resolved height down even
	// when threshold others already agree on something fresher.
	sort.Slice(heights, func(i, j int) bool { return heights[i] > heights[j] })
	height := heights[threshold-1]

	header, err := counterpartyChain.GetBlockHeader(ctx, height)
	if err != nil {
		return 0, time.Time{}, errors.Wrapf(err, "getting header for resolved height %d", height)
	}

	return height, header.Timestamp, nil
}
