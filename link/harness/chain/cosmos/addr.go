package cosmos

import (
	"fmt"

	sdkbech32 "github.com/cosmos/cosmos-sdk/types/bech32"
)

// AccountAddressFromKeyHex derives the cosmos bech32 account address for a plain secp256k1 hex private
// key (with or without 0x) — see newSigner for the derivation. This is the harness side's own copy (the
// stub carries its own in link/internal/cosmos); the two must agree on the account, so both derive it from
// the same hex key rather than one telling the other. It differs from the same key's EVM address
// (Keccak over the uncompressed pubkey), so the Cosmos account is distinct from the EVM
// faucet/relayer even when a key is reused.
func AccountAddressFromKeyHex(hexKey string) (string, error) {
	sg, err := newSigner(hexKey)
	if err != nil {
		return "", err
	}
	return sg.address, nil
}

// CanonicalAddress validates s as a cosmos account address (bech32 under the "cosmos" HRP) and returns
// its canonical form (re-encoded, so mixed-case bech32 folds to the lowercase convention). It is the
// cosmos side of the Reader's CanonicalAddr seam — the family's single string->address choke point,
// mirroring the EVM reader's hex validation.
func CanonicalAddress(s string) (string, error) {
	hrp, data, err := sdkbech32.DecodeAndConvert(s)
	if err != nil {
		return "", fmt.Errorf("cosmos: %q is not a valid bech32 address: %w", s, err)
	}
	if hrp != Bech32HRP {
		return "", fmt.Errorf("cosmos: address %q has prefix %q, want %q", s, hrp, Bech32HRP)
	}
	return sdkbech32.ConvertAndEncode(hrp, data)
}
