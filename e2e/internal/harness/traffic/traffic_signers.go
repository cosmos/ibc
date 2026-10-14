// SPDX-License-Identifier: Apache-2.0

package traffic

import (
	"context"
	"crypto/ecdsa"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/cosmos/ibc/cli/keyfile"
	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
)

const relayerSignerAlias = "traffic-relayer"

// Signer is an independent local identity that drives traffic. Its
// credentials remain private to this package.
type Signer struct {
	key     *ecdsa.PrivateKey
	account evm.Account
}

// NewSigner generates a randomly-keyed local identity.
func NewSigner() (Signer, error) {
	key, err := crypto.GenerateKey()
	if err != nil {
		return Signer{}, fmt.Errorf("traffic: generate signer key: %w", err)
	}
	return signerFromKey(key), nil
}

// NewSignerFromHex creates a deterministically keyed local identity.
func NewSignerFromHex(privateKeyHex string) (Signer, error) {
	key, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return Signer{}, fmt.Errorf("traffic: parse signer key: %w", err)
	}
	return signerFromKey(key), nil
}

func signerFromKey(key *ecdsa.PrivateKey) Signer {
	return Signer{key: key, account: evm.AccountFromKey(key)}
}

// Address returns the signer's public identity.
func (s Signer) Address() common.Address {
	return s.account.Address()
}

// BroadcastTx signs, submits, and waits for a zero-value transaction from the signer.
func (s Signer) BroadcastTx(
	ctx context.Context,
	evm *environment.EVM,
	to common.Address,
	data []byte,
) error {
	_, err := evm.BroadcastTx(ctx, s.account, &to, data, nil)
	return err
}

// String renders only the public signer address.
func (s Signer) String() string {
	return fmt.Sprintf("traffic signer %s", s.Address())
}

// GoString renders only the public signer address, including for %#v formatting.
func (s Signer) GoString() string {
	return s.String()
}

// storeKey writes the signer's key file for use by the relayer process.
func (s Signer) storeKey(path string) error {
	if err := keyfile.Store(path, keyfile.ECDSA, crypto.FromECDSA(s.key)); err != nil {
		return fmt.Errorf("store signer key: %w", err)
	}
	return nil
}
