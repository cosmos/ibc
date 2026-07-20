package attestation

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"sync"

	"connectrpc.com/connect"
	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/config"
	ibcattestor "github.com/cosmos/ibc/link/internal/types/ibcattestor"
)

// quorumResult the aggregated, quorum-verified attestation for one claim:
// every kept signature attests to the exact same attestationData.
type quorumResult struct {
	AttestationData []byte
	Signatures      [][]byte
}

// namedAttestorClient pairs an attestor client with the name used to
// attribute errors to it.
type namedAttestorClient struct {
	Name   string
	Client ibcattestor.AttestationServiceClient
}

// queryStateQuorum aggregates a StateAttestation claim across attestors.
func queryStateQuorum(
	ctx context.Context,
	attestors []namedAttestorClient,
	threshold int,
	height uint64,
) (quorumResult, error) {
	return queryQuorum(ctx, attestors, threshold, attestationTypeState, func(
		ctx context.Context,
		client ibcattestor.AttestationServiceClient,
	) (*ibcattestor.Attestation, error) {
		resp, err := client.StateAttestation(ctx, connect.NewRequest(&ibcattestor.StateAttestationRequest{
			Height: height,
		}))
		if err != nil {
			return nil, err
		}

		return resp.Msg.GetAttestation(), nil
	})
}

// queryPacketQuorum aggregates a PacketAttestation claim across attestors.
// packets are the abi-encoded IICS26RouterMsgs.Packet blobs the attestor
// independently re-derives paths and commitments from.
func queryPacketQuorum(
	ctx context.Context,
	attestors []namedAttestorClient,
	threshold int,
	packets [][]byte,
	height uint64,
	kind ibcattestor.CommitmentType,
) (quorumResult, error) {
	return queryQuorum(ctx, attestors, threshold, attestationTypePacket, func(
		ctx context.Context,
		client ibcattestor.AttestationServiceClient,
	) (*ibcattestor.Attestation, error) {
		resp, err := client.PacketAttestation(ctx, connect.NewRequest(&ibcattestor.PacketAttestationRequest{
			Packets:        packets,
			Height:         height,
			CommitmentType: kind,
		}))
		if err != nil {
			return nil, err
		}

		return resp.Msg.GetAttestation(), nil
	})
}

type attestationQuery func(context.Context, ibcattestor.AttestationServiceClient) (*ibcattestor.Attestation, error)

// quorumResponse one attestor's contribution to a quorum, before reduction.
type quorumResponse struct {
	name   string
	signer common.Address
	data   []byte
	sig    []byte
	err    error
}

// queryQuorum fans query out to every attestor concurrently, keeps only
// responses whose signature recovers over the domain-tagged digest, requires
// byte-equality of attestationData across kept responses (an honest quorum
// always attests to the same claim), and requires the number of distinct
// signers to reach threshold. The on-chain light client is the final arbiter
// of which addresses are trusted; this only dedupes by recovered signer so a
// single key answering on behalf of two attestor entries cannot count twice.
func queryQuorum(
	ctx context.Context,
	attestors []namedAttestorClient,
	threshold int,
	typeTag byte,
	query attestationQuery,
) (quorumResult, error) {
	if len(attestors) == 0 {
		return quorumResult{}, errors.New("no attestors configured")
	}

	responses := make([]quorumResponse, len(attestors))

	var wg sync.WaitGroup

	for i, attestor := range attestors {
		wg.Add(1)

		go func(i int, attestor namedAttestorClient) {
			defer wg.Done()

			responses[i] = queryOne(ctx, attestor, typeTag, query)
		}(i, attestor)
	}

	wg.Wait()

	return reduceQuorum(responses, threshold)
}

func queryOne(ctx context.Context, attestor namedAttestorClient, typeTag byte, query attestationQuery) quorumResponse {
	attestation, err := query(ctx, attestor.Client)
	if err != nil {
		return quorumResponse{name: attestor.Name, err: errors.Wrapf(err, "attestor %q", attestor.Name)}
	}

	data := attestation.GetAttestedData()
	sig := attestation.GetSignature()

	signer, err := recoverSigner(taggedDigest(data, typeTag), sig)
	if err != nil {
		return quorumResponse{name: attestor.Name, err: errors.Wrapf(err, "attestor %q", attestor.Name)}
	}

	return quorumResponse{name: attestor.Name, signer: signer, data: data, sig: sig}
}

