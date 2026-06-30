package store

import (
	"context"
	"log/slog"
	"time"

	repopostgres "github.com/cosmos/ibc/link/internal/store/repository/postgres"
	reposqlite "github.com/cosmos/ibc/link/internal/store/repository/sqlite"
)

type repositoryStore struct {
	backend relaySubmissionBackend
	logger  *slog.Logger
}

var _ Repository = (*repositoryStore)(nil)

type relaySubmissionBackend interface {
	getRelaySubmission(ctx context.Context, chainID string, txHash string) (*RelaySubmission, error)
	upsertRelaySubmission(ctx context.Context, chainID string, txHash string) error
}

func newRepositoryStore(backend relaySubmissionBackend, logger *slog.Logger) *repositoryStore {
	return &repositoryStore{
		backend: backend,
		logger:  logger,
	}
}

func (s *repositoryStore) GetRelaySubmission(
	ctx context.Context,
	chainID string,
	txHash string,
) (*RelaySubmission, error) {
	s.logger.Debug("GetRelaySubmission", "chainID", chainID, "txHash", txHash)

	if chainID == "" || txHash == "" {
		return nil, ErrMissingChainTx
	}

	entry, err := s.backend.getRelaySubmission(ctx, chainID, txHash)
	if err != nil {
		return nil, errNormalize(err)
	}

	return entry, nil
}

func (s *repositoryStore) UpsertRelaySubmission(ctx context.Context, chainID string, txHash string) error {
	s.logger.Debug("UpsertRelaySubmission", "chainID", chainID, "txHash", txHash)

	if chainID == "" || txHash == "" {
		return ErrMissingChainTx
	}

	return s.backend.upsertRelaySubmission(ctx, chainID, txHash)
}

type postgresRepository struct {
	queries *repopostgres.Queries
}

func (r postgresRepository) getRelaySubmission(
	ctx context.Context,
	chainID string,
	txHash string,
) (*RelaySubmission, error) {
	entry, err := r.queries.GetRelaySubmission(ctx, chainID, txHash)
	if err != nil {
		return nil, err
	}

	return newRelaySubmission(entry.ID, entry.SourceChainID, entry.SourceTxHash, entry.CreatedAt), nil
}

func (r postgresRepository) upsertRelaySubmission(ctx context.Context, chainID string, txHash string) error {
	return r.queries.UpsertRelaySubmission(ctx, chainID, txHash)
}

type sqliteRepository struct {
	queries *reposqlite.Queries
}

func (r sqliteRepository) getRelaySubmission(
	ctx context.Context,
	chainID string,
	txHash string,
) (*RelaySubmission, error) {
	entry, err := r.queries.GetRelaySubmission(ctx, chainID, txHash)
	if err != nil {
		return nil, err
	}

	return newRelaySubmission(entry.ID, entry.SourceChainID, entry.SourceTxHash, entry.CreatedAt), nil
}

func (r sqliteRepository) upsertRelaySubmission(ctx context.Context, chainID string, txHash string) error {
	return r.queries.UpsertRelaySubmission(ctx, chainID, txHash)
}

func newRelaySubmission(id int64, chainID string, txHash string, createdAt time.Time) *RelaySubmission {
	return &RelaySubmission{
		ID:        id,
		ChainID:   chainID,
		TxHash:    txHash,
		CreatedAt: createdAt.UTC(),
	}
}
