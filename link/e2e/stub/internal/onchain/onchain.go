// Package onchain provides EVM fixture bindings and event decoding for stub deploy and relay operations.
package onchain

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/cosmos/ibc/link/harness/fixtures"
	"github.com/cosmos/ibc/link/harness/testkeys"

	ethereum "github.com/ethereum/go-ethereum"
)

var (
	faucetKey  *ecdsa.PrivateKey
	faucetAddr common.Address
)

// Fixture ABIs are immutable embedded artifacts, parsed once.
var (
	mockIFTABI = fixtures.MockIFT.MustABI()
	mockGMPABI = fixtures.MockGMP.MustABI()
)

func init() {
	// The faucet is the well-known dev source account (Anvil #0); it escrows the source-side effect and
	// funds fresh accounts. The relayer's signing key is NOT a shared constant: it is the per-chain
	// EVMSignerKey the relay daemon parses from the wire config (see ParseKey / RelayerTransactor).
	faucetKey, faucetAddr = mustKey(testkeys.FaucetPrivateKeyHex, "faucet")
}

// mustKey parses a well-known test dev key. These are compile-time constants, so a parse failure is a
// programmer error, not a runtime one.
func mustKey(hexKey, name string) (*ecdsa.PrivateKey, common.Address) {
	k, err := crypto.HexToECDSA(hexKey)
	if err != nil {
		panic(fmt.Sprintf("onchain: invalid %s key: %v", name, err))
	}
	return k, crypto.PubkeyToAddress(k.PublicKey)
}

// ParseKey parses a hex-encoded secp256k1 private key (with or without a 0x prefix). The relay daemon
// parses each EVM chain's configured EVMSignerKey once at dial time so the relayer signs destination
// effects from the config-declared identity rather than a shared constant.
func ParseKey(hexKey string) (*ecdsa.PrivateKey, error) {
	k, err := crypto.HexToECDSA(strings.TrimPrefix(hexKey, "0x"))
	if err != nil {
		return nil, fmt.Errorf("parse evm signer key: %w", err)
	}
	return k, nil
}

// FaucetAddress returns the faucet account address (Anvil dev account #0).
func FaucetAddress() common.Address { return faucetAddr }

// dial opens an ethclient against url, bounded by ctx for the connection setup.
func dial(ctx context.Context, url string) (*ethclient.Client, error) {
	c, err := ethclient.DialContext(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("dial rpc: %w", err)
	}
	return c, nil
}

// Conn is one dialed chain plus its probed chain id. Connect builds it once so deploy/app/relay share a
// single dial + chain-id-as-liveness-check path instead of each repeating it.
type Conn struct {
	Client  *ethclient.Client
	ChainID *big.Int
}

// Connect dials url and probes the node's chain id (a liveness check and the value signing needs),
// bounded by ctx. The caller owns Close on the returned Conn's Client. A probe failure closes the
// client so a failed Connect never leaks a connection.
func Connect(ctx context.Context, url string) (*Conn, error) {
	client, err := dial(ctx, url)
	if err != nil {
		return nil, err
	}
	// Trust the node's reported chain id for signing rather than the config's, so a stale config can't
	// silently produce wrong-chain signatures; this call also doubles as a liveness check.
	id, err := client.ChainID(ctx)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("query chain id: %w", err)
	}
	return &Conn{Client: client, ChainID: id}, nil
}

// FaucetTransactor builds a faucet-signed transactor for this connection's chain id. Nonce and gas
// are left zero so go-ethereum's bind fills them from the backend at send time (pending nonce + gas
// estimate); ctx bounds every RPC the resulting bound call makes.
func (c *Conn) FaucetTransactor(ctx context.Context) (*bind.TransactOpts, error) {
	return keyedTransactor(ctx, faucetKey, c.ChainID, "faucet")
}

// RelayerTransactor builds a transactor signed by the chain's configured relayer key for chainID. The
// relay daemon uses it for the destination-side effect (and source-side refund), keeping its nonce space
// separate from the faucet's. The key is the per-chain EVMSignerKey from the wire config, not a constant.
func RelayerTransactor(ctx context.Context, key *ecdsa.PrivateKey, chainID *big.Int) (*bind.TransactOpts, error) {
	return keyedTransactor(ctx, key, chainID, "relayer")
}

