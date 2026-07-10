package onchain

import (
	"context"
	"math/big"
	"time"
)

// Budget is the timing a Reader needs to bound its waits: how long to wait for an on-chain effect and at
// what cadence to poll for it, plus how long the daemon's status row may trail an effect the Reader has
// already observed. It is the onchain package's own view of a chain's timing profile (the harness resolves
// a topology.TimingProfile into this at reader construction), so onchain stays free of the topology package.
type Budget struct {
	// Completion bounds the wait for the destination mint/delivery or source refund effect.
	Completion time.Duration
	// Poll is the effect-scan / status re-probe cadence shared by effect waits and StatusCrossCheck.
	Poll time.Duration
	// StatusRow bounds StatusCrossCheck — the daemon's persist lag behind an effect the Reader has
	// already observed (see wire.PacketStatus for the ordering contract).
	StatusRow time.Duration
}

// Reader is one chain's independent read surface: it reads packet effects and app state
// straight from that chain via the harness's own client and deployed fixtures,
// never the relayer. It is the per-chain-family seam of this package: the
// Tracker and the app asserters go through Reader for every on-chain read. A new
// chain family implements Reader plus two composition-root registrations — its
// construction arm in the harness's buildReaders and its launcher arm in
// provision (and any family-specific wire config fields in topology.Compile);
// the correlation logic and the harness surface stay untouched. One Reader is
// bound to one chain and that chain's deployed fixtures.
type Reader interface {
	// AwaitIFTReceived waits (bounded by ctx and the reader's completion budget) for the destination IFT
	// mint effect for the packet. routeID + seq identify it; each family correlates using its native packet
	// identity or its destination fixture's route-scoped sequence.
	AwaitIFTReceived(ctx context.Context, routeID string, seq uint64) (IFTReceived, error)

	// AwaitIFTRefunded waits for the source escrow-refund effect carrying seq (IFT timeout leg). The refund
	// is a source-side effect keyed by the raw sequence, which is already unique per source fixture, so it
	// takes no route scoping.
	AwaitIFTRefunded(ctx context.Context, seq uint64) (IFTRefunded, error)

	// AwaitGMPReceived waits for the destination GMP delivery effect for the packet, identified by routeID +
	// seq using the family-specific correlation mechanism.
	AwaitGMPReceived(ctx context.Context, routeID string, seq uint64) (GMPReceived, error)

	// IFTBalance reads the IFT token balance of holder (a family-native address string) at the chain's
	// deployed IFT fixture.
	IFTBalance(ctx context.Context, holder string) (*big.Int, error)

	// GMPCount reads the GMP target's count/observable state at target (a family-native address string).
	GMPCount(ctx context.Context, target string) (*big.Int, error)

	// GMPDefaultPayload returns the family-native default GMP call payload bytes the happy-path GMP action
	// submits when a test does not supply its own. It is inherently family-specific, so it lives behind the
	// reader rather than as a package-level helper in the harness's family-agnostic gmp.go.
	GMPDefaultPayload() []byte

	// CanonicalAddr validates s as a family-native address string and returns its canonical form (EVM:
	// EIP-55 checksummed hex; cosmos: validated bech32, re-encoded). It is how the family-agnostic harness
	// surface compares two address strings for equality — canonical forms compare byte-for-byte — without
	// carrying a family switch of its own. A malformed address is an error, never a silent coercion.
	CanonicalAddr(s string) (string, error)

	// Budget is the timing this Reader was built with — the chain's completion/poll/status-row budgets. The
	// Tracker reads the destination Reader's budget to bound StatusCrossCheck, so the status-row wait derives
	// from the same per-chain profile as the on-chain wait rather than a separate literal.
	Budget() Budget
}

// IFTReceived is the family-normalized view of an IFT mint effect. Address fields are canonical
// family-native strings.
type IFTReceived struct {
	Receiver string
	Amount   *big.Int
}

// IFTRefunded is the family-normalized view of an IFT escrow-refund effect.
type IFTRefunded struct {
	Amount *big.Int
}

// GMPReceived is the family-normalized view of a GMP delivery effect. Address fields are canonical
// family-native strings.
type GMPReceived struct {
	Target  string
	Success bool
}
