// Package onchain provides the EVM bindings used by the synthetic relayer.
package onchain

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/cosmos/ibc/e2e/internal/testapp/contracts"

	ethereum "github.com/ethereum/go-ethereum"
)

var (
	mockIFTABI = contracts.MockIFT.MustABI()
	mockGMPABI = contracts.MockGMP.MustABI()
)

func dial(ctx context.Context, url string) (*ethclient.Client, error) {
	c, err := ethclient.DialContext(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("dial rpc: %w", err)
	}
	return c, nil
}

type Conn struct {
	Client  *ethclient.Client
	ChainID *big.Int
}

func Connect(ctx context.Context, url string) (*Conn, error) {
	client, err := dial(ctx, url)
	if err != nil {
		return nil, err
	}
	// Sign with the node's reported chain id, not config (also liveness probe).
	id, err := client.ChainID(ctx)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("query chain id: %w", err)
	}
	return &Conn{Client: client, ChainID: id}, nil
}

func Transactor(
	ctx context.Context,
	key *ecdsa.PrivateKey,
	chainID *big.Int,
) (*bind.TransactOpts, error) {
	opts, err := bind.NewKeyedTransactorWithChainID(key, chainID)
	if err != nil {
		return nil, fmt.Errorf("build EVM transactor: %w", err)
	}
	opts.Context = ctx
	return opts, nil
}

func WaitMined(ctx context.Context, client *ethclient.Client, tx *types.Transaction) (*types.Receipt, error) {
	rcpt, err := bind.WaitMined(ctx, client, tx)
	if err != nil {
		return nil, fmt.Errorf("await tx %s: %w", tx.Hash().Hex(), err)
	}
	return rcpt, nil
}

type boundContract struct {
	Address common.Address
	client  *ethclient.Client
	bound   *bind.BoundContract
	abi     abi.ABI
}

func newBoundContract(addr common.Address, client *ethclient.Client, parsed abi.ABI) boundContract {
	return boundContract{
		Address: addr,
		client:  client,
		bound:   bind.NewBoundContract(addr, parsed, client, client, client),
		abi:     parsed,
	}
}

type seqDecoder func(data []byte) (*big.Int, error)

func (b *boundContract) filterByEvent(
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

func (b *boundContract) findBySeq(
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

type ReceivedResult struct {
	TxHash  common.Hash
	Success bool
}

type ReceivedKey struct {
	RouteID  string
	Sequence uint64
}

type receivedDecoder func(data []byte) (key ReceivedKey, success bool, err error)

// recvCursor in relay holds fromBlock across ticks so idempotency scans read only new blocks.
func (b *boundContract) scanReceived(
	ctx context.Context,
	event string,
	fromBlock uint64,
	decode receivedDecoder,
) (map[ReceivedKey]ReceivedResult, uint64, error) {
	logs, next, err := b.scanEvent(ctx, event, fromBlock)
	if err != nil {
		return nil, fromBlock, err
	}
	out := make(map[ReceivedKey]ReceivedResult, len(logs))
	for _, lg := range logs {
		key, success, err := decode(lg.Data)
		if err != nil {
			return nil, fromBlock, err
		}
		out[key] = ReceivedResult{TxHash: lg.TxHash, Success: success}
	}
	return out, next, nil
}

func (b *boundContract) scanEvent(ctx context.Context, event string, fromBlock uint64) ([]types.Log, uint64, error) {
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

func (b *boundContract) firstReceiptLog(rcpt *types.Receipt, event string) (*types.Log, bool) {
	topic := b.abi.Events[event].ID
	for _, lg := range rcpt.Logs {
		if lg.Address == b.Address && len(lg.Topics) > 0 && lg.Topics[0] == topic {
			return lg, true
		}
	}
	return nil, false
}

type IFTSent struct {
	Seq              *big.Int
	RouteID          string
	Receiver         string
	Amount           *big.Int
	TimeoutTimestamp *big.Int
	TxHash           common.Hash
}

// abi:"routeId" required — default mapping leaves RouteID empty and drops every discovered packet.
type iftSentData struct {
	Seq              *big.Int
	RouteID          string `abi:"routeId"`
	Receiver         string
	Amount           *big.Int
	TimeoutTimestamp *big.Int
}

type iftReceived struct {
	RouteID  string `abi:"routeId"`
	Seq      *big.Int
	Receiver common.Address
	Amount   *big.Int
}

type MockIFT struct {
	boundContract
}

func NewMockIFT(addr common.Address, client *ethclient.Client) *MockIFT {
	return &MockIFT{newBoundContract(addr, client, mockIFTABI)}
}

func (m *MockIFT) ReceiveTransfer(
	opts *bind.TransactOpts,
	routeID string,
	seq *big.Int,
	receiver common.Address,
	amount *big.Int,
) (*types.Transaction, error) {
	return m.bound.Transact(opts, "receiveTransfer", routeID, seq, receiver, amount)
}

func (m *MockIFT) Refund(opts *bind.TransactOpts, seq *big.Int) (*types.Transaction, error) {
	return m.bound.Transact(opts, "refund", seq)
}

func (m *MockIFT) ScanReceivedFrom(
	ctx context.Context,
	fromBlock uint64,
) (map[ReceivedKey]ReceivedResult, uint64, error) {
	return m.scanReceived(ctx, "IFTReceived", fromBlock, func(data []byte) (ReceivedKey, bool, error) {
		var ev iftReceived
		if err := m.abi.UnpackIntoInterface(&ev, "IFTReceived", data); err != nil {
			return ReceivedKey{}, false, fmt.Errorf("decode IFTReceived: %w", err)
		}
		return ReceivedKey{RouteID: ev.RouteID, Sequence: ev.Seq.Uint64()}, true, nil
	})
}

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

// Adopt an on-chain refund before re-issuing — a second refund reverts and wedges the packet pending.
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

type GMPSent struct {
	Seq     *big.Int
	RouteID string
	Target  string
	Payload []byte
	TxHash  common.Hash
}

type gmpSentData struct {
	Seq     *big.Int
	RouteID string `abi:"routeId"`
	Target  string
	Payload []byte
}

// Success=false is the error-ack outcome (inner target reverted), not a delivery failure.
type GMPReceived struct {
	RouteID string `abi:"routeId"`
	Seq     *big.Int
	Target  common.Address
	Success bool
}

type MockGMP struct {
	boundContract
}

func NewMockGMP(addr common.Address, client *ethclient.Client) *MockGMP {
	return &MockGMP{newBoundContract(addr, client, mockGMPABI)}
}

func (m *MockGMP) Deliver(
	opts *bind.TransactOpts,
	routeID string,
	seq *big.Int,
	target common.Address,
	payload []byte,
) (*types.Transaction, error) {
	return m.bound.Transact(opts, "deliver", routeID, seq, target, payload)
}

func (m *MockGMP) ScanReceivedFrom(
	ctx context.Context,
	fromBlock uint64,
) (map[ReceivedKey]ReceivedResult, uint64, error) {
	return m.scanReceived(ctx, "GMPReceived", fromBlock, func(data []byte) (ReceivedKey, bool, error) {
		ev, err := m.decodeReceived(data)
		if err != nil {
			return ReceivedKey{}, false, err
		}
		return ReceivedKey{RouteID: ev.RouteID, Sequence: ev.Seq.Uint64()}, ev.Success, nil
	})
}

// deliver() mines successfully either way; GMPReceived.success is the only inner-outcome signal.
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
