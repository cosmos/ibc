package cosmos

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"

	sdkbech32 "github.com/cosmos/cosmos-sdk/types/bech32"
)

// signer holds the parsed relayer key as an sdk secp256k1 private key (which the tx path signs with and whose
// PubKey rides in the tx's SignerInfo) plus its derived bech32 account address (the MsgSend sender and the
// escrow the harness deploy records). The harness derives the same address from the same key independently,
// so the escrow address is agreed by construction.
type signer struct {
	privKey *secp256k1.PrivKey
	address string
}

// AccountAddressFromKeyHex derives the cosmos bech32 account address for a plain-secp256k1 hex private key
// (with or without 0x), without retaining the key. `deploy` uses it to compute the user/faucet's bech32
// (from the config's FaucetKey) for the fixturekeys.IFTFaucet entry — the source holder the harness debits —
// mirroring how the escrow address is derived from the signer key. It is the same derivation newSigner runs.
func AccountAddressFromKeyHex(hexKey string) (string, error) {
	sg, err := newSigner(hexKey)
	if err != nil {
		return "", err
	}
	return sg.address, nil
}

// gmpCounterTargetLabel is the fixed, documented seed for the cosmos GMP counter target account. The target
// only receives the counter denom (its balance is the count), so it needs no signing key; deriving its
// account bytes deterministically from a constant label (sha256 truncated to the 20-byte cosmos account
// width) yields a stable, keyless, human-traceable address distinct from the escrow/faucet, so the counter
// balance is isolated. It is not a hashed pubkey (there is no key); the label makes its provenance explicit.
const gmpCounterTargetLabel = "ibc-link/gmp/counter-target"

// GMPCounterTarget returns the cosmos GMP counter target bech32 — the account `deploy` records under
// fixturekeys.Counter (the cosmos analog of the deployed Counter contract). It is deterministic (a pure function
// of gmpCounterTargetLabel), so the address is stable across runs and both `deploy` (which emits it) and any
// reader (which reads its balance) agree by construction.
func GMPCounterTarget() (string, error) {
	sum := sha256.Sum256([]byte(gmpCounterTargetLabel))
	return sdkbech32.ConvertAndEncode(Bech32HRP, sum[:20])
}

// newSigner parses a plain-secp256k1 hex private key (with or without 0x) into an sdk secp256k1.PrivKey and
// derives the cosmos bech32 address from its PubKey().Address() — the standard cosmos-sdk derivation
// RIPEMD160(SHA256(compressed-pubkey)) with the legacy hash consensus-pinned inside the dep — bech32-encoded
// under "cosmos" with the explicit sdk encoder (never sdk.AccAddress.String(), which reads the SDK's global
// sealable prefix config a library must not mutate). This is not the eth_secp256k1 / Keccak EVM scheme, so the
// account is distinct from the same key's EVM address.
func newSigner(hexKey string) (signer, error) {
	raw, err := hex.DecodeString(strings.TrimPrefix(hexKey, "0x"))
	if err != nil {
		return signer{}, fmt.Errorf("cosmos: parse signer key: %w", err)
	}
	if len(raw) != secp256k1.PrivKeySize {
		return signer{}, fmt.Errorf("cosmos: signer key must be %d bytes, got %d", secp256k1.PrivKeySize, len(raw))
	}
	privKey := &secp256k1.PrivKey{Key: raw}
	addr, err := sdkbech32.ConvertAndEncode(Bech32HRP, privKey.PubKey().Address())
	if err != nil {
		return signer{}, fmt.Errorf("cosmos: encode signer address: %w", err)
	}
	return signer{privKey: privKey, address: addr}, nil
}
