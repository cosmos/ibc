// Package evm implements the chain client for EVM chains.
package evm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"log/slog"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains/evm/contracts/ics26router"

	v2 "github.com/cosmos/ibc/link/internal/types/v2"
	ethereum "github.com/ethereum/go-ethereum"
)

// ics26 router events consumed by the client
const (
	sendPacketEvent    = "SendPacket"
	writeAckEvent      = "WriteAcknowledgement"
	ackPacketEvent     = "AckPacket"
	timeoutPacketEvent = "TimeoutPacket"
)

// commitment path kinds per ICS-24
const (
	packetCommitmentKind byte = 1
	packetReceiptKind    byte = 2
)

// errorAcknowledgement is the universal error acknowledgement commitment.
var errorAcknowledgement = sha256.Sum256([]byte("UNIVERSAL_ERROR_ACKNOWLEDGEMENT"))

// ETHClient go-ethereum methods used by Client.
type ETHClient interface {
	bind.ContractBackend

	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	TransactionByHash(ctx context.Context, hash common.Hash) (*types.Transaction, bool, error)
}

// Client implements chains.Client for EVM chains.
type Client struct {
	chainID       string
	routerAddress common.Address
	eth           ETHClient
	router        *ics26router.Contract
	routerABI     *abi.ABI
	logger        *slog.Logger
}

func New(chainID, rpcURL, ics26RouterAddress string) (*Client, error) {
	eth, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, errors.Wrapf(err, "dialing rpc for chain %s", chainID)
	}

	return NewWithClient(chainID, eth, ics26RouterAddress)
}

func NewWithClient(chainID string, eth ETHClient, ics26RouterAddress string) (*Client, error) {
	if !common.IsHexAddress(ics26RouterAddress) {
		return nil, errors.Errorf("invalid ics26 router address %q for chain %s", ics26RouterAddress, chainID)
	}

	routerAddress := common.HexToAddress(ics26RouterAddress)

	router, err := ics26router.NewContract(routerAddress, eth)
	if err != nil {
		return nil, errors.Wrap(err, "creating ics26 router binding")
	}

	routerABI, err := ics26router.ContractMetaData.GetAbi()
	if err != nil {
		return nil, errors.Wrap(err, "getting ics26 router abi")
	}

	for _, event := range []string{sendPacketEvent, writeAckEvent, ackPacketEvent, timeoutPacketEvent} {
		if _, ok := routerABI.Events[event]; !ok {
			return nil, errors.Errorf("event %q not found in ics26 router abi", event)
		}
	}

	return &Client{
		chainID:       chainID,
		routerAddress: routerAddress,
		eth:           eth,
		router:        router,
		routerABI:     routerABI,
		logger:        slog.With("module", "chains", "chainType", "evm", "chainID", chainID),
	}, nil
}

func (c *Client) TxPacketEvents(ctx context.Context, rawTxHash []byte) ([]v2.PacketEvent, error) {
	if len(rawTxHash) != common.HashLength {
		return nil, errors.Errorf("invalid tx hash length %d, expected %d", len(rawTxHash), common.HashLength)
	}

	txHash := common.BytesToHash(rawTxHash)

	receipt, err := c.eth.TransactionReceipt(ctx, txHash)
	if err != nil {
		return nil, errors.Wrapf(err, "getting receipt for tx %s on chain %s", txHash, c.chainID)
	}

	var packets []v2.Packet

	for _, log := range receipt.Logs {
		switch {
		case log == nil, len(log.Topics) == 0:
			continue
		case log.Address != c.routerAddress:
			continue
		case log.Topics[0] != c.routerABI.Events[sendPacketEvent].ID:
			continue
		}

		sendPacket, errParse := c.router.ParseSendPacket(*log)
		if errParse != nil {
			return nil, errors.Wrapf(errParse, "parsing send packet event from tx %s on chain %s", txHash, c.chainID)
		}

		packets = append(packets, toPacket(sendPacket.Packet))
	}

	if len(packets) == 0 {
		return nil, nil
	}

	header, err := c.eth.HeaderByNumber(ctx, receipt.BlockNumber)
	if err != nil {
		return nil, errors.Wrapf(err, "getting header %s for tx %s on chain %s", receipt.BlockNumber, txHash, c.chainID)
	}

	events := make([]v2.PacketEvent, len(packets))
	for i, packet := range packets {
		events[i] = v2.PacketEvent{
			Height:    receipt.BlockNumber.Uint64(),
			BlockTime: blockTime(header),
			Kind:      v2.KindSendPacket,
			Packet:    packet,
		}
	}

	return events, nil
}

func toPacket(packet ics26router.IICS26RouterMsgsPacket) v2.Packet {
	payloads := make([]v2.Payload, len(packet.Payloads))
	for i, payload := range packet.Payloads {
		payloads[i] = v2.Payload{
			SourcePort: payload.SourcePort,
			DestPort:   payload.DestPort,
			Version:    payload.Version,
			Encoding:   payload.Encoding,
			Value:      payload.Value,
		}
	}

	return v2.Packet{
		Sequence:         packet.Sequence,
		SourceClient:     packet.SourceClient,
		DestClient:       packet.DestClient,
		TimeoutTimestamp: packet.TimeoutTimestamp,
		Payloads:         payloads,
	}
}