func keyedTransactor(
	ctx context.Context,
	key *ecdsa.PrivateKey,
	chainID *big.Int,
	name string,
) (*bind.TransactOpts, error) {
	opts, err := bind.NewKeyedTransactorWithChainID(key, chainID)
	if err != nil {
		return nil, fmt.Errorf("build %s transactor: %w", name, err)
	}
	opts.Context = ctx
	return opts, nil
}

// WaitMined blocks until tx is mined (or ctx ends), returning its receipt.
func WaitMined(ctx context.Context, client *ethclient.Client, tx *types.Transaction) (*types.Receipt, error) {
	rcpt, err := bind.WaitMined(ctx, client, tx)
	if err != nil {
		return nil, fmt.Errorf("await tx %s: %w", tx.Hash().Hex(), err)
	}
	return rcpt, nil
}

// boundFixture is the bind/decode scaffolding every typed fixture wrapper shares: the deployed address,
// the dialed client, a bound contract for calls, and the parsed ABI. MockIFT and MockGMP embed it and
// add only their typed calls + event decoders, so the filter/find/receipt-scan plumbing and
// sequence-matching caveats live once rather than once per fixture.
type boundFixture struct {
	Address common.Address
	client  *ethclient.Client
	bound   *bind.BoundContract
	abi     abi.ABI
}

func newBoundFixture(addr common.Address, client *ethclient.Client, parsed abi.ABI) boundFixture {
	return boundFixture{
		Address: addr,
		client:  client,
		bound:   bind.NewBoundContract(addr, parsed, client, client, client),
		abi:     parsed,
	}
}

// seqDecoder pulls the *big.Int sequence out of an event log's data (each fixture supplies one bound to
// the relevant event). It is what findBySeq / sentSeqFromReceipt match on without knowing the rest of
// the event's shape.
type seqDecoder func(data []byte) (*big.Int, error)

// filterByEvent returns this fixture's `event` logs in [fromBlock, toBlock]. The destination idempotency
// scan polls it from a per-fixture cursor to read only newly-mined Received events.
func (b *boundFixture) filterByEvent(
	ctx context.Context,
	event string,
	fromBlock, toBlock uint64,
) ([]types.Log, error) {
	logs, err := b.client.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(fromBlock),
		ToBlock:   new(big.Int).SetUint64(toBlock),
		Addresses: []common.Address{b.Address},
		Topics:    [][]common.Hash{{b.abi.Events[event].ID}},
	})
	if err != nil {
		return nil, fmt.Errorf("filter %s: %w", event, err)
	}
	return logs, nil
}

// findBySeq scans this fixture's `event` logs from genesis for one whose decoded sequence equals seq,
// returning the emitting tx hash. It backs the source-refund idempotency check (a rare, terminal path),
// where a genesis scan is cheap; the hot destination-received check uses the cursor-bounded scanReceived.
//
// Sequence-only matching is sound here because the refund event is read off the SOURCE fixture, and a
// fixture assigns sequences from its own monotonic counter — unique per fixture no matter how many
// routes share it. (Destination-side matching cannot rely on this: routes sharing a destination fixture
// deliver colliding source sequences, which is why the delivery path keys on the route-scoped form.)
func (b *boundFixture) findBySeq(
	ctx context.Context,
	event string,
	seq *big.Int,
	seqOf seqDecoder,
) (common.Hash, bool, error) {
	logs, err := b.client.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: big.NewInt(0),
		Addresses: []common.Address{b.Address},
		Topics:    [][]common.Hash{{b.abi.Events[event].ID}},
	})
	if err != nil {
		return common.Hash{}, false, fmt.Errorf("filter %s: %w", event, err)
	}
	for _, lg := range logs {
		s, err := seqOf(lg.Data)
		if err != nil {
			return common.Hash{}, false, err
		}
		if s.Cmp(seq) == 0 {
			return lg.TxHash, true, nil
		}
	}
	return common.Hash{}, false, nil
}

