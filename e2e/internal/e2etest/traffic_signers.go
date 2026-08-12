// SPDX-License-Identifier: Apache-2.0

package e2etest

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/link/keyfile"
)

const relayerSignerAlias = "e2etest-relayer"

// Signer is an independent local identity used by the acceptance tests. Its
// credentials remain private to this package.
type Signer struct {
	key     *ecdsa.PrivateKey
	account evm.Account
}

// NewSigner generates a randomly-keyed local identity.
func NewSigner(t testing.TB) Signer {
	t.Helper()
	key, err := crypto.GenerateKey()
	require.NoError(t, err, "e2etest: generate signer key")
	account, err := evm.AccountFromHex(hex.EncodeToString(crypto.FromECDSA(key)))
	require.NoError(t, err, "e2etest: create signer account")
	return Signer{key: key, account: account}
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
	return fmt.Sprintf("e2etest signer %s", s.Address())
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