func blockTime(header *types.Header) time.Time {
	return time.Unix(int64(header.Time), 0).UTC() //nolint:gosec // block times fit in int64
}

func (c *Client) IsPacketReceived(ctx context.Context, destClientID string, sequence uint64) (bool, error) {
	return c.commitmentExists(ctx, destClientID, packetReceiptKind, sequence)
}

func (c *Client) IsPacketCommitted(ctx context.Context, sourceClientID string, sequence uint64) (bool, error) {
	return c.commitmentExists(ctx, sourceClientID, packetCommitmentKind, sequence)
}

func (c *Client) commitmentExists(ctx context.Context, clientID string, kind byte, sequence uint64) (bool, error) {
	path := commitmentPath(clientID, kind, sequence)

	commitment, err := c.router.GetCommitment(&bind.CallOpts{Context: ctx}, crypto.Keccak256Hash(path))
	if err != nil {
		return false, errors.Wrapf(
			err,
			"getting commitment for client %s sequence %d on chain %s",
			clientID,
			sequence,
			c.chainID,
		)
	}

	// an absent commitment reads as uninitialized storage: 32 zero bytes
	return commitment != [32]byte{}, nil
}

// commitmentPath builds the ICS-24 provable path: clientID ++ kind ++ big-endian sequence.
func commitmentPath(clientID string, kind byte, sequence uint64) []byte {
	var buf bytes.Buffer

	buf.WriteString(clientID)
	buf.WriteByte(kind)

	sequenceBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(sequenceBytes, sequence)
	buf.Write(sequenceBytes)

	return buf.Bytes()
}

// FindRecvTx looks for the WriteAcknowledgement event because the router emits
// no RecvPacket event; acks are written synchronously in the receive tx.
func (c *Client) FindRecvTx(ctx context.Context, destClientID string, sequence uint64) (*v2.Tx, error) {
	return c.findPacketTx(ctx, writeAckEvent, destClientID, sequence)
}

func (c *Client) FindAckTx(ctx context.Context, sourceClientID string, sequence uint64) (*v2.Tx, error) {
	return c.findPacketTx(ctx, ackPacketEvent, sourceClientID, sequence)
}

func (c *Client) FindTimeoutTx(ctx context.Context, sourceClientID string, sequence uint64) (*v2.Tx, error) {
	return c.findPacketTx(ctx, timeoutPacketEvent, sourceClientID, sequence)
}

func (c *Client) findPacketTx(ctx context.Context, eventName, clientID string, sequence uint64) (*v2.Tx, error) {
	topics, err := abi.MakeTopics(
		[]any{c.routerABI.Events[eventName].ID},
		[]any{clientID},
		[]any{sequence},
	)
	if err != nil {
		return nil, errors.Wrapf(err, "creating %s topics for client %s sequence %d", eventName, clientID, sequence)
	}

	logs, err := c.eth.FilterLogs(ctx, ethereum.FilterQuery{
		Addresses: []common.Address{c.routerAddress},
		Topics:    topics,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "filtering %s logs on chain %s", eventName, c.chainID)
	}

	switch {
	case len(logs) == 0:
		return nil, v2.ErrTxNotFound
	case len(logs) != 1:
		return nil, errors.Errorf(
			"expected 1 %s log for client %s sequence %d on chain %s, got %d",
			eventName, clientID, sequence, c.chainID, len(logs),
		)
	}

	log := logs[0]

	header, err := c.eth.HeaderByNumber(ctx, new(big.Int).SetUint64(log.BlockNumber))
	if err != nil {
		return nil, errors.Wrapf(err, "getting header %d on chain %s", log.BlockNumber, c.chainID)
	}

	// prefer missing sender info over failing the lookup
	sender, err := c.txSender(ctx, log.TxHash)
	if err != nil {
		c.logger.Error("Found packet tx but could not determine its sender", "txHash", log.TxHash, "error", err)
	}

	return &v2.Tx{
		Hash:           log.TxHash.String(),
		Timestamp:      time.Unix(int64(header.Time), 0).UTC(),
		RelayerAddress: sender,
	}, nil
}

func (c *Client) txSender(ctx context.Context, hash common.Hash) (string, error) {
	tx, _, err := c.eth.TransactionByHash(ctx, hash)
	if err != nil {
		return "", errors.Wrapf(err, "getting tx %s", hash)
	}

	chainID, ok := new(big.Int).SetString(c.chainID, 10)
	if !ok {
		return "", errors.Errorf("invalid evm chain id %q", c.chainID)
	}

	sender, err := types.Sender(types.LatestSignerForChainID(chainID), tx)
	if err != nil {
		return "", errors.Wrapf(err, "recovering sender of tx %s", hash)
	}

	return sender.String(), nil
}

