// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"math"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

func FuzzDecodeStateAttestation(f *testing.F) {
	for _, seed := range [][2]uint64{{0, 0}, {1, 1_700_000_000}, {math.MaxUint64, math.MaxUint64}} {
		data, err := EncodeStateAttestation(seed[0], seed[1])
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		height, timestamp, err := DecodeStateAttestation(data)
		if err != nil {
			return
		}

		encoded, err := EncodeStateAttestation(height, timestamp)
		require.NoError(t, err)
		gotHeight, gotTimestamp, err := DecodeStateAttestation(encoded)
		require.NoError(t, err)
		require.Equal(t, height, gotHeight)
		require.Equal(t, timestamp, gotTimestamp)
	})
}

func FuzzDecodePacketAttestation(f *testing.F) {
	for _, packets := range [][]PacketCompact{nil, {{Path: [32]byte{1}, Commitment: [32]byte{2}}, {}}} {
		data, err := EncodePacketAttestation(7, packets)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		height, packets, err := DecodePacketAttestation(data)
		if err != nil {
			return
		}

		encoded, err := EncodePacketAttestation(height, packets)
		require.NoError(t, err)
		gotHeight, gotPackets, err := DecodePacketAttestation(encoded)
		require.NoError(t, err)
		require.Equal(t, height, gotHeight)
		require.Equal(t, packets, gotPackets)
	})
}

// FuzzRecoverSigner feeds attestor-supplied signatures to RecoverSigner, and
// checks it recovers the signer of genuine signatures in either v form.
func FuzzRecoverSigner(f *testing.F) {
	f.Add([]byte("digest"), []byte("key"), make([]byte, 65))
	f.Add([]byte{}, []byte{}, []byte{})

	f.Fuzz(func(t *testing.T, digestSeed, keySeed, sig []byte) {
		digest := [32]byte(crypto.Keccak256(digestSeed))

		_, _ = RecoverSigner(digest, sig)

		key, err := crypto.ToECDSA(crypto.Keccak256(keySeed))
		if err != nil {
			return
		}
		want := crypto.PubkeyToAddress(key.PublicKey)

		sig, err = crypto.Sign(digest[:], key)
		require.NoError(t, err)
		legacy, err := normalizeSignature(sig)
		require.NoError(t, err)
		for _, form := range [][]byte{sig, legacy} {
			got, err := RecoverSigner(digest, form)
			require.NoError(t, err, "v=%d", form[64])
			require.Equal(t, want, got, "v=%d", form[64])
		}
	})
}