// ReceivedResult is a destination Received event matched by sequence: the emitting tx and the delivery
// outcome. Success is the GMPReceived inner-call bit; for an IFT mint it is always true (the mint is
// unconditional), so the two families share one idempotency shape.
type ReceivedResult struct {
	TxHash  common.Hash
	Success bool
}

// receivedDecoder pulls the (sequence, success) an idempotency scan matches on out of a Received log's
// data. IFT reports success=true unconditionally; GMP reports the inner-call bit.
type receivedDecoder func(data []byte) (seq uint64, success bool, err error)

// scanReceived returns this fixture's `event` logs in [fromBlock, head] decoded by sequence, plus the
// next block to scan (head+1). The relayer holds the cursor across ticks so the idempotency check reads
// only new blocks instead of rescanning the whole log history every 250ms tick.
func (b *boundFixture) scanReceived(
	ctx context.Context,
	event string,
	fromBlock uint64,
	decode receivedDecoder,
) (map[uint64]ReceivedResult, uint64, error) {
	head, err := b.client.BlockNumber(ctx)
	if err != nil {
		return nil, fromBlock, fmt.Errorf("head for %s scan: %w", event, err)
	}
	if head < fromBlock {
		return nil, fromBlock, nil
	}
	logs, err := b.filterByEvent(ctx, event, fromBlock, head)
	if err != nil {
		return nil, fromBlock, err
	}
	out := make(map[uint64]ReceivedResult, len(logs))
	for _, lg := range logs {
		seq, success, err := decode(lg.Data)
		if err != nil {
			return nil, fromBlock, err
		}
		out[seq] = ReceivedResult{TxHash: lg.TxHash, Success: success}
	}
	return out, head + 1, nil
}

// scanEvent returns event logs in [fromBlock, head] and the next block to scan.
func (b *boundFixture) scanEvent(ctx context.Context, event string, fromBlock uint64) ([]types.Log, uint64, error) {
	head, err := b.client.BlockNumber(ctx)
	if err != nil {
		return nil, fromBlock, fmt.Errorf("head for %s scan: %w", event, err)
	}
	if head < fromBlock {
		return nil, fromBlock, nil
	}
	logs, err := b.filterByEvent(ctx, event, fromBlock, head)
	if err != nil {
		return nil, fromBlock, err
	}
	return logs, head + 1, nil
}

// firstReceiptLog returns the first log in rcpt emitted by this fixture for `event`. found is false if
// none is present. It is the shared receipt-log predicate the sequence and success extractors match on.
func (b *boundFixture) firstReceiptLog(rcpt *types.Receipt, event string) (*types.Log, bool) {
	topic := b.abi.Events[event].ID
	for _, lg := range rcpt.Logs {
		if lg.Address == b.Address && len(lg.Topics) > 0 && lg.Topics[0] == topic {
			return lg, true
		}
	}
	return nil, false
}

// IFTSent is a decoded source escrow event. RouteID attributes it to a configured route.
type IFTSent struct {
	Seq              *big.Int
	RouteID          string
	Receiver         string
	Amount           *big.Int
	TimeoutTimestamp *big.Int
	// TxHash comes from the enclosing log, not the event data.
	TxHash common.Hash
}

// iftSentData mirrors the IFTSent event args exactly (no TxHash) for ABI unpack. The abi tag pins the
// event-arg->field mapping: go-ethereum maps "routeId" to a field "RouteId" by default, so a "RouteID"
// field would silently stay empty (and drop every discovered packet); the tag makes the mapping explicit.
type iftSentData struct {
	Seq              *big.Int
	RouteID          string `abi:"routeId"`
	Receiver         string
	Amount           *big.Int
	TimeoutTimestamp *big.Int
}

// iftReceived differs from IFTSent because its receiver is an EVM address.
type iftReceived struct {
	Seq      *big.Int
	Receiver common.Address
	Amount   *big.Int
}

// MockIFT is a typed handle to a deployed MockIFT fixture: the shared scaffolding plus the token's typed
// calls and event decoding.
type MockIFT struct {
	boundFixture
}

// NewMockIFT binds the MockIFT fixture at addr on client.
func NewMockIFT(addr common.Address, client *ethclient.Client) *MockIFT {
	return &MockIFT{newBoundFixture(addr, client, mockIFTABI)}
}

