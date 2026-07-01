package store

import (
	"context"
	"fmt"
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
	getRelaySubmission(ctx context.Context, key RelaySubmissionKey) (*RelaySubmission, error)
	upsertRelaySubmission(ctx context.Context, key RelaySubmissionKey) error
}

func newRepositoryStore(backend relaySubmissionBackend, logger *slog.Logger) *repositoryStore {
	return &repositoryStore{
		backend: backend,
		logger:  logger,
	}
}

func (s *repositoryStore) GetRelaySubmission(
	ctx context.Context,
	key RelaySubmissionKey,
) (*RelaySubmission, error) {
	s.logger.Debug("GetRelaySubmission", "chainID", key.ChainID, "txHash", key.TxHash)

	if err := key.Validate(); err != nil {
		return nil, err
	}

	entry, err := s.backend.getRelaySubmission(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("get relay submission: %w", errNormalize(err))
	}

	return entry, nil
}

func (s *repositoryStore) UpsertRelaySubmission(ctx context.Context, key RelaySubmissionKey) error {
	s.logger.Debug("UpsertRelaySubmission", "chainID", key.ChainID, "txHash", key.TxHash)

	if err := key.Validate(); err != nil {
		return err
	}

	if err := s.backend.upsertRelaySubmission(ctx, key); err != nil {
		return fmt.Errorf("upsert relay submission: %w", err)
	}

	return nil
}

type postgresRepository struct {
	queries *repopostgres.Queries
}

func (r postgresRepository) getRelaySubmission(
	ctx context.Context,
	key RelaySubmissionKey,
) (*RelaySubmission, error) {
	entry, err := r.queries.GetRelaySubmission(ctx, key.ChainID, key.TxHash)
	if err != nil {
		return nil, err
	}

	return newRelaySubmission(entry.ID, entry.SourceChainID, entry.SourceTxHash, entry.CreatedAt), nil
}

func (r postgresRepository) upsertRelaySubmission(ctx context.Context, key RelaySubmissionKey) error {
	return r.queries.UpsertRelaySubmission(ctx, key.ChainID, key.TxHash)
}

type sqliteRepository struct {
	queries *reposqlite.Queries
}

func (r sqliteRepository) getRelaySubmission(
	ctx context.Context,
	key RelaySubmissionKey,
) (*RelaySubmission, error) {
	entry, err := r.queries.GetRelaySubmission(ctx, key.ChainID, key.TxHash)
	if err != nil {
		return nil, err
	}

	return newRelaySubmission(entry.ID, entry.SourceChainID, entry.SourceTxHash, entry.CreatedAt), nil
}

func (r sqliteRepository) upsertRelaySubmission(ctx context.Context, key RelaySubmissionKey) error {
	return r.queries.UpsertRelaySubmission(ctx, key.ChainID, key.TxHash)
}

func newRelaySubmission(id int64, chainID string, txHash string, createdAt time.Time) *RelaySubmission {
	return &RelaySubmission{
		ID:        id,
		ChainID:   chainID,
		TxHash:    txHash,
		CreatedAt: createdAt.UTC(),
	}
}
