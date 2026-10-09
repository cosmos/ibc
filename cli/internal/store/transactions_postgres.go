// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repo "github.com/cosmos/ibc/cli/internal/store/repository/postgres"
)

func (db *PostgresDB) InsertTxAttempt(ctx context.Context, a TxAttempt) (int64, error) {
	if err := a.Validate(); err != nil {
		return 0, err
	}
	id, err := db.repo.InsertTxAttempt(ctx, repo.InsertTxAttemptParams{
		TxHash: a.Hash, ChainID: a.ChainID, TxType: a.Kind, RelayerAddress: a.Sender,
		SubmittedAt: pgtype.Timestamptz{Time: a.SubmittedAt, Valid: true}, Nonce: int64(a.Nonce), SignedTx: a.SignedTx,
	})
	return id, errNormalize(err)
}

func (db *PostgresDB) LinkTxPacket(ctx context.Context, id int64, key PacketKey) error {
	count, err := db.repo.LinkTxPacket(
		ctx,
		repo.LinkTxPacketParams{
			SubmissionID:   id,
			SourceChainID:  key.SourceChainID,
			SourceClientID: key.SourceClientID,
			Sequence:       int64(key.Sequence),
		},
	)
	if err != nil {
		return errNormalize(err)
	}
	if count != 1 {
		return fmt.Errorf("packet %v not found for transaction %d", key, id)
	}
	return nil
}

func (db *PostgresDB) ListTxPackets(ctx context.Context, id int64) ([]PacketKey, error) {
	rows, err := db.repo.ListTxPackets(ctx, id)
	if err != nil {
		return nil, errNormalize(err)
	}
	keys := make([]PacketKey, 0, len(rows))
	for _, r := range rows {
		keys = append(keys, PacketKey{r.SourceChainID, r.PacketSourceClientID, uint64(r.PacketSequenceNumber)})
	}
	return keys, nil
}

func (db *PostgresDB) ListPendingTxAttempts(ctx context.Context, chainID, sender string) ([]TxAttempt, error) {
	rows, err := db.repo.ListPendingTxAttempts(ctx, chainID, sender)
	if err != nil {
		return nil, errNormalize(err)
	}
	attempts := make([]TxAttempt, 0, len(rows))
	for _, r := range rows {
		attempts = append(attempts, txAttemptPostgresDB(r))
	}
	return attempts, nil
}

func (db *PostgresDB) GetTxAttempt(ctx context.Context, chainID, hash string) (TxAttempt, error) {
	row, err := db.repo.GetTxAttempt(ctx, chainID, hash)
	return txAttemptPostgresDB(row), errNormalize(err)
}

func (db *PostgresDB) ListTxNoncesFrom(
	ctx context.Context,
	chainID, sender string,
	minedNonce uint64,
) ([]uint64, error) {
	rows, err := db.repo.ListTxNoncesFrom(ctx, repo.ListTxNoncesFromParams{
		ChainID: chainID, RelayerAddress: sender, MinedNonce: int64(minedNonce),
	})
	if err != nil {
		return nil, errNormalize(err)
	}
	nonces := make([]uint64, len(rows))
	for i, nonce := range rows {
		nonces[i] = uint64(nonce)
	}
	return nonces, nil
}

func (db *PostgresDB) ListTxResolutionsFrom(
	ctx context.Context,
	chainID, sender string,
	minedNonce uint64,
) ([]TxResolution, error) {
	rows, err := db.repo.ListTxResolutionsFrom(ctx, repo.ListTxResolutionsFromParams{
		ChainID: chainID, RelayerAddress: sender, MinedNonce: int64(minedNonce),
	})
	if err != nil {
		return nil, errNormalize(err)
	}
	resolutions := make([]TxResolution, len(rows))
	for i, r := range rows {
		resolutions[i] = TxResolution{
			Nonce:       uint64(r.Nonce),
			BlockNumber: uint64(*r.ResolvedBlockNumber),
			BlockHash:   *r.ResolvedBlockHash,
		}
	}
	return resolutions, nil
}

func (db *PostgresDB) ReopenTxAttempts(ctx context.Context, chainID, sender string, resolution TxResolution) error {
	if err := resolution.Validate(); err != nil {
		return err
	}
	number := int64(resolution.BlockNumber)
	return errNormalize(db.repo.ReopenTxAttempts(ctx, repo.ReopenTxAttemptsParams{
		ChainID: chainID, RelayerAddress: sender, Nonce: int64(resolution.Nonce),
		ResolvedBlockNumber: &number, ResolvedBlockHash: &resolution.BlockHash,
	}))
}

func (db *PostgresDB) HasPendingPacketSubmission(ctx context.Context, key PacketKey, kind string) (bool, error) {
	pending, err := db.repo.HasPendingPacketSubmission(ctx, repo.HasPendingPacketSubmissionParams{
		SourceChainID: key.SourceChainID, SourceClientID: key.SourceClientID,
		Sequence: int64(key.Sequence), TxType: kind,
	})
	return pending, errNormalize(err)
}

func (db *PostgresDB) ResolveTxNonce(ctx context.Context, outcome TxOutcome) error {
	if err := outcome.Validate(); err != nil {
		return err
	}
	at := time.Now().UTC()
	number := int64(outcome.BlockNumber)
	return errNormalize(db.repo.ResolveTxNonce(ctx, repo.ResolveTxNonceParams{
		ResolvedBlockNumber: &number, ResolvedBlockHash: &outcome.BlockHash,
		ChainID:        outcome.ChainID,
		RelayerAddress: outcome.Sender,
		Nonce:          int64(outcome.Nonce),
		TxHash:         outcome.Hash,
		Status:         outcome.Status,
		ResolvedAt:     pgtype.Timestamptz{Time: at, Valid: true},
	}))
}

func txAttemptPostgresDB(r repo.RelayerTxSubmission) TxAttempt {
	var lastBroadcast *time.Time
	if r.LastBroadcastAt.Valid {
		lastBroadcast = &r.LastBroadcastAt.Time
	}
	return TxAttempt{
		ID:              r.ID,
		ChainID:         r.ChainID,
		Sender:          r.RelayerAddress,
		Hash:            r.TxHash,
		Kind:            r.TxType,
		Nonce:           uint64(r.Nonce),
		SignedTx:        r.SignedTx,
		SubmittedAt:     r.SubmittedAt.Time,
		LastBroadcastAt: lastBroadcast,
		Status:          r.Status,
	}
}

func (db *PostgresDB) SetTxBroadcast(ctx context.Context, chainID, hash string, at time.Time) error {
	return errNormalize(db.repo.SetTxBroadcast(ctx, repo.SetTxBroadcastParams{
		ChainID: chainID, TxHash: hash, LastBroadcastAt: pgtype.Timestamptz{Time: at, Valid: true},
	}))
}
