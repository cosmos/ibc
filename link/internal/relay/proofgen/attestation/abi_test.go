package attestation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAttestationProofEncode(t *testing.T) {
	proof := AttestationProof{
		AttestationData: []byte{0x01, 0x02, 0x03},
		Signatures:      [][]byte{make([]byte, 65), make([]byte, 65)},
	}

	encoded, err := encodeAttestationProof(proof)
	require.NoError(t, err)
	require.NotEmpty(t, encoded)
}
