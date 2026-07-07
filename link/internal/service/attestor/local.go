package attestor

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// LocalAttestor provides attestation data from the local process.
type LocalAttestor struct {
	chainID string
	name    string
	logger  *slog.Logger
}

var _ Attestor = &LocalAttestor{}

func NewLocal(chainID, name string) *LocalAttestor {
	return &LocalAttestor{
		chainID: chainID,
		name:    name,
		logger:  slog.With("module", "attestor", "name", attestorFQN("local", chainID, name)),
	}
}

func (a *LocalAttestor) Name() string { return a.name }

func (a *LocalAttestor) LatestAttestableHeight(_ context.Context) (uint64, error) {
	// todo: mocked
	return uint64(time.Now().Unix()), nil
}

func attestorFQN(connection, chainID, name string) string {
	return fmt.Sprintf("%s-%s-%s", chainID, connection, name)
}
