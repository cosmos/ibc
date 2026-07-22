package ibcrelay

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"

	channelv2types "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
)

var zeroProofHeight = ics26router.IICS02ClientMsgsHeight{}

// Error acknowledgement sentinel written when the destination app reverts.
var errorAckSentinel = channelv2types.ErrorAcknowledgement[:]

type chainConn struct {
	id         string
	client     *ethclient.Client
	chainID    *big.Int
	signerKey  *ecdsa.PrivateKey
	routerAddr common.Address
	ops        chainOps
}

// chainOps abstracts ICS26 chain interactions so the loop can be unit-tested with a fake.
type chainOps interface {
	blockTimestamp(ctx context.Context) (uint64, error)
	submitRecv(ctx context.Context, packet ics26router.IICS26RouterMsgsPacket) (*types.Receipt, error)
	submitAck(ctx context.Context, packet ics26router.IICS26RouterMsgsPacket, ack []byte) (*types.Receipt, error)
	submitTimeout(ctx context.Context, packet ics26router.IICS26RouterMsgsPacket) (*types.Receipt, error)
	writeAckFromReceipt(rcpt *types.Receipt) ([]byte, bool, error)
	findWriteAck(ctx context.Context, destClient string, seq uint64) (ackCapture, bool, error)
	findAckPacket(ctx context.Context, sourceClient string, seq uint64) (ackCapture, bool, error)
	findTimeoutPacket(ctx context.Context, sourceClient string, seq uint64) (common.Hash, bool, error)
	scanSendPackets(ctx context.Context, fromBlock uint64) ([]sentPacket, uint64, error)
	sendPacketsFromTx(ctx context.Context, txHash common.Hash) ([]sentPacket, error)
}

type ackCapture struct {
	Ack    []byte
	TxHash common.Hash
}

type sentPacket struct {
	Packet ics26router.IICS26RouterMsgsPacket
	TxHash common.Hash
}