// ReceiveTransfer mints amount to receiver on the destination — the relayer's real on-chain effect.
func (m *MockIFT) ReceiveTransfer(
	opts *bind.TransactOpts,
	seq *big.Int,
	receiver common.Address,
	amount *big.Int,
) (*types.Transaction, error) {
	return m.bound.Transact(opts, "receiveTransfer", seq, receiver, amount)
}

// Refund releases the source escrow for seq back to its original sender — the relayer's on-chain effect
// when a transfer times out undelivered. The fixture rejects a refund that is not genuinely timed out
// (unknown/no-timeout/not-yet-expired/already-refunded), so calling it is safe even on a re-scan.
func (m *MockIFT) Refund(opts *bind.TransactOpts, seq *big.Int) (*types.Transaction, error) {
	return m.bound.Transact(opts, "refund", seq)
}

// ScanReceivedFrom returns the IFTReceived events in [fromBlock, head] decoded by sequence, plus the
// next cursor. It backs the relayer's mint idempotency check; a mint is unconditional, so success is
// always true.
func (m *MockIFT) ScanReceivedFrom(
	ctx context.Context,
	fromBlock uint64,
) (map[uint64]ReceivedResult, uint64, error) {
	return m.scanReceived(ctx, "IFTReceived", fromBlock, func(data []byte) (uint64, bool, error) {
		var ev iftReceived
		if err := m.abi.UnpackIntoInterface(&ev, "IFTReceived", data); err != nil {
			return 0, false, fmt.Errorf("decode IFTReceived: %w", err)
		}
		return ev.Seq.Uint64(), true, nil
	})
}

// ScanSentFrom returns decoded IFTSent events and the next scan cursor.
func (m *MockIFT) ScanSentFrom(ctx context.Context, fromBlock uint64) ([]IFTSent, uint64, error) {
	logs, next, err := m.scanEvent(ctx, "IFTSent", fromBlock)
	if err != nil {
		return nil, fromBlock, err
	}
	out := make([]IFTSent, 0, len(logs))
	for _, lg := range logs {
		ev, derr := decodeIFTSent(lg.Data)
		if derr != nil {
			return nil, fromBlock, derr
		}
		ev.TxHash = lg.TxHash
		out = append(out, ev)
	}
	return out, next, nil
}

// SentFromReceipt decodes this fixture's IFTSent event from rcpt.
func (m *MockIFT) SentFromReceipt(rcpt *types.Receipt) (IFTSent, bool, error) {
	lg, ok := m.firstReceiptLog(rcpt, "IFTSent")
	if !ok {
		return IFTSent{}, false, nil
	}
	ev, err := decodeIFTSent(lg.Data)
	if err != nil {
		return IFTSent{}, false, err
	}
	ev.TxHash = lg.TxHash
	return ev, true, nil
}

func decodeIFTSent(data []byte) (IFTSent, error) {
	var ev iftSentData
	if err := mockIFTABI.UnpackIntoInterface(&ev, "IFTSent", data); err != nil {
		return IFTSent{}, fmt.Errorf("decode IFTSent: %w", err)
	}
	return IFTSent{
		Seq:              ev.Seq,
		RouteID:          ev.RouteID,
		Receiver:         ev.Receiver,
		Amount:           ev.Amount,
		TimeoutTimestamp: ev.TimeoutTimestamp,
	}, nil
}

// FindRefunded reports whether this token already emitted an IFTRefunded for seq, returning the emitting
// refund tx hash. The relayer consults it before re-issuing refund so an already-settled escrow (refunded
// before a crash marked the packet timed_out) is adopted rather than re-refunded (which reverts).
func (m *MockIFT) FindRefunded(ctx context.Context, seq *big.Int) (common.Hash, bool, error) {
	return m.findBySeq(ctx, "IFTRefunded", seq, func(data []byte) (*big.Int, error) {
		var ev struct {
			Seq    *big.Int
			Sender common.Address
			Amount *big.Int
		}
		if err := m.abi.UnpackIntoInterface(&ev, "IFTRefunded", data); err != nil {
			return nil, fmt.Errorf("decode IFTRefunded: %w", err)
		}
		return ev.Seq, nil
	})
}

