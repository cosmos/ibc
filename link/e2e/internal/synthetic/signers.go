package synthetic

import (
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/cosmos/ibc/link/harness/chain/evm"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/internal/service/signer/keyfile"
)

const (
	applicationSignerAlias = "synthetic-application"
	relayerSignerAlias     = "synthetic-relayer"
)

// Signers are the two independent local identities used by the synthetic
// acceptance path. Their credentials remain private to this package.
type Signers struct {
	application localSigner
	relayer     localSigner
}

// SignerAddresses identifies the synthetic actors without exposing their keys.
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

// Addresses returns the public identities of the synthetic actors.
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
		"synthetic signers (application %s, relayer %s)",
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
		t.Fatalf("synthetic: generate %s signer: %v", role, err)
	}
	account, err := evm.AccountFromHex(hex.EncodeToString(crypto.FromECDSA(key)))
	if err != nil {
		t.Fatalf("synthetic: create %s account: %v", role, err)
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

func (s Signers) store(dir string) ([]wire.Signer, error) {
	appPath := filepath.Join(dir, "keys", applicationSignerAlias+".json")
	relayerPath := filepath.Join(dir, "keys", relayerSignerAlias+".json")
	if err := keyfile.Store(appPath, keyfile.ECDSA, crypto.FromECDSA(s.application.key)); err != nil {
		return nil, fmt.Errorf("store application signer: %w", err)
	}
	if err := keyfile.Store(relayerPath, keyfile.ECDSA, crypto.FromECDSA(s.relayer.key)); err != nil {
		return nil, fmt.Errorf("store relayer signer: %w", err)
	}
	return []wire.Signer{
		{Alias: applicationSignerAlias, Type: wire.SignerTypeLocal, File: appPath},
		{Alias: relayerSignerAlias, Type: wire.SignerTypeLocal, File: relayerPath},
	}, nil
}
