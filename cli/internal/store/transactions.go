// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"fmt"
	"math"
	"time"
)

// Transaction kinds and journal states.
const (
	TxKindRecv    = "recv"
	TxKindAck     = "ack"
	TxKindTimeout = "timeout"
	TxPending     = "PENDING"
	TxConfirmed   = "CONFIRMED"
	TxReverted    = "REVERTED"
	TxReplaced    = "REPLACED"
	TxConsumed    = "CONSUMED"
)

// TxAttempt retains the signed bytes of one broadcast or fee replacement.
type TxAttempt struct {
	ID              int64
	ChainID         string
	Sender          string
	Hash            string
	Kind            string
	Nonce           uint64
	SignedTx        []byte
	SubmittedAt     time.Time
	LastBroadcastAt *time.Time
	Status          string
}

// TxOutcome resolves a nonce, optionally identifying the mined attempt.
type TxOutcome struct {
	ChainID string
	Sender  string
	Nonce   uint64
	Hash    string
	Status  string
}

func (o TxOutcome) Validate() error {
	if o.ChainID == "" || o.Sender == "" || o.Nonce > math.MaxInt64 {
		return fmt.Errorf("invalid transaction outcome")
	}
	switch o.Status {
	case TxConfirmed, TxReverted:
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
	switch a.Kind {
	case TxKindRecv, TxKindAck, TxKindTimeout:
		return nil
	default:
		return fmt.Errorf("invalid transaction kind %q", a.Kind)
	}
}

// SaveTxAttempt commits nonce ownership and packet links before any broadcast.
func SaveTxAttempt(ctx context.Context, db Store, attempt TxAttempt, packets []PacketKey) (int64, error) {
	var id int64
	err := db.Transact(ctx, func(repo Repository) error {
		var err error
		id, err = repo.InsertTxAttempt(ctx, attempt)
		if err != nil {
			return err
		}
		for _, key := range packets {
			if err = repo.LinkTxPacket(ctx, id, key); err != nil {
				return err
			}
		}
		return nil
	})
	return id, err
}
