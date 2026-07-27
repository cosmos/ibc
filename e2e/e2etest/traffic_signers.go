package e2etest

import (
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/link/keyfile"
)

const (
	applicationSignerAlias = "e2etest-application"
	relayerSignerAlias     = "e2etest-relayer"
)

// Signers are the two independent local identities used by the acceptance
// tests. Their credentials remain private to this package.
type Signers struct {
	application localSigner
	relayer     localSigner
}

// SignerAddresses identifies the test actors without exposing their keys.
type SignerAddresses struct {
	Application common.Address
	Relayer     common.Address
}

type localSigner struct {
	key     *ecdsa.PrivateKey
	account evm.Account
}

// NewSigners generates independent application and relayer identities.
func NewSigners(t testing.TB) Signers {
	t.Helper()
	application := newLocalSigner(t, "application")
	relayer := newLocalSigner(t, "relayer")
	return Signers{application: application, relayer: relayer}
}

// Addresses returns the public identities of the test actors.
func (s Signers) Addresses() SignerAddresses {
	return SignerAddresses{
		Application: s.application.account.Address(),
		Relayer:     s.relayer.account.Address(),
	}
}

// String renders only public signer addresses.
func (s Signers) String() string {
	addresses := s.Addresses()
	return fmt.Sprintf(
		"e2etest signers (application %s, relayer %s)",
		addresses.Application,
		addresses.Relayer,
	)
}

// GoString renders only public signer addresses, including for %#v formatting.
func (s Signers) GoString() string {
	return s.String()
}

func newLocalSigner(t testing.TB, role string) localSigner {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("e2etest: generate %s signer: %v", role, err)
	}
	account, err := evm.AccountFromHex(hex.EncodeToString(crypto.FromECDSA(key)))
	if err != nil {
		t.Fatalf("e2etest: create %s account: %v", role, err)
	}
	return localSigner{key: key, account: account}
}

func (s Signers) validate() error {
	switch {
	case s.application.key == nil:
		return errors.New("application signer is required")
	case s.relayer.key == nil:
		return errors.New("relayer signer is required")
	default:
		return nil
	}
}

// storeRelayerKey writes the relayer's signing key file and returns its path.
// The relayer signs relay transactions and local attestations with it; the
// application signer never enters the relayer process.
func (s Signers) storeRelayerKey(dir string) (string, error) {
	relayerPath := filepath.Join(dir, "keys", relayerSignerAlias+".json")
	if err := keyfile.Store(relayerPath, keyfile.ECDSA, crypto.FromECDSA(s.relayer.key)); err != nil {
		return "", fmt.Errorf("store relayer signer: %w", err)
	}
	return relayerPath, nil
}
