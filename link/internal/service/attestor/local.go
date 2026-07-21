package attestor

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/cosmos/ibc/link/internal/service/signer"
)

// LocalAttestor provides attestation data from the local process.
// Right now we support only EVM attestations. Then porting Cosmos/Solana attestors, we'd need to refactor
// LocalAttestor into LocalEVMAttestor, LocalCosmosAttestor, LocalSolanaAttestor, etc.
type LocalAttestor struct {
	chainID string
	name    string
	signer  signer.Signer
	logger  *slog.Logger
}

// Client is a chain client used by a local attestor.
type Client interface {
	// todo
	ChainID() string
}

var _ Attestor = &LocalAttestor{}

func NewLocal(chainID, name string, client Client, backingSigner signer.Signer) (*LocalAttestor, error) {
	switch {
	case chainID == "":
		return nil, fmt.Errorf("chainID required")
	case name == "":
		return nil, fmt.Errorf("name required")
	case client == nil:
		return nil, fmt.Errorf("client required")
	case client.ChainID() != chainID:
		return nil, fmt.Errorf("client chainID mismatch: got %s, want %s", client.ChainID(), chainID)
	case backingSigner == nil:
		return nil, fmt.Errorf("signer required")
	case backingSigner.Type() != signer.ECDSA:
		return nil, fmt.Errorf("ECDSA signer required, got %s", backingSigner.Type())
	}

	logger := slog.With("module", "attestor", "name", attestorFQN("local", chainID, name))

	return &LocalAttestor{
		chainID: chainID,
		name:    name,
		signer:  backingSigner,
		logger:  logger,
	}, nil
}

func (a *LocalAttestor) LatestAttestableHeight(_ context.Context) (uint64, error) {
	// todo: mocked
	return uint64(time.Now().Unix()), nil
}

func (a *LocalAttestor) StateAttestation(_ context.Context, height uint64) (Attestation, error) {
	// todo: mocked
	return Attestation{
		Height: height,
	}, nil
}

func (a *LocalAttestor) PacketAttestation(_ context.Context, req PacketAttestationRequest) (Attestation, error) {
	// todo: mocked
	return Attestation{
		Height: req.Height,
	}, nil
}

// name and alias are identical for local attestors
func (a *LocalAttestor) Name() string    { return a.name }
func (a *LocalAttestor) Alias() string   { return a.name }
func (a *LocalAttestor) ChainID() string { return a.chainID }
func (a *LocalAttestor) IsLocal() bool   { return true }

func attestorFQN(connection, chainID, name string) string {
	return fmt.Sprintf("%s-%s-%s", chainID, connection, name)
}