type evmOps struct {
	conn   *chainConn
	router *ics26router.Contract
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

func newEVMOps(conn *chainConn) (*evmOps, error) {
	router, err := ics26router.NewContract(conn.routerAddr, conn.client)
	if err != nil {
		return nil, fmt.Errorf("bind ics26 router %s: %w", conn.routerAddr.Hex(), err)
	}
	return &evmOps{conn: conn, router: router}, nil
}

func (o *evmOps) blockTimestamp(ctx context.Context) (uint64, error) {
	hdr, err := o.conn.client.HeaderByNumber(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("destination head: %w", err)
	}
	return hdr.Time, nil
}

func (o *evmOps) submitRecv(
	ctx context.Context,
	packet ics26router.IICS26RouterMsgsPacket,
) (*types.Receipt, error) {
	opts, err := newTransactor(ctx, o.conn.signerKey, o.conn.chainID)
	if err != nil {
		return nil, err
	}
	tx, err := o.router.RecvPacket(opts, ics26router.IICS26RouterMsgsMsgRecvPacket{
		Packet:          packet,
		ProofCommitment: []byte{},
		ProofHeight:     zeroProofHeight,
	})
	if err != nil {
		return nil, fmt.Errorf("submit recvPacket: %w", err)
	}
	rcpt, err := waitMined(ctx, o.conn.client, tx)
	if err != nil {
		return nil, err
	}
	if rcpt.Status != types.ReceiptStatusSuccessful {
		return nil, fmt.Errorf("recvPacket reverted (tx %s)", tx.Hash().Hex())
	}
	return rcpt, nil
}

func (o *evmOps) submitAck(
	ctx context.Context,
	packet ics26router.IICS26RouterMsgsPacket,
	ack []byte,
) (*types.Receipt, error) {
	opts, err := newTransactor(ctx, o.conn.signerKey, o.conn.chainID)
	if err != nil {
		return nil, err
	}
	tx, err := o.router.AckPacket(opts, ics26router.IICS26RouterMsgsMsgAckPacket{
		Packet:          packet,
		Acknowledgement: ack,
		ProofAcked:      []byte{},
		ProofHeight:     zeroProofHeight,
	})
	if err != nil {
		return nil, fmt.Errorf("submit ackPacket: %w", err)
	}
	rcpt, err := waitMined(ctx, o.conn.client, tx)
	if err != nil {
		return nil, err
	}
	if rcpt.Status != types.ReceiptStatusSuccessful {
		return nil, fmt.Errorf("ackPacket reverted (tx %s)", tx.Hash().Hex())
	}
	return rcpt, nil
}

func (o *evmOps) submitTimeout(
	ctx context.Context,
	packet ics26router.IICS26RouterMsgsPacket,
) (*types.Receipt, error) {
	opts, err := newTransactor(ctx, o.conn.signerKey, o.conn.chainID)
	if err != nil {
		return nil, err
	}
	tx, err := o.router.TimeoutPacket(opts, ics26router.IICS26RouterMsgsMsgTimeoutPacket{
		Packet:       packet,
		ProofTimeout: []byte{},
		ProofHeight:  zeroProofHeight,
	})
	if err != nil {
		return nil, fmt.Errorf("submit timeoutPacket: %w", err)
	}
	rcpt, err := waitMined(ctx, o.conn.client, tx)
	if err != nil {
		return nil, err
	}
	if rcpt.Status != types.ReceiptStatusSuccessful {
		return nil, fmt.Errorf("timeoutPacket reverted (tx %s)", tx.Hash().Hex())
	}
	return rcpt, nil
}

func (o *evmOps) findWriteAck(
	ctx context.Context,
	destClient string,
	seq uint64,
) (ackCapture, bool, error) {
	it, err := o.router.FilterWriteAcknowledgement(
		&bind.FilterOpts{Context: ctx, Start: 0},
		nil,
		[]*big.Int{new(big.Int).SetUint64(seq)},
	)
	if err != nil {
		return ackCapture{}, false, fmt.Errorf("filter WriteAcknowledgement: %w", err)
	}
	defer it.Close() //nolint:errcheck
	for it.Next() {
		ev := it.Event
		if ev.Packet.Sequence != seq || ev.Packet.DestClient != destClient {
			continue
		}
		if len(ev.Acknowledgements) == 0 {
			return ackCapture{}, false, fmt.Errorf("WriteAcknowledgement missing acknowledgements")
		}
		return ackCapture{Ack: ev.Acknowledgements[0], TxHash: ev.Raw.TxHash}, true, nil
	}
	if err := it.Error(); err != nil {
		return ackCapture{}, false, err
	}
	return ackCapture{}, false, nil
}

func (o *evmOps) findAckPacket(
	ctx context.Context,
	sourceClient string,
	seq uint64,
) (ackCapture, bool, error) {
	it, err := o.router.FilterAckPacket(
		&bind.FilterOpts{Context: ctx, Start: 0},
		nil,
		[]*big.Int{new(big.Int).SetUint64(seq)},
	)
	if err != nil {
		return ackCapture{}, false, fmt.Errorf("filter AckPacket: %w", err)
	}
	defer it.Close() //nolint:errcheck
	for it.Next() {
		ev := it.Event
		if ev.Packet.Sequence != seq || ev.Packet.SourceClient != sourceClient {
			continue
		}
		return ackCapture{Ack: ev.Acknowledgement, TxHash: ev.Raw.TxHash}, true, nil
	}
	if err := it.Error(); err != nil {
		return ackCapture{}, false, err
	}
	return ackCapture{}, false, nil
}

func (o *evmOps) findTimeoutPacket(
	ctx context.Context,
	sourceClient string,
	seq uint64,
) (common.Hash, bool, error) {
	it, err := o.router.FilterTimeoutPacket(
		&bind.FilterOpts{Context: ctx, Start: 0},
		nil,
		[]*big.Int{new(big.Int).SetUint64(seq)},
	)
	if err != nil {
		return common.Hash{}, false, fmt.Errorf("filter TimeoutPacket: %w", err)
	}
	defer it.Close() //nolint:errcheck
	for it.Next() {
		ev := it.Event
		if ev.Packet.Sequence != seq || ev.Packet.SourceClient != sourceClient {
			continue
		}
		return ev.Raw.TxHash, true, nil
	}
	if err := it.Error(); err != nil {
		return common.Hash{}, false, err
	}
	return common.Hash{}, false, nil
}

func (o *evmOps) scanSendPackets(ctx context.Context, fromBlock uint64) ([]sentPacket, uint64, error) {
	head, err := o.conn.client.BlockNumber(ctx)
	if err != nil {
		return nil, fromBlock, fmt.Errorf("head for SendPacket scan: %w", err)
	}
	if head < fromBlock {
		return nil, fromBlock, nil
	}
	it, err := o.router.FilterSendPacket(&bind.FilterOpts{
		Context: ctx,
		Start:   fromBlock,
		End:     &head,
	}, nil, nil)
	if err != nil {
		return nil, fromBlock, fmt.Errorf("filter SendPacket: %w", err)
	}
	defer it.Close() //nolint:errcheck

	var out []sentPacket
	for it.Next() {
		ev := it.Event
		out = append(out, sentPacket{Packet: ev.Packet, TxHash: ev.Raw.TxHash})
	}
	if err := it.Error(); err != nil {
		return nil, fromBlock, err
	}
	return out, head + 1, nil
}

func (o *evmOps) sendPacketsFromTx(ctx context.Context, txHash common.Hash) ([]sentPacket, error) {
	rcpt, err := o.conn.client.TransactionReceipt(ctx, txHash)
	if err != nil {
		return nil, err
	}
	if rcpt.Status != types.ReceiptStatusSuccessful {
		return nil, fmt.Errorf("source transaction %s failed", rcpt.TxHash.Hex())
	}
	var out []sentPacket
	for _, lg := range rcpt.Logs {
		if lg.Address != o.conn.routerAddr {
			continue
		}
		ev, err := o.router.ParseSendPacket(*lg)
		if err != nil {
			continue
		}
		out = append(out, sentPacket{Packet: ev.Packet, TxHash: lg.TxHash})
	}
	return out, nil
}

func (o *evmOps) writeAckFromReceipt(rcpt *types.Receipt) ([]byte, bool, error) {
	for _, lg := range rcpt.Logs {
		if lg.Address != o.conn.routerAddr {
			continue
		}
		ev, err := o.router.ParseWriteAcknowledgement(*lg)
		if err != nil {
			continue
		}
		if len(ev.Acknowledgements) == 0 {
			return nil, false, fmt.Errorf("WriteAcknowledgement missing acknowledgements")
		}
		return ev.Acknowledgements[0], true, nil
	}
	return nil, false, nil
}

func isErrorAck(ack []byte) bool {
	return bytes.Equal(ack, errorAckSentinel)
}

func ackHex(ack []byte) string {
	return hexutil.Encode(ack)
}
