package onchain

import (
	"context"
	"math/big"
	"time"
)

type Budget struct {
	Completion time.Duration
	Poll       time.Duration
	StatusRow  time.Duration
}

type Reader interface {
	AwaitIFTReceived(ctx context.Context, routeID string, seq uint64) (IFTReceived, error)

	AwaitIFTRefunded(ctx context.Context, seq uint64) (IFTRefunded, error)

	AwaitGMPReceived(ctx context.Context, routeID string, seq uint64) (GMPReceived, error)

	IFTBalance(ctx context.Context, holder string) (*big.Int, error)

	GMPCount(ctx context.Context, target string) (*big.Int, error)

	GMPDefaultPayload() []byte

	CanonicalAddr(s string) (string, error)

	Budget() Budget
}

type IFTReceived struct {
	Receiver string
	Amount   *big.Int
}

type IFTRefunded struct {
	Amount *big.Int
}

type GMPReceived struct {
	Target  string
	Success bool
}
