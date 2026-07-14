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

	"github.com/cosmos/ibc/e2e/internal/testapp/contracts/bindings"

	ethereum "github.com/ethereum/go-ethereum"
)

var (
	mockIFTABI = mustABI(bindings.MockIFTMetaData)
	mockGMPABI = mustABI(bindings.MockGMPMetaData)
)

func mustABI(metadata *bind.MetaData) abi.ABI {
	parsed, err := metadata.GetAbi()
	if err != nil {
		panic(fmt.Sprintf("parse generated contract ABI: %v", err))
	}
	return *parsed
}

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
}

func newBoundContract(addr common.Address, client *ethclient.Client) boundContract {
	return boundContract{Address: addr, client: client}
}

type seqDecoder func(types.Log) (*big.Int, error)

func (b *boundContract) filterByEvent(
	ctx context.Context,
	event string,
	topic common.Hash,
	fromBlock, toBlock uint64,
) ([]types.Log, error) {
	logs, err := b.client.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(fromBlock),
		ToBlock:   new(big.Int).SetUint64(toBlock),
		Addresses: []common.Address{b.Address},
		Topics:    [][]common.Hash{{topic}},
	})
	if err != nil {
		return nil, fmt.Errorf("filter %s: %w", event, err)
	}
	return logs, nil
}