func (c *Client) PacketWriteAckStatus(
	ctx context.Context,
	recvTxHash string,
	sequence uint64,
	sourceClientID string,
	destClientID string,
) (v2.WriteAckStatus, error) {
	receipt, err := c.eth.TransactionReceipt(ctx, common.HexToHash(recvTxHash))
	if err != nil {
		if errors.Is(err, ethereum.NotFound) {
			return v2.WriteAckStatusUnknown, v2.ErrTxNotFound
		}

		return v2.WriteAckStatusUnknown, errors.Wrapf(
			err,
			"getting receipt for tx %s on chain %s",
			recvTxHash,
			c.chainID,
		)
	}

	for _, log := range receipt.Logs {
		switch {
		case log == nil, len(log.Topics) == 0:
			continue
		case log.Topics[0] != c.routerABI.Events[writeAckEvent].ID:
			continue
		}

		writeAck, errParse := c.router.ParseWriteAcknowledgement(*log)
		if errParse != nil {
			return v2.WriteAckStatusUnknown, errors.Wrapf(
				v2.ErrWriteAckDecoding,
				"parsing write ack from tx %s: %s",
				recvTxHash,
				errParse,
			)
		}

		packet := writeAck.Packet
		if packet.Sequence != sequence || packet.SourceClient != sourceClientID || packet.DestClient != destClientID {
			continue
		}

		if len(writeAck.Acknowledgements) == 1 && bytes.Equal(writeAck.Acknowledgements[0], errorAcknowledgement[:]) {
			return v2.WriteAckStatusError, nil
		}

		return v2.WriteAckStatusSuccess, nil
	}

	return v2.WriteAckStatusUnknown, v2.ErrWriteAckNotFoundForPacket
}

func (c *Client) IsTxFinalized(ctx context.Context, txHash string, finalityOffset *uint64) (bool, error) {
	receipt, err := c.eth.TransactionReceipt(ctx, common.HexToHash(txHash))
	if err != nil {
		if errors.Is(err, ethereum.NotFound) {
			return false, v2.ErrTxNotFound
		}

		return false, errors.Wrapf(err, "getting receipt for tx %s on chain %s", txHash, c.chainID)
	}

	if finalityOffset == nil {
		finalized, errHeader := c.eth.HeaderByNumber(ctx, big.NewInt(rpc.FinalizedBlockNumber.Int64()))
		if errHeader != nil {
			return false, errors.Wrapf(errHeader, "getting finalized header on chain %s", c.chainID)
		}

		return receipt.BlockNumber.Cmp(finalized.Number) <= 0, nil
	}

	latest, err := c.eth.HeaderByNumber(ctx, nil)
	if err != nil {
		return false, errors.Wrapf(err, "getting latest header on chain %s", c.chainID)
	}

	txBlock := receipt.BlockNumber.Uint64()
	latestBlock := latest.Number.Uint64()

	return latestBlock >= txBlock && latestBlock-txBlock >= *finalityOffset, nil
}

func (c *Client) IsTimestampFinalized(ctx context.Context, timestamp time.Time, finalityOffset *uint64) (bool, error) {
	var header *types.Header

	if finalityOffset == nil {
		finalized, errHeader := c.eth.HeaderByNumber(ctx, big.NewInt(rpc.FinalizedBlockNumber.Int64()))
		if errHeader != nil {
			return false, errors.Wrapf(errHeader, "getting finalized header on chain %s", c.chainID)
		}

		header = finalized
	} else {
		latest, errHeader := c.eth.HeaderByNumber(ctx, nil)
		if errHeader != nil {
			return false, errors.Wrapf(errHeader, "getting latest header on chain %s", c.chainID)
		}

		if latest.Number.Uint64() < *finalityOffset {
			return false, nil
		}

		finalized, errHeader := c.eth.HeaderByNumber(ctx, new(big.Int).SetUint64(latest.Number.Uint64()-*finalityOffset))
		if errHeader != nil {
			return false, errors.Wrapf(errHeader, "getting offset header on chain %s", c.chainID)
		}

		header = finalized
	}

	blockTime := time.Unix(int64(header.Time), 0)

	return !blockTime.Before(timestamp), nil
}

func (c *Client) WaitForChain(ctx context.Context) error {
	const initialTick = time.Millisecond
	const tick = time.Second

	ticker := time.NewTicker(initialTick)
	defer ticker.Stop()

	start := time.Now()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			latest, err := c.eth.HeaderByNumber(ctx, nil)
			if err != nil {
				return errors.Wrapf(err, "getting latest header on chain %s", c.chainID)
			}

			latestTime := time.Unix(int64(latest.Time), 0)
			if latestTime.After(start) {
				return nil
			}

			if time.Since(start) > 30*time.Second {
				c.logger.Warn("Chain time is behind current time for more than 30s, waiting", "chainTime", latestTime)
			} else {
				c.logger.Debug("Chain time is behind current time, waiting", "chainTime", latestTime)
			}

			ticker.Reset(tick)
		}
	}
}