// GMPSent is a decoded source message event. RouteID attributes it to a configured route.
type GMPSent struct {
	Seq     *big.Int
	RouteID string
	Target  string
	Payload []byte
	// TxHash comes from the enclosing log, not the event data.
	TxHash common.Hash
}

// gmpSentData mirrors the GMPSent event args exactly (no TxHash) for ABI unpack; the abi tag pins the
// "routeId" event arg to the RouteID field (see iftSentData for why the default mapping would drop it).
type gmpSentData struct {
	Seq     *big.Int
	RouteID string `abi:"routeId"`
	Target  string
	Payload []byte
}

// GMPReceived is the decoded MockGMP.GMPReceived event, the destination-side delivery record. Success
// reflects the inner target.call outcome, not whether deliver() itself succeeded — deliver() emits this
// regardless, so it is the ground-truth "this sequence was delivered" marker the relayer dedups on.
type GMPReceived struct {
	Seq     *big.Int
	Target  common.Address
	Success bool
}

// MockGMP is a typed handle to a deployed MockGMP fixture, mirroring MockIFT: the shared scaffolding
// plus the send/deliver calls and the two events the relayer keys off (GMPSent on the source,
// GMPReceived on the destination).
type MockGMP struct {
	boundFixture
}

// NewMockGMP binds the MockGMP fixture at addr on client.
func NewMockGMP(addr common.Address, client *ethclient.Client) *MockGMP {
	return &MockGMP{newBoundFixture(addr, client, mockGMPABI)}
}

// Deliver performs the destination effect: it calls target with payload (e.g. Counter.increment()) and
// emits GMPReceived. seq is the source sequence being delivered — the GMP analog of ReceiveTransfer.
func (m *MockGMP) Deliver(
	opts *bind.TransactOpts,
	seq *big.Int,
	target common.Address,
	payload []byte,
) (*types.Transaction, error) {
	return m.bound.Transact(opts, "deliver", seq, target, payload)
}

// DeliverIFT executes canonical iftMint calldata with source-client context available to the target.
func (m *MockGMP) DeliverIFT(
	opts *bind.TransactOpts,
	seq *big.Int,
	clientID string,
	target common.Address,
	payload []byte,
) (*types.Transaction, error) {
	return m.bound.Transact(opts, "deliverIFT", seq, clientID, target, payload)
}

// ScanIFTReceivedFrom returns GMP deliveries to ift in [fromBlock, head], correlated with the canonical
// mint event from the same transaction. The outer GMP sequence is the destination packet identity.
func (m *MockGMP) ScanIFTReceivedFrom(
	ctx context.Context,
	ift common.Address,
	clientID string,
	fromBlock uint64,
) (map[uint64]ReceivedResult, uint64, error) {
	gmpLogs, next, err := m.scanEvent(ctx, "GMPReceived", fromBlock)
	if err != nil {
		return nil, fromBlock, err
	}
	if next == fromBlock {
		return nil, next, nil
	}
	mintLogs, err := m.client.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(fromBlock),
		ToBlock:   new(big.Int).SetUint64(next - 1),
		Addresses: []common.Address{ift},
		Topics:    [][]common.Hash{{mockIFTABI.Events["IFTMintReceived"].ID}},
	})
	if err != nil {
		return nil, fromBlock, fmt.Errorf("filter IFTMintReceived: %w", err)
	}
	mintTxs := make(map[common.Hash]struct{}, len(mintLogs))
	for _, lg := range mintLogs {
		var mint struct {
			ClientID string `abi:"clientId"`
			Amount   *big.Int
		}
		if err := mockIFTABI.UnpackIntoInterface(&mint, "IFTMintReceived", lg.Data); err != nil {
			return nil, fromBlock, fmt.Errorf("decode IFTMintReceived: %w", err)
		}
		if mint.ClientID == clientID {
			mintTxs[lg.TxHash] = struct{}{}
		}
	}

	out := make(map[uint64]ReceivedResult, len(gmpLogs))
	for _, lg := range gmpLogs {
		ev, derr := m.decodeReceived(lg.Data)
		if derr != nil {
			return nil, fromBlock, derr
		}
		if ev.Target != ift {
			continue
		}
		if ev.Success {
			if _, ok := mintTxs[lg.TxHash]; !ok {
				return nil, fromBlock, fmt.Errorf(
					"successful GMP delivery %s emitted no matching IFTMintReceived",
					lg.TxHash.Hex(),
				)
			}
		}
		out[ev.Seq.Uint64()] = ReceivedResult{TxHash: lg.TxHash, Success: ev.Success}
	}
	return out, next, nil
}

