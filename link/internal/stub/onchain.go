// Package onchain provides the EVM bindings used by the synthetic relayer.
package stub

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

	ethereum "github.com/ethereum/go-ethereum"
)

var (
	mockIFTABI = mustABI(MockIFTMetaData)
	mockGMPABI = mustABI(MockGMPMetaData)
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

type chainConnection struct {
	Client  *ethclient.Client
	ChainID *big.Int
}

func connectChain(ctx context.Context, url string) (*chainConnection, error) {
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
	return &chainConnection{Client: client, ChainID: id}, nil
}

func newTransactor(
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

func waitMined(ctx context.Context, client *ethclient.Client, tx *types.Transaction) (*types.Receipt, error) {
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

type receivedResult struct {
	TxHash  common.Hash
	Success bool
}

type receivedEventKey struct {
	RouteID  string
	Sequence uint64
}

type receivedDecoder func(types.Log) (key receivedEventKey, success bool, err error)

// recvCursor in relay holds fromBlock across ticks so idempotency scans read only new blocks.
func (b *boundContract) scanReceived(
	ctx context.Context,
	event string,
	topic common.Hash,
	fromBlock uint64,
	decode receivedDecoder,
) (map[receivedEventKey]receivedResult, uint64, error) {
	logs, next, err := b.scanEvent(ctx, event, topic, fromBlock)
	if err != nil {
		return nil, fromBlock, err
	}
	out := make(map[receivedEventKey]receivedResult, len(logs))
	for _, lg := range logs {
		key, success, err := decode(lg)
		if err != nil {
			return nil, fromBlock, err
		}
		out[key] = receivedResult{TxHash: lg.TxHash, Success: success}
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

type iftSent struct {
	Seq              *big.Int
	RouteID          string
	Receiver         string
	Amount           *big.Int
	TimeoutTimestamp *big.Int
	TxHash           common.Hash
}

type testAppIFT struct {
	boundContract
	contract *MockIFT
}

func newTestAppIFT(addr common.Address, client *ethclient.Client) *testAppIFT {
	contract, err := NewMockIFT(addr, client)
	if err != nil {
		panic(fmt.Sprintf("bind MockIFT %s: %v", addr, err))
	}
	return &testAppIFT{boundContract: newBoundContract(addr, client), contract: contract}
}

func (m *testAppIFT) ReceiveTransfer(
	opts *bind.TransactOpts,
	routeID string,
	seq *big.Int,
	receiver common.Address,
	amount *big.Int,
) (*types.Transaction, error) {
	return m.contract.ReceiveTransfer(opts, routeID, seq, receiver, amount)
}

func (m *testAppIFT) Refund(opts *bind.TransactOpts, seq *big.Int) (*types.Transaction, error) {
	return m.contract.Refund(opts, seq)
}

func (m *testAppIFT) ScanReceivedFrom(
	ctx context.Context,
	fromBlock uint64,
) (map[receivedEventKey]receivedResult, uint64, error) {
	return m.scanReceived(
		ctx,
		"IFTReceived",
		mockIFTABI.Events["IFTReceived"].ID,
		fromBlock,
		func(log types.Log) (receivedEventKey, bool, error) {
			ev, err := m.contract.ParseIFTReceived(log)
			if err != nil {
				return receivedEventKey{}, false, fmt.Errorf("decode IFTReceived: %w", err)
			}
			return receivedEventKey{RouteID: ev.RouteId, Sequence: ev.Seq.Uint64()}, true, nil
		},
	)
}

func (m *testAppIFT) ScanSentFrom(ctx context.Context, fromBlock uint64) ([]iftSent, uint64, error) {
	logs, next, err := m.scanEvent(ctx, "IFTSent", mockIFTABI.Events["IFTSent"].ID, fromBlock)
	if err != nil {
		return nil, fromBlock, err
	}
	out := make([]iftSent, 0, len(logs))
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

func (m *testAppIFT) SentFromReceipt(rcpt *types.Receipt) (iftSent, bool, error) {
	lg, ok := m.firstReceiptLog(rcpt, mockIFTABI.Events["IFTSent"].ID)
	if !ok {
		return iftSent{}, false, nil
	}
	ev, err := m.decodeSent(*lg)
	if err != nil {
		return iftSent{}, false, err
	}
	ev.TxHash = lg.TxHash
	return ev, true, nil
}

func (m *testAppIFT) decodeSent(log types.Log) (iftSent, error) {
	ev, err := m.contract.ParseIFTSent(log)
	if err != nil {
		return iftSent{}, fmt.Errorf("decode iftSent: %w", err)
	}
	return iftSent{
		Seq:              ev.Seq,
		RouteID:          ev.RouteId,
		Receiver:         ev.Receiver,
		Amount:           ev.Amount,
		TimeoutTimestamp: ev.TimeoutTimestamp,
	}, nil
}

// Adopt an on-chain refund before re-issuing — a second refund reverts and wedges the packet pending.
func (m *testAppIFT) FindRefunded(ctx context.Context, seq *big.Int) (common.Hash, bool, error) {
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

type gmpSent struct {
	Seq     *big.Int
	RouteID string
	Target  string
	Payload []byte
	TxHash  common.Hash
}

// Success=false is the error-ack outcome (inner target reverted), not a delivery failure.
type gmpReceived struct {
	RouteID string
	Seq     *big.Int
	Target  common.Address
	Success bool
}

type testAppGMP struct {
	boundContract
	contract *MockGMP
}

func newTestAppGMP(addr common.Address, client *ethclient.Client) *testAppGMP {
	contract, err := NewMockGMP(addr, client)
	if err != nil {
		panic(fmt.Sprintf("bind MockGMP %s: %v", addr, err))
	}
	return &testAppGMP{boundContract: newBoundContract(addr, client), contract: contract}
}

func (m *testAppGMP) Deliver(
	opts *bind.TransactOpts,
	routeID string,
	seq *big.Int,
	target common.Address,
	payload []byte,
) (*types.Transaction, error) {
	return m.contract.Deliver(opts, routeID, seq, target, payload)
}

func (m *testAppGMP) ScanReceivedFrom(
	ctx context.Context,
	fromBlock uint64,
) (map[receivedEventKey]receivedResult, uint64, error) {
	return m.scanReceived(
		ctx,
		"GMPReceived",
		mockGMPABI.Events["GMPReceived"].ID,
		fromBlock,
		func(log types.Log) (receivedEventKey, bool, error) {
			ev, err := m.decodeReceived(log)
			if err != nil {
				return receivedEventKey{}, false, err
			}
			return receivedEventKey{RouteID: ev.RouteID, Sequence: ev.Seq.Uint64()}, ev.Success, nil
		},
	)
}

// deliver() mines successfully either way; gmpReceived.success is the only inner-outcome signal.
func (m *testAppGMP) DeliveredSuccessFromReceipt(rcpt *types.Receipt) (success bool, found bool, err error) {
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

func (m *testAppGMP) ScanSentFrom(ctx context.Context, fromBlock uint64) ([]gmpSent, uint64, error) {
	logs, next, err := m.scanEvent(ctx, "GMPSent", mockGMPABI.Events["GMPSent"].ID, fromBlock)
	if err != nil {
		return nil, fromBlock, err
	}
	out := make([]gmpSent, 0, len(logs))
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

func (m *testAppGMP) SentFromReceipt(rcpt *types.Receipt) (gmpSent, bool, error) {
	lg, ok := m.firstReceiptLog(rcpt, mockGMPABI.Events["GMPSent"].ID)
	if !ok {
		return gmpSent{}, false, nil
	}
	ev, err := m.decodeSent(*lg)
	if err != nil {
		return gmpSent{}, false, err
	}
	ev.TxHash = lg.TxHash
	return ev, true, nil
}

func (m *testAppGMP) decodeSent(log types.Log) (gmpSent, error) {
	ev, err := m.contract.ParseGMPSent(log)
	if err != nil {
		return gmpSent{}, fmt.Errorf("decode gmpSent: %w", err)
	}
	return gmpSent{Seq: ev.Seq, RouteID: ev.RouteId, Target: ev.Target, Payload: ev.Payload}, nil
}

func (m *testAppGMP) decodeReceived(log types.Log) (gmpReceived, error) {
	ev, err := m.contract.ParseGMPReceived(log)
	if err != nil {
		return gmpReceived{}, fmt.Errorf("decode gmpReceived: %w", err)
	}
	return gmpReceived{RouteID: ev.RouteId, Seq: ev.Seq, Target: ev.Target, Success: ev.Success}, nil
}
