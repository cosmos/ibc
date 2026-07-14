// Package txmgr signs and broadcasts transactions for the relayer.
package txmgr

import (
	"context"
	"time"
)

// Submitter signs and broadcasts transactions; implementations own nonce
// selection and gas pricing.
type Submitter interface {
	Submit(ctx context.Context, chainID string, intent TxIntent) (*Submission, error)

	// ShouldRetry reports whether a transaction submitted at sentAt is failed
	// or has been pending past expiry and should be resubmitted.
	ShouldRetry(
		ctx context.Context,
		chainID string,
		txHash string,
		expiry time.Duration,
		sentAt time.Time,
	) (bool, error)
}

// TxIntent a transaction to submit.
type TxIntent struct {
	To   string
	Data []byte
}

// Submission a broadcast transaction.
type Submission struct {
	TxHash         string
	SubmittedAt    time.Time
	RelayerAddress string
}
