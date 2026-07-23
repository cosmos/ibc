// Package attestation implements proofgen.ProofGenerator for the
// attestation-based light client: it queries a client's configured attestor
// set for quorum-verified state and packet claims.
package attestation

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/pkg/errors"
)

// AttestationProof mirrors IAttestationMsgs.AttestationProof: the calldata
// the light client decodes on updateClient, verifyMembership, and
// verifyNonMembership calls. The attestor package has no equivalent: only
// the relayer ever assembles this for on-chain submission.
type AttestationProof struct {
	AttestationData []byte
	Signatures      [][]byte
}

var attestationProofArgs abi.Arguments

//nolint:gochecknoinits // one-time ABI type construction, mirrors abigen's own package-level MetaData pattern
func init() {
	attestationProofType, err := abi.NewType("tuple", "", []abi.ArgumentMarshaling{
		{Name: "attestationData", Type: "bytes"},
		{Name: "signatures", Type: "bytes[]"},
	})
	if err != nil {
		panic(errors.Wrap(err, "constructing attestation proof abi type"))
	}

	attestationProofArgs = abi.Arguments{{Type: attestationProofType}}
}

func encodeAttestationProof(p AttestationProof) ([]byte, error) {
	encoded, err := attestationProofArgs.Pack(p)
	if err != nil {
		return nil, errors.Wrap(err, "encoding attestation proof")
	}

	return encoded, nil
}
