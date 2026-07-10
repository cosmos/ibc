package attestor

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/cosmos/ibc/link/internal/service/signer"
)

// LocalAttestor provides attestation data from the local process.
type LocalAttestor struct {
	chainID string
	name    string
	signer  signer.Signer
	logger  *slog.Logger
}

var _ Attestor = &LocalAttestor{}

func NewLocal(chainID, name string, backingSigner signer.Signer) (*LocalAttestor, error) {
	switch {
	case chainID == "":
		return nil, fmt.Errorf("chainID required")
	case name == "":
		return nil, fmt.Errorf("name required")
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

// name and alias are identical for local attestors
func (a *LocalAttestor) Name() string    { return a.name }
func (a *LocalAttestor) Alias() string   { return a.name }
func (a *LocalAttestor) ChainID() string { return a.chainID }
func (a *LocalAttestor) IsLocal() bool   { return true }

func attestorFQN(connection, chainID, name string) string {
	return fmt.Sprintf("%s-%s-%s", chainID, connection, name)
}
