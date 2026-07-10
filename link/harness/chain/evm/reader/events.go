package reader

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/cosmos/ibc/link/harness/fixtures"
)

// eventIFTReceived is the MockIFT event the relayer emits on the destination when it mints to the
// receiver — the on-chain effect the correlator keys its terminal assertion off.
const eventIFTReceived = "IFTReceived"

// mockIFTABI is the parsed MockIFT fixture ABI (decoded from the embedded artifact). It is the same
// fixture the stub deploys/relays, but reached through the shared fixtures package — not the stub's
// walled internals — so the harness decodes the event independently of the black box. Parsed once at
// init: the embedded artifact is build-time-fixed, so a parse failure is a packaging bug, not runtime.
var mockIFTABI = fixtures.MockIFT.MustABI()

// iftReceivedLog is the decoded MockIFT.IFTReceived event body. All three fields are non-indexed (they
// live in the log data), matching the fixture's emit.
type iftReceivedLog struct {
	Seq      *big.Int
	Receiver common.Address
	Amount   *big.Int
}

// decodeIFTReceived unpacks an IFTReceived log's data into the typed struct.
func decodeIFTReceived(data []byte) (iftReceivedLog, error) {
	var ev iftReceivedLog
	if err := mockIFTABI.UnpackIntoInterface(&ev, eventIFTReceived, data); err != nil {
		return iftReceivedLog{}, fmt.Errorf("decode IFTReceived: %w", err)
	}
	return ev, nil
}

// eventIFTRefunded is the MockIFT event the relayer emits on the source when it refunds a timed-out
// escrow — the on-chain effect the correlator's timeout floor (VerifyTimedOut) keys off.
const eventIFTRefunded = "IFTRefunded"

// iftRefundedLog is the decoded MockIFT.IFTRefunded event body (seq, sender, amount), all non-indexed. The
// timeout floor asserts the refunded amount equals what the action escrowed.
type iftRefundedLog struct {
	Seq    *big.Int
	Sender common.Address
	Amount *big.Int
}

// decodeIFTRefunded unpacks an IFTRefunded log's data into the typed struct.
func decodeIFTRefunded(data []byte) (iftRefundedLog, error) {
	var ev iftRefundedLog
	if err := mockIFTABI.UnpackIntoInterface(&ev, eventIFTRefunded, data); err != nil {
		return iftRefundedLog{}, fmt.Errorf("decode IFTRefunded: %w", err)
	}
	return ev, nil
}

// eventGMPReceived is the MockGMP event the relayer emits on the destination when it delivers a message
// (deliver() -> target.call) — the on-chain effect the correlator keys its GMP terminal assertion off.
const eventGMPReceived = "GMPReceived"

// mockGMPABI is the parsed MockGMP fixture ABI (decoded from the embedded artifact), reached through the
// shared fixtures package — not the stub's walled internals — so the harness decodes GMPReceived
// independently of the black box. Parsed once at init: a parse failure is a packaging bug, not runtime.
var mockGMPABI = fixtures.MockGMP.MustABI()

// gmpReceivedLog is the decoded MockGMP.GMPReceived event body. All three fields are non-indexed (they
// live in the log data), matching the fixture's emit. Success reflects the inner target.call outcome:
// the GMP terminal floor requires it true (the target state actually changed).
type gmpReceivedLog struct {
	Seq     *big.Int
	Target  common.Address
	Success bool
}

// decodeGMPReceived unpacks a GMPReceived log's data into the typed struct.
func decodeGMPReceived(data []byte) (gmpReceivedLog, error) {
	var ev gmpReceivedLog
	if err := mockGMPABI.UnpackIntoInterface(&ev, eventGMPReceived, data); err != nil {
		return gmpReceivedLog{}, fmt.Errorf("decode GMPReceived: %w", err)
	}
	return ev, nil
}

// counterABI is the parsed Counter fixture ABI (from the shared embedded artifact). Parsed once: the
// artifact is build-time-fixed, so a parse failure is a packaging bug, not a runtime condition.
var counterABI = fixtures.Counter.MustABI()

// counterIncrementCalldata is the ABI-encoded Counter.increment() calldata (its 4-byte selector),
// computed once. The GMP happy path submits this as the default payload (the destination target call),
// so a test never hardcodes the selector; the relayer replays it via MockGMP.deliver(target.call).
var counterIncrementCalldata = mustIncrementCalldata()

// mustIncrementCalldata packs Counter.increment(). The Counter ABI is build-time-fixed and increment()
// takes no args, so a pack failure is a packaging bug (a renamed/removed method), not a runtime one.
func mustIncrementCalldata() []byte {
	data, err := counterABI.Pack("increment")
	if err != nil {
		panic(fmt.Sprintf("onchain: pack Counter.increment(): %v", err))
	}
	return data
}