// ScanReceivedFrom returns the GMPReceived events in [fromBlock, head] decoded by (sequence, success),
// plus the next cursor. It backs the relayer's delivery idempotency check — a second deliver would
// replay target.call (e.g. increment the Counter twice) — and carries the inner-call success bit so a
// restart can tell a real delivery from an error-ack.
func (m *MockGMP) ScanReceivedFrom(
	ctx context.Context,
	fromBlock uint64,
) (map[uint64]ReceivedResult, uint64, error) {
	return m.scanReceived(ctx, "GMPReceived", fromBlock, func(data []byte) (uint64, bool, error) {
		ev, err := m.decodeReceived(data)
		if err != nil {
			return 0, false, err
		}
		return ev.Seq.Uint64(), ev.Success, nil
	})
}

// DeliveredSuccessFromReceipt reads the inner-call outcome the deliver() tx recorded: it finds this
// messenger's GMPReceived log in the receipt and returns its success bit. found is false if no
// GMPReceived from this fixture is present (the deliver did not run). The relayer uses it to tell a
// real delivery (success=true -> complete) from a destination call that reverted (success=false ->
// error_ack): deliver() emits GMPReceived regardless, so the tx mines successfully either way and the
// success bit is the only signal of the inner outcome.
func (m *MockGMP) DeliveredSuccessFromReceipt(rcpt *types.Receipt) (success bool, found bool, err error) {
	lg, ok := m.firstReceiptLog(rcpt, "GMPReceived")
	if !ok {
		return false, false, nil
	}
	ev, derr := m.decodeReceived(lg.Data)
	if derr != nil {
		return false, false, derr
	}
	return ev.Success, true, nil
}

// ScanSentFrom returns decoded GMPSent events and the next scan cursor.
func (m *MockGMP) ScanSentFrom(ctx context.Context, fromBlock uint64) ([]GMPSent, uint64, error) {
	logs, next, err := m.scanEvent(ctx, "GMPSent", fromBlock)
	if err != nil {
		return nil, fromBlock, err
	}
	out := make([]GMPSent, 0, len(logs))
	for _, lg := range logs {
		ev, derr := decodeGMPSent(lg.Data)
		if derr != nil {
			return nil, fromBlock, derr
		}
		ev.TxHash = lg.TxHash
		out = append(out, ev)
	}
	return out, next, nil
}

// SentFromReceipt decodes this fixture's GMPSent event from rcpt.
func (m *MockGMP) SentFromReceipt(rcpt *types.Receipt) (GMPSent, bool, error) {
	lg, ok := m.firstReceiptLog(rcpt, "GMPSent")
	if !ok {
		return GMPSent{}, false, nil
	}
	ev, err := decodeGMPSent(lg.Data)
	if err != nil {
		return GMPSent{}, false, err
	}
	ev.TxHash = lg.TxHash
	return ev, true, nil
}

func decodeGMPSent(data []byte) (GMPSent, error) {
	var ev gmpSentData
	if err := mockGMPABI.UnpackIntoInterface(&ev, "GMPSent", data); err != nil {
		return GMPSent{}, fmt.Errorf("decode GMPSent: %w", err)
	}
	return GMPSent{Seq: ev.Seq, RouteID: ev.RouteID, Target: ev.Target, Payload: ev.Payload}, nil
}

func (m *MockGMP) decodeReceived(data []byte) (GMPReceived, error) {
	var ev GMPReceived
	if err := m.abi.UnpackIntoInterface(&ev, "GMPReceived", data); err != nil {
		return GMPReceived{}, fmt.Errorf("decode GMPReceived: %w", err)
	}
	return ev, nil
}
