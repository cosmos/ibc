// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"fmt"
	"time"

	repo "github.com/cosmos/ibc/cli/internal/store/repository/sqlite"
)

func (db *SqliteDB) InsertTxAttempt(ctx context.Context, a TxAttempt) (int64, error) {
	if err := a.Validate(); err != nil {
		return 0, err
	}
	id, err := db.repo.InsertTxAttempt(ctx, repo.InsertTxAttemptParams{
		TxHash: a.Hash, ChainID: a.ChainID, RelayerAddress: a.Sender,
		SubmittedAt: a.SubmittedAt, Nonce: int64(a.Nonce), SignedTx: a.SignedTx,
	})
	return id, errNormalize(err)
}

func (db *SqliteDB) InsertTxOperation(ctx context.Context, id int64, operation TxOperation) error {
	if err := operation.Validate(); err != nil {
		return err
	}
	count, err := db.repo.InsertTxOperation(
		ctx,
		repo.InsertTxOperationParams{
			SubmissionID:   id,
			SourceChainID:  operation.SourceChainID,
			SourceClientID: operation.SourceClientID,
			Sequence:       int64(operation.Sequence),
			Type:           operation.Type,
		},
	)
	if err != nil {
		return errNormalize(err)
	}
	if count != 1 {
		return fmt.Errorf("packet %v not found for transaction %d", operation.PacketKey, id)
	}
	return nil
}

func (db *SqliteDB) ListTxOperations(ctx context.Context, id int64) ([]TxOperation, error) {
	rows, err := db.repo.ListTxOperations(ctx, id)
	if err != nil {
		return nil, errNormalize(err)
	}
	operations := make([]TxOperation, 0, len(rows))
	for _, r := range rows {
		operations = append(operations, TxOperation{
			PacketKey: PacketKey{r.SourceChainID, r.PacketSourceClientID, uint64(r.PacketSequenceNumber)},
			Type:      r.Type,
		})
	}
	return operations, nil
}

func (db *SqliteDB) ListUnresolvedTxAttempts(
	ctx context.Context,
	chainID, sender string,
	minedNonce uint64,
) ([]TxAttempt, error) {
	rows, err := db.repo.ListUnresolvedTxAttempts(ctx, repo.ListUnresolvedTxAttemptsParams{
		ChainID: chainID, RelayerAddress: sender, MinedNonce: int64(minedNonce),
	})
	if err != nil {
		return nil, errNormalize(err)
	}
	attempts := make([]TxAttempt, 0, len(rows))
	for _, r := range rows {
		attempts = append(attempts, txAttemptSqliteDB(r))
	}
	return attempts, nil
}

func (db *SqliteDB) GetTxAttempt(ctx context.Context, chainID, hash string) (TxAttempt, error) {
	row, err := db.repo.GetTxAttempt(ctx, chainID, hash)
	return txAttemptSqliteDB(row), errNormalize(err)
}

func (db *SqliteDB) ListTxNoncesFrom(ctx context.Context, chainID, sender string, minedNonce uint64) ([]uint64, error) {
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

func (db *SqliteDB) ListTxResolutionsFrom(
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

func (db *SqliteDB) ReopenTxAttempts(ctx context.Context, chainID, sender string, resolution TxResolution) error {
	if err := resolution.Validate(); err != nil {
		return err
	}
	number := int64(resolution.BlockNumber)
	return errNormalize(db.repo.ReopenTxAttempts(ctx, repo.ReopenTxAttemptsParams{
		ChainID: chainID, RelayerAddress: sender, Nonce: int64(resolution.Nonce),
		ResolvedBlockNumber: &number, ResolvedBlockHash: &resolution.BlockHash,
	}))
}

func (db *SqliteDB) HasPendingOperation(ctx context.Context, operation TxOperation) (bool, error) {
	if err := operation.Validate(); err != nil {
		return false, err
	}
	pending, err := db.repo.HasPendingOperation(ctx, repo.HasPendingOperationParams{
		SourceChainID: operation.SourceChainID, SourceClientID: operation.SourceClientID,
		Sequence: int64(operation.Sequence), Type: operation.Type,
	})
	return pending != 0, errNormalize(err)
}

func (db *SqliteDB) ResolveTxNonce(ctx context.Context, outcome TxOutcome) error {
	if err := outcome.Validate(); err != nil {
		return err
	}
	at := time.Now().UTC()
	number := int64(outcome.BlockNumber)
	return errNormalize(db.repo.ResolveTxNonce(ctx, repo.ResolveTxNonceParams{
		ResolvedBlockNumber: &number, ResolvedBlockHash: &outcome.BlockHash,
		ChainID: outcome.ChainID, RelayerAddress: outcome.Sender, Nonce: int64(outcome.Nonce),
		TxHash: outcome.Hash, Status: outcome.Status, ResolvedAt: &at,
	}))
}

func txAttemptSqliteDB(r repo.RelayerTxSubmission) TxAttempt {
	return TxAttempt{
		ID:              r.ID,
		ChainID:         r.ChainID,
		Sender:          r.RelayerAddress,
		Hash:            r.TxHash,
		Nonce:           uint64(r.Nonce),
		SignedTx:        r.SignedTx,
		SubmittedAt:     r.SubmittedAt,
		LastBroadcastAt: r.LastBroadcastAt,
		Status:          r.Status,
	}
}

func (db *SqliteDB) SetTxBroadcast(ctx context.Context, chainID, hash string, at time.Time) error {
	return errNormalize(db.repo.SetTxBroadcast(ctx, repo.SetTxBroadcastParams{
		ChainID: chainID, TxHash: hash, LastBroadcastAt: &at,
	}))
}

func (db *SqliteDB) AbandonTxAttempts(ctx context.Context, chainID, sender string, nonce uint64) error {
	return errNormalize(db.repo.AbandonTxAttempts(ctx, repo.AbandonTxAttemptsParams{
		ChainID: chainID, RelayerAddress: sender, Nonce: int64(nonce),
	}))
}
