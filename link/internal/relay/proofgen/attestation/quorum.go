package attestation

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/service/attestor"
	attestorevm "github.com/cosmos/ibc/link/internal/service/attestor/evm"
)

// quorumResult the aggregated, quorum-verified attestation for one claim:
// every kept signature attests to the exact same attestationData.
type quorumResult struct {
	AttestationData []byte
	Signatures      [][]byte
}

// queryStateQuorum aggregates a StateAttestation claim across attestors.
func queryStateQuorum(
	ctx context.Context,
	attestors []attestor.Attestor,
	threshold int,
	height uint64,
) (quorumResult, error) {
	return queryQuorum(ctx, attestors, threshold, attestorevm.TagStateAttestation, func(
		ctx context.Context,
		a attestor.Attestor,
	) (attestor.Attestation, error) {
		return a.StateAttestation(ctx, height)
	})
}

// queryPacketQuorum aggregates a PacketAttestation claim across attestors.
func queryPacketQuorum(
	ctx context.Context,
	attestors []attestor.Attestor,
	threshold int,
	packets [][]byte,
	height uint64,
	kind attestor.CommitmentType,
) (quorumResult, error) {
	return queryQuorum(ctx, attestors, threshold, attestorevm.TagPacketAttestation, func(
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
	name   string
	signer common.Address
	data   []byte
	sig    []byte
	err    error
}

// queryQuorum fans query out to every attestor concurrently, keeps only
// responses whose signature recovers over the domain-tagged digest, requires
// byte-equality of attestationData across kept responses, and requires
// the number of distinct signers to reach threshold.
func queryQuorum(
	ctx context.Context,
	attestors []attestor.Attestor,
	threshold int,
	typeTag byte,
	query attestationQuery,
) (quorumResult, error) {
	if len(attestors) == 0 {
		return quorumResult{}, errors.New("no attestors configured")
	}

	responses := make([]quorumResponse, len(attestors))

	var wg sync.WaitGroup

	for i, a := range attestors {
		wg.Add(1)

		go func(i int, a attestor.Attestor) {
			defer wg.Done()

			responses[i] = queryOne(ctx, a, typeTag, query)
		}(i, a)
	}

	wg.Wait()

	return reduceQuorum(responses, threshold)
}

func queryOne(ctx context.Context, a attestor.Attestor, typeTag byte, query attestationQuery) quorumResponse {
	attestation, err := query(ctx, a)
	if err != nil {
		return quorumResponse{name: a.Name(), err: errors.Wrapf(err, "attestor %q", a.Name())}
	}

	data := attestation.AttestedData
	sig := attestation.Signature

	signer, err := attestorevm.RecoverSigner(attestorevm.Digest(typeTag, data), sig)
	if err != nil {
		return quorumResponse{name: a.Name(), err: errors.Wrapf(err, "attestor %q", a.Name())}
	}

	return quorumResponse{name: a.Name(), signer: signer, data: data, sig: sig}
}

// reduceQuorum groups responses by their exact attestationData value
// and returns the first value whose distinct signers reach threshold.
func reduceQuorum(responses []quorumResponse, threshold int) (quorumResult, error) {
	buckets := make(map[string][]quorumResponse)

	for _, resp := range responses {
		if resp.err != nil {
			continue
		}

		buckets[string(resp.data)] = append(buckets[string(resp.data)], resp)
	}

	for _, bucket := range buckets {
		seenSigners := make(map[common.Address]struct{})

		var signatures [][]byte

		for _, resp := range bucket {
			if _, dup := seenSigners[resp.signer]; dup {
				continue
			}

			seenSigners[resp.signer] = struct{}{}
			signatures = append(signatures, resp.sig)
		}

		if len(signatures) >= threshold {
			return quorumResult{AttestationData: bucket[0].data, Signatures: signatures}, nil
		}
	}

	return quorumResult{}, errors.Errorf(
		"quorum not met: no attestationData value reached %d required signatures from %d configured attestors (%s)",
		threshold, len(responses), joinResponseErrors(responses),
	)
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
		return "no errors; excluded due to disagreeing data or duplicate signer"
	}

	return strings.Join(msgs, "; ")
}

// latestProvableHeight finds a height every attestor in the quorum has. It fans
// LatestHeight out concurrently and takes the minimum among the attestors
// that answered, requiring at least threshold of them to respond.
func latestProvableHeight(
	ctx context.Context,
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
