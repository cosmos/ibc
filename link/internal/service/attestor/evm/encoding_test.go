package evm

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeStateAttestation(t *testing.T) {
	for _, tt := range []struct {
		name      string
		height    uint64
		timestamp uint64
	}{
		{
			name:      "values",
			height:    42,
			timestamp: 1_700_000_000,
		},
		{
			name:      "zero values",
			height:    0,
			timestamp: 0,
		},
		{
			name:      "maximum values",
			height:    ^uint64(0),
			timestamp: ^uint64(0),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// ACT
			encoded, err := EncodeStateAttestation(tt.height, tt.timestamp)

			// ASSERT
			require.NoError(t, err)
			require.Len(t, encoded, 64)
			assert.Equal(t, abiWord(tt.height), encoded[:32])
			assert.Equal(t, abiWord(tt.timestamp), encoded[32:])
		})
	}
}

func abiWord(value uint64) []byte {
	word := make([]byte, 32)
	binary.BigEndian.PutUint64(word[24:], value)

	return word
}
