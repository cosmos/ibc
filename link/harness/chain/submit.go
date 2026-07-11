package chain

import (
	"context"
	"math/big"
)

type AppSubmitter interface {
	SubmitIFT(ctx context.Context, in IFTSubmission) (AppSubmitResult, error)
	SubmitGMP(ctx context.Context, in GMPSubmission) (AppSubmitResult, error)
}

type IFTSubmission struct {
	RouteID          string
	Receiver         string
	Amount           *big.Int
	TimeoutTimestamp uint64
}

type GMPSubmission struct {
	RouteID string
	Target  string
	Payload []byte
}

type AppSubmitResult struct {
	SourceTxHash string
	Sequence     uint64
}
