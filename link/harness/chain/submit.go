package chain

import (
	"context"
	"math/big"
)

// AppSubmitter submits IFT and GMP actions in a chain's native form. Each submitter is bound to one
// source chain and its deployed fixtures.
type AppSubmitter interface {
	// SubmitIFT escrows an IFT transfer and returns its transaction hash and source sequence.
	SubmitIFT(ctx context.Context, in IFTSubmission) (AppSubmitResult, error)
	// SubmitGMP sends a GMP message and returns its transaction hash and source sequence.
	SubmitGMP(ctx context.Context, in GMPSubmission) (AppSubmitResult, error)
}

// IFTSubmission is a family-agnostic IFT escrow request. Receiver is the destination-native account
// string (ICS20 form: an EVM 0x hex or a cosmos1 bech32), carried verbatim into the source action.
type IFTSubmission struct {
	RouteID          string
	Receiver         string
	Amount           *big.Int
	TimeoutTimestamp uint64 // absolute unix seconds; 0 asks the chain family for its default.
}

// GMPSubmission is a family-agnostic GMP send request. Target is the destination-native account string;
// Payload is the raw destination calldata.
type GMPSubmission struct {
	RouteID string
	Target  string
	Payload []byte
}

// AppSubmitResult is the outcome of a source-side submission: the source tx hash and the packet's source
// sequence, from which the harness derives the packet id via wire.PacketID.
type AppSubmitResult struct {
	SourceTxHash string
	Sequence     uint64
}
