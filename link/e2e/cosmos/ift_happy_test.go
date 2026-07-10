// Package cosmos_test proves an anvil-EVM -> cosmos IFT transfer flows through the UNCHANGED harness
// surface (run.IFT + out.VerifyComplete). NO EVM imports here ON PURPOSE (no go-ethereum, no common.*,
// no harness/chain/evm) and NO cosmos-sdk imports either — that absence IS part of the proof: a test
// author drives a genuinely different chain family with the exact same code they use for evm -> evm.
package cosmos_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/e2e/e2etest"
	"github.com/cosmos/ibc/link/harness"
	"github.com/cosmos/ibc/link/harness/topology"
)

// TestIFTTransfer_EVMToCosmos runs the default-receiver happy path: the receiver defaults to a fresh
// cosmos1 bech32 address, and VerifyComplete asserts the terminal floor through the cosmos Reader.
func TestIFTTransfer_EVMToCosmos(t *testing.T) {
	e2etest.RequireAnvilLane(t)
	e2etest.RequireSandboxd(t)

	run := e2etest.Start(t, topology.Anvil(topology.EVMCosmos()))
	ctx := t.Context()

	out, err := run.IFT(ctx, harness.IFT{
		Route:  topology.RouteAtoB,
		Amount: big.NewInt(4_242_000),
	})
	require.NoError(t, err)
	require.NoError(t, out.VerifyComplete(ctx))
}

// TestIFTTransfer_EVMToCosmos_ExplicitReceiver reuses the same flow with a caller-supplied cosmos1
// receiver instead of the default, proving the bech32 string surface end to end.
func TestIFTTransfer_EVMToCosmos_ExplicitReceiver(t *testing.T) {
	e2etest.RequireAnvilLane(t)
	e2etest.RequireSandboxd(t)

	run := e2etest.Start(t, topology.Anvil(topology.EVMCosmos()))
	ctx := t.Context()

	// A fresh, valid cosmos account (bech32 of 20 fixed bytes). It needs no prior funding — a bank MsgSend
	// creates the recipient account on first receipt — which is exactly the non-EVM receiver difference.
	const receiver = "cosmos14w46h2at4w46h2at4w46h2at4w46h2atuw643a"

	out, err := run.IFT(ctx, harness.IFT{
		Route:    topology.RouteAtoB,
		Amount:   big.NewInt(7_000_001),
		Receiver: receiver,
	})
	require.NoError(t, err)
	require.NoError(t, out.VerifyComplete(ctx))
}
