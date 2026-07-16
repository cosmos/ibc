// Package txmgr signs and broadcasts transactions for the relayer.
package txmgr

import (
	"context"
	"time"
)

// Submitter signs and broadcasts transactions on a single chain;
// implementations own nonce selection and gas pricing.
type Submitter interface {
	Submit(ctx context.Context, intent TxIntent) (*Submission, error)

	// ShouldRetry reports whether a transaction submitted at sentAt is failed
	// or has been pending past expiry and should be resubmitted.
	ShouldRetry(ctx context.Context, txHash string, expiry time.Duration, sentAt time.Time) (bool, error)
}

// SubmitterSet holds the submitter for every chain relayed by the configured
// routes.
type SubmitterSet struct {
	submitters map[string]Submitter
}

func NewSubmitterSet(submitters map[string]Submitter) *SubmitterSet {
	if submitters == nil {
		submitters = make(map[string]Submitter)
	}

	return &SubmitterSet{submitters: submitters}
}

func (s *SubmitterSet) Get(chainID string) (Submitter, bool) {
	submitter, ok := s.submitters[chainID]
	return submitter, ok
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