func reduceQuorum(responses []quorumResponse, threshold int) (quorumResult, error) {
	seenSigners := make(map[common.Address]struct{})

	var (
		attestationData []byte
		signatures      [][]byte
	)

	for _, resp := range responses {
		if resp.err != nil {
			continue
		}

		if attestationData == nil {
			attestationData = resp.data
		} else if !bytes.Equal(attestationData, resp.data) {
			// disagreeing attestor: excluded from quorum, not fatal on its own
			continue
		}

		if _, dup := seenSigners[resp.signer]; dup {
			continue
		}

		seenSigners[resp.signer] = struct{}{}
		signatures = append(signatures, resp.sig)
	}

	if len(signatures) < threshold {
		return quorumResult{}, errors.Errorf(
			"quorum not met: got %d of %d required signatures from %d configured attestors (%s)",
			len(signatures), threshold, len(responses), joinResponseErrors(responses),
		)
	}

	return quorumResult{AttestationData: attestationData, Signatures: signatures}, nil
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

// latestHeight resolves a height every attestor in the quorum has observed,
// so a subsequent state/packet attestation query at that height can succeed
// against all of them. It fans LatestHeight out concurrently and takes the
// minimum among the attestors that answered, requiring at least threshold of
// them to respond.
func latestHeight(ctx context.Context, attestors []namedAttestorClient, threshold int) (uint64, error) {
	type heightResponse struct {
		height uint64
		err    error
	}

	responses := make([]heightResponse, len(attestors))

	var wg sync.WaitGroup

	for i, attestor := range attestors {
		wg.Add(1)

		go func(i int, attestor namedAttestorClient) {
			defer wg.Done()

			resp, err := attestor.Client.LatestHeight(ctx, connect.NewRequest(&ibcattestor.LatestHeightRequest{}))
			if err != nil {
				responses[i] = heightResponse{err: errors.Wrapf(err, "attestor %q", attestor.Name)}

				return
			}

			responses[i] = heightResponse{height: resp.Msg.GetHeight()}
		}(i, attestor)
	}

	wg.Wait()

	var (
		height  uint64
		found   bool
		answers int
	)

	var errMsgs []string

	for _, resp := range responses {
		if resp.err != nil {
			errMsgs = append(errMsgs, resp.err.Error())

			continue
		}

		answers++

		if !found || resp.height < height {
			height = resp.height
			found = true
		}
	}

	if answers < threshold {
		return 0, errors.Errorf(
			"latest height quorum not met: got %d of %d required responses (%s)",
			answers, threshold, strings.Join(errMsgs, "; "),
		)
	}

	return height, nil
}

func attestorClient(entry config.AttestorEntry) (ibcattestor.AttestationServiceClient, error) {
	if entry.Type != config.AttestorTypeRemote {
		return nil, errors.Errorf("type %q not yet supported for proof generation", entry.Type)
	}

	if entry.GRPC == "" {
		return nil, errors.New("no grpc address configured")
	}

	return ibcattestor.NewAttestationServiceClient(newAttestorHTTPClient(), "http://"+entry.GRPC, connect.WithGRPC()), nil
}

// newAttestorHTTPClient dials attestors over unencrypted h2c, matching the
// existing remote-attestor client used elsewhere in link. HTTP1 must stay
// unset: net/http.Transport only uses unencrypted HTTP/2 for http:// URLs
// when HTTP1 is excluded from the protocol set, otherwise it falls back to
// HTTP/1.1 and fails to parse the server's HTTP/2 preface.
func newAttestorHTTPClient() *http.Client {
	protocols := new(http.Protocols)
	protocols.SetHTTP2(true)
	protocols.SetUnencryptedHTTP2(true)

	return &http.Client{Transport: &http.Transport{Protocols: protocols}}
}
