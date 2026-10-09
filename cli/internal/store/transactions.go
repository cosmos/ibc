// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"fmt"
	"math"
	"time"
)

// Operation types and journal states.
const (
	OperationTypeRecv    = "recv"
	OperationTypeAck     = "ack"
	OperationTypeTimeout = "timeout"
	TxPending            = "PENDING"
	TxAbandoned          = "ABANDONED"
	TxConfirmed          = "CONFIRMED"
	TxFailed             = "FAILED"
	TxReplaced           = "REPLACED"
	TxConsumed           = "CONSUMED"
)

// TxAttempt retains the signed bytes of one broadcast or fee replacement.
type TxAttempt struct {
	ID              int64
	ChainID         string
	Sender          string
	Hash            string
	Nonce           uint64
	SignedTx        []byte
	SubmittedAt     time.Time
	LastBroadcastAt *time.Time
	Status          string
}

// TxOperation associates a packet action with a transaction attempt.
type TxOperation struct {
	PacketKey
	Type string
}

func (o TxOperation) Validate() error {
	if o.SourceChainID == "" || o.SourceClientID == "" || o.Sequence == 0 || o.Sequence > math.MaxInt64 {
		return fmt.Errorf("invalid operation packet key")
	}
	switch o.Type {
	case OperationTypeRecv, OperationTypeAck, OperationTypeTimeout:
		return nil
	default:
		return fmt.Errorf("invalid operation type %q", o.Type)
	}
}

// TxOutcome resolves a nonce, optionally identifying the mined attempt.
type TxOutcome struct {
	ChainID     string
	Sender      string
	Nonce       uint64
	Hash        string
	Status      string
	BlockNumber uint64
	BlockHash   string
}

// TxResolution identifies the chain state in which a nonce was consumed.
type TxResolution struct {
	Nonce       uint64
	BlockNumber uint64
	BlockHash   string
}

func (r TxResolution) Validate() error {
	if r.Nonce > math.MaxInt64 || r.BlockNumber > math.MaxInt64 || r.BlockHash == "" {
		return fmt.Errorf("invalid transaction resolution")
	}
	return nil
}

func (o TxOutcome) Validate() error {
	if o.ChainID == "" || o.Sender == "" || o.Nonce > math.MaxInt64 {
		return fmt.Errorf("invalid transaction outcome")
	}
	if err := (TxResolution{Nonce: o.Nonce, BlockNumber: o.BlockNumber, BlockHash: o.BlockHash}).Validate(); err != nil {
		return err
	}
	switch o.Status {
	case TxConfirmed, TxFailed:
		if o.Hash != "" {
			return nil
		}
	case TxConsumed:
		if o.Hash == "" {
			return nil
		}
	}
	return fmt.Errorf("invalid transaction outcome %q with hash %q", o.Status, o.Hash)
}

func (a TxAttempt) Validate() error {
	if a.ChainID == "" || a.Sender == "" || a.Hash == "" || len(a.SignedTx) == 0 || a.SubmittedAt.IsZero() ||
		a.Nonce > math.MaxInt64 {
		return fmt.Errorf("invalid transaction attempt")
	}
	return nil
}

// SaveTxAttempt commits nonce ownership and operations before any broadcast.
func SaveTxAttempt(ctx context.Context, db Store, attempt TxAttempt, operations []TxOperation) (int64, error) {
	var id int64
	err := db.Transact(ctx, func(repo Repository) error {
		var err error
		id, err = repo.InsertTxAttempt(ctx, attempt)
		if err != nil {
			return err
		}
		for _, operation := range operations {
			if err = repo.InsertTxOperation(ctx, id, operation); err != nil {
				return err
			}
		}
		return nil
	})
	return id, err
}