func (b *boundContract) findBySeq(
	ctx context.Context,
	event string,
	topic common.Hash,
	seq *big.Int,
	seqOf seqDecoder,
) (common.Hash, bool, error) {
	logs, err := b.client.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: big.NewInt(0),
		Addresses: []common.Address{b.Address},
		Topics:    [][]common.Hash{{topic}},
	})
	if err != nil {
		return common.Hash{}, false, fmt.Errorf("filter %s: %w", event, err)
	}
	for _, lg := range logs {
		s, err := seqOf(lg)
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

type receivedDecoder func(types.Log) (key ReceivedKey, success bool, err error)

// recvCursor in relay holds fromBlock across ticks so idempotency scans read only new blocks.
func (b *boundContract) scanReceived(
	ctx context.Context,
	event string,
	topic common.Hash,
	fromBlock uint64,
	decode receivedDecoder,
) (map[ReceivedKey]ReceivedResult, uint64, error) {
	logs, next, err := b.scanEvent(ctx, event, topic, fromBlock)
	if err != nil {
		return nil, fromBlock, err
	}
	out := make(map[ReceivedKey]ReceivedResult, len(logs))
	for _, lg := range logs {
		key, success, err := decode(lg)
		if err != nil {
			return nil, fromBlock, err
		}
		out[key] = ReceivedResult{TxHash: lg.TxHash, Success: success}
	}
	return out, next, nil
}

func (b *boundContract) scanEvent(
	ctx context.Context,
	event string,
	topic common.Hash,
	fromBlock uint64,
) ([]types.Log, uint64, error) {
	head, err := b.client.BlockNumber(ctx)
	if err != nil {
		return nil, fromBlock, fmt.Errorf("head for %s scan: %w", event, err)
	}
	if head < fromBlock {
		return nil, fromBlock, nil
	}
	logs, err := b.filterByEvent(ctx, event, topic, fromBlock, head)
	if err != nil {
		return nil, fromBlock, err
	}
	return logs, head + 1, nil
}

func (b *boundContract) firstReceiptLog(rcpt *types.Receipt, topic common.Hash) (*types.Log, bool) {
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

type MockIFT struct {
	boundContract
	contract *bindings.MockIFT
}

func NewMockIFT(addr common.Address, client *ethclient.Client) *MockIFT {
	contract, err := bindings.NewMockIFT(addr, client)
	if err != nil {
		panic(fmt.Sprintf("bind MockIFT %s: %v", addr, err))
	}
	return &MockIFT{boundContract: newBoundContract(addr, client), contract: contract}
}

func (m *MockIFT) ReceiveTransfer(
	opts *bind.TransactOpts,
	routeID string,
	seq *big.Int,
	receiver common.Address,
	amount *big.Int,
) (*types.Transaction, error) {
	return m.contract.ReceiveTransfer(opts, routeID, seq, receiver, amount)
}

func (m *MockIFT) Refund(opts *bind.TransactOpts, seq *big.Int) (*types.Transaction, error) {
	return m.contract.Refund(opts, seq)
}

func (m *MockIFT) ScanReceivedFrom(
	ctx context.Context,
	fromBlock uint64,
) (map[ReceivedKey]ReceivedResult, uint64, error) {
	return m.scanReceived(
		ctx,
		"IFTReceived",
		mockIFTABI.Events["IFTReceived"].ID,
		fromBlock,
		func(log types.Log) (ReceivedKey, bool, error) {
			ev, err := m.contract.ParseIFTReceived(log)
			if err != nil {
				return ReceivedKey{}, false, fmt.Errorf("decode IFTReceived: %w", err)
			}
			return ReceivedKey{RouteID: ev.RouteId, Sequence: ev.Seq.Uint64()}, true, nil
		},
	)
}

func (m *MockIFT) ScanSentFrom(ctx context.Context, fromBlock uint64) ([]IFTSent, uint64, error) {
	logs, next, err := m.scanEvent(ctx, "IFTSent", mockIFTABI.Events["IFTSent"].ID, fromBlock)
	if err != nil {
		return nil, fromBlock, err
	}
	out := make([]IFTSent, 0, len(logs))
	for _, lg := range logs {
		ev, derr := m.decodeSent(lg)
		if derr != nil {
			return nil, fromBlock, derr
		}
		ev.TxHash = lg.TxHash
		out = append(out, ev)
	}
	return out, next, nil
}

func (m *MockIFT) SentFromReceipt(rcpt *types.Receipt) (IFTSent, bool, error) {
	lg, ok := m.firstReceiptLog(rcpt, mockIFTABI.Events["IFTSent"].ID)
	if !ok {
		return IFTSent{}, false, nil
	}
	ev, err := m.decodeSent(*lg)
	if err != nil {
		return IFTSent{}, false, err
	}
	ev.TxHash = lg.TxHash
	return ev, true, nil
}

func (m *MockIFT) decodeSent(log types.Log) (IFTSent, error) {
	ev, err := m.contract.ParseIFTSent(log)
	if err != nil {
		return IFTSent{}, fmt.Errorf("decode IFTSent: %w", err)
	}
	return IFTSent{
		Seq:              ev.Seq,
		RouteID:          ev.RouteId,
		Receiver:         ev.Receiver,
		Amount:           ev.Amount,
		TimeoutTimestamp: ev.TimeoutTimestamp,
	}, nil
}

// Adopt an on-chain refund before re-issuing — a second refund reverts and wedges the packet pending.
func (m *MockIFT) FindRefunded(ctx context.Context, seq *big.Int) (common.Hash, bool, error) {
	return m.findBySeq(
		ctx,
		"IFTRefunded",
		mockIFTABI.Events["IFTRefunded"].ID,
		seq,
		func(log types.Log) (*big.Int, error) {
			ev, err := m.contract.ParseIFTRefunded(log)
			if err != nil {
				return nil, fmt.Errorf("decode IFTRefunded: %w", err)
			}
			return ev.Seq, nil
		},
	)
}

type GMPSent struct {
	Seq     *big.Int
	RouteID string
	Target  string
	Payload []byte
	TxHash  common.Hash
}

// Success=false is the error-ack outcome (inner target reverted), not a delivery failure.
type GMPReceived struct {
	RouteID string
	Seq     *big.Int
	Target  common.Address
	Success bool
}

type MockGMP struct {
	boundContract
	contract *bindings.MockGMP
}

func NewMockGMP(addr common.Address, client *ethclient.Client) *MockGMP {
	contract, err := bindings.NewMockGMP(addr, client)
	if err != nil {
		panic(fmt.Sprintf("bind MockGMP %s: %v", addr, err))
	}
	return &MockGMP{boundContract: newBoundContract(addr, client), contract: contract}
}

func (m *MockGMP) Deliver(
	opts *bind.TransactOpts,
	routeID string,
	seq *big.Int,
	target common.Address,
	payload []byte,
) (*types.Transaction, error) {
	return m.contract.Deliver(opts, routeID, seq, target, payload)
}

func (m *MockGMP) ScanReceivedFrom(
	ctx context.Context,
	fromBlock uint64,
) (map[ReceivedKey]ReceivedResult, uint64, error) {
	return m.scanReceived(
		ctx,
		"GMPReceived",
		mockGMPABI.Events["GMPReceived"].ID,
		fromBlock,
		func(log types.Log) (ReceivedKey, bool, error) {
			ev, err := m.decodeReceived(log)
			if err != nil {
				return ReceivedKey{}, false, err
			}
			return ReceivedKey{RouteID: ev.RouteID, Sequence: ev.Seq.Uint64()}, ev.Success, nil
		},
	)
}

// deliver() mines successfully either way; GMPReceived.success is the only inner-outcome signal.
func (m *MockGMP) DeliveredSuccessFromReceipt(rcpt *types.Receipt) (success bool, found bool, err error) {
	lg, ok := m.firstReceiptLog(rcpt, mockGMPABI.Events["GMPReceived"].ID)
	if !ok {
		return false, false, nil
	}
	ev, derr := m.decodeReceived(*lg)
	if derr != nil {
		return false, false, derr
	}
	return ev.Success, true, nil
}

func (m *MockGMP) ScanSentFrom(ctx context.Context, fromBlock uint64) ([]GMPSent, uint64, error) {
	logs, next, err := m.scanEvent(ctx, "GMPSent", mockGMPABI.Events["GMPSent"].ID, fromBlock)
	if err != nil {
		return nil, fromBlock, err
	}
	out := make([]GMPSent, 0, len(logs))
	for _, lg := range logs {
		ev, derr := m.decodeSent(lg)
		if derr != nil {
			return nil, fromBlock, derr
		}
		ev.TxHash = lg.TxHash
		out = append(out, ev)
	}
	return out, next, nil
}

func (m *MockGMP) SentFromReceipt(rcpt *types.Receipt) (GMPSent, bool, error) {
	lg, ok := m.firstReceiptLog(rcpt, mockGMPABI.Events["GMPSent"].ID)
	if !ok {
		return GMPSent{}, false, nil
	}
	ev, err := m.decodeSent(*lg)
	if err != nil {
		return GMPSent{}, false, err
	}
	ev.TxHash = lg.TxHash
	return ev, true, nil
}

func (m *MockGMP) decodeSent(log types.Log) (GMPSent, error) {
	ev, err := m.contract.ParseGMPSent(log)
	if err != nil {
		return GMPSent{}, fmt.Errorf("decode GMPSent: %w", err)
	}
	return GMPSent{Seq: ev.Seq, RouteID: ev.RouteId, Target: ev.Target, Payload: ev.Payload}, nil
}

func (m *MockGMP) decodeReceived(log types.Log) (GMPReceived, error) {
	ev, err := m.contract.ParseGMPReceived(log)
	if err != nil {
		return GMPReceived{}, fmt.Errorf("decode GMPReceived: %w", err)
	}
	return GMPReceived{RouteID: ev.RouteId, Seq: ev.Seq, Target: ev.Target, Success: ev.Success}, nil
}
