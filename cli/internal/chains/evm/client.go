// SPDX-License-Identifier: Apache-2.0

// Package evm implements the chain client for EVM chains.
package evm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"log/slog"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	hostv2 "github.com/cosmos/ibc-go/v11/modules/core/24-host/v2"
	"github.com/cosmos/ibc/cli/internal/chains/evm/contracts/attestation"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// ics26 router events consumed by the client
const (
	sendPacketEvent    = "SendPacket"
	writeAckEvent      = "WriteAcknowledgement"
	ackPacketEvent     = "AckPacket"
	timeoutPacketEvent = "TimeoutPacket"
)

// errorAcknowledgement is the universal error acknowledgement commitment.
var errorAcknowledgement = sha256.Sum256([]byte("UNIVERSAL_ERROR_ACKNOWLEDGEMENT"))

// ETHClient go-ethereum methods used by Client.
type ETHClient interface {
	bind.ContractBackend

	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	TransactionByHash(ctx context.Context, hash common.Hash) (*types.Transaction, bool, error)
	StorageAt(ctx context.Context, account common.Address, key common.Hash, blockNumber *big.Int) ([]byte, error)
}

// Client implements chains.Client for EVM chains.
type Client struct {
	chainID       string
	routerAddress common.Address
	eth           ETHClient
	ws            ETHClient // nil unless a websocket endpoint is configured
	router        *ics26router.Contract
	routerABI     *abi.ABI
	logger        *slog.Logger
}

func New(chainID, rpcURL, wsURL, ics26RouterAddress string) (*Client, error) {
	eth, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, errors.Wrapf(err, "dialing rpc for chain %s", chainID)
	}

	var ws ETHClient

	if wsURL != "" {
		dialed, errDial := ethclient.Dial(wsURL)
		if errDial != nil {
			return nil, errors.Wrapf(errDial, "dialing websocket for chain %s", chainID)
		}

		ws = dialed
	}

	return NewWithClients(chainID, eth, ws, ics26RouterAddress)
}

func NewWithClient(chainID string, eth ETHClient, ics26RouterAddress string) (*Client, error) {
	return NewWithClients(chainID, eth, nil, ics26RouterAddress)
}

func NewWithClients(chainID string, eth, ws ETHClient, ics26RouterAddress string) (*Client, error) {
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
		ws:            ws,
		router:        router,
		routerABI:     routerABI,
		logger:        slog.With("module", "chains", "chainType", "evm", "chainID", chainID),
	}, nil
}

func (c *Client) ChainID() string {
	return c.chainID
}

func (c *Client) TxPacketEvents(ctx context.Context, rawTxHash []byte) ([]v2.PacketEvent, error) {
	if len(rawTxHash) != common.HashLength {
		return nil, errors.Errorf("invalid tx hash length %d, expected %d", len(rawTxHash), common.HashLength)
	}

	txHash := common.BytesToHash(rawTxHash)

	// TODO: cache the call
	receipt, err := c.eth.TransactionReceipt(ctx, txHash)
	if err != nil {
		return nil, errors.Wrapf(err, "getting receipt for tx %s on chain %s", txHash, c.chainID)
	}

	sendPacketID := c.routerABI.Events[sendPacketEvent].ID
	writeAckID := c.routerABI.Events[writeAckEvent].ID

	var events []v2.PacketEvent

	for _, log := range receipt.Logs {
		if log == nil || len(log.Topics) == 0 || log.Address != c.routerAddress {
			continue
		}

		switch log.Topics[0] {
		case sendPacketID:
			sendPacket, errParse := c.router.ParseSendPacket(*log)
			if errParse != nil {
				return nil, errors.Wrapf(
					errParse,
					"parsing send packet event from tx %s on chain %s",
					txHash,
					c.chainID,
				)
			}

			events = append(events, v2.PacketEvent{
				Kind:   v2.KindSendPacket,
				Packet: toPacket(sendPacket.Packet),
				TxHash: txHash.String(),
			})
		case writeAckID:
			writeAck, errParse := c.router.ParseWriteAcknowledgement(*log)
			if errParse != nil {
				return nil, errors.Wrapf(errParse, "parsing write ack event from tx %s on chain %s", txHash, c.chainID)
			}

			events = append(events, v2.PacketEvent{
				Kind:   v2.KindWriteAck,
				Packet: toPacket(writeAck.Packet),
				Acks:   writeAck.Acknowledgements,
				TxHash: txHash.String(),
			})
		}
	}

	if len(events) == 0 {
		return nil, nil
	}

	header, err := c.eth.HeaderByNumber(ctx, receipt.BlockNumber)
	if err != nil {
		return nil, errors.Wrapf(err, "getting header %s for tx %s on chain %s", receipt.BlockNumber, txHash, c.chainID)
	}

	for i := range events {
		events[i].Height = receipt.BlockNumber.Uint64()
		events[i].BlockTime = blockTime(header)
	}

	return events, nil
}

func (c *Client) TxHeight(ctx context.Context, rawTxHash []byte) (uint64, error) {
	if len(rawTxHash) != common.HashLength {
		return 0, errors.Errorf("invalid tx hash length %d, expected %d", len(rawTxHash), common.HashLength)
	}

	txHash := common.BytesToHash(rawTxHash)

	receipt, err := c.eth.TransactionReceipt(ctx, txHash)
	if err != nil {
		return 0, errors.Wrapf(err, "getting receipt for tx %s on chain %s", txHash, c.chainID)
	}

	return receipt.BlockNumber.Uint64(), nil
}

func (c *Client) GetBlockHeader(ctx context.Context, height uint64) (v2.BlockHeader, error) {
	header, err := c.eth.HeaderByNumber(ctx, heightToBigInt(height))
	switch {
	case err != nil:
		return v2.BlockHeader{}, errors.Wrapf(err, "getting header for height %d", height)
	case header == nil:
		return v2.BlockHeader{}, errors.Errorf("header is nil for height %d", height)
	}

	return v2.BlockHeader{
		Height:    header.Number.Uint64(),
		Timestamp: blockTime(header),
	}, nil
}

func (c *Client) GetCommitment(ctx context.Context, height uint64, hashedPath [32]byte) ([32]byte, error) {
	opts := &bind.CallOpts{
		Context:     ctx,
		BlockNumber: heightToBigInt(height),
	}

	commitment, err := c.router.GetCommitment(opts, hashedPath)
	if err != nil {
		return [32]byte{}, errors.Wrapf(err, "getting commitment at height %d on chain %s", height, c.chainID)
	}

	return commitment, nil
}

// GetCounterparty returns the client ID this chain's router has registered
// as clientID's on-chain counterparty.
func (c *Client) GetCounterparty(ctx context.Context, clientID string) (string, error) {
	info, err := c.router.GetCounterparty(&bind.CallOpts{Context: ctx}, clientID)
	if err != nil {
		return "", errors.Wrapf(err, "querying on-chain counterparty for client %q on chain %s", clientID, c.chainID)
	}

	return info.ClientId, nil
}

// GetAttestationSet returns clientID's on-chain attestor addresses and
// minimum required signature count.
func (c *Client) GetAttestationSet(ctx context.Context, clientID string) ([]string, uint8, error) {
	lightClientAddr, err := c.router.GetClient(&bind.CallOpts{Context: ctx}, clientID)
	if err != nil {
		return nil, 0, errors.Wrapf(err, "resolving light client address for %q on chain %s", clientID, c.chainID)
	}

	lightClient, err := attestation.NewContract(lightClientAddr, c.eth)
	if err != nil {
		return nil, 0, errors.Wrapf(err, "binding attestation light client %q on chain %s", clientID, c.chainID)
	}

	set, err := lightClient.GetAttestationSet(&bind.CallOpts{Context: ctx})
	if err != nil {
		return nil, 0, errors.Wrapf(err, "querying attestation set for client %q on chain %s", clientID, c.chainID)
	}

	addresses := make([]string, len(set.AttestorAddresses))
	for i, addr := range set.AttestorAddresses {
		addresses[i] = strings.ToLower(addr.Hex())
	}

	return addresses, set.MinRequiredSigs, nil
}

func toPacket(packet ics26router.IICS26RouterMsgsPacket) channeltypesv2.Packet {
	payloads := make([]channeltypesv2.Payload, len(packet.Payloads))
	for i, payload := range packet.Payloads {
		payloads[i] = channeltypesv2.Payload{
			SourcePort:      payload.SourcePort,
			DestinationPort: payload.DestPort,
			Version:         payload.Version,
			Encoding:        payload.Encoding,
			Value:           payload.Value,
		}
	}

	return channeltypesv2.Packet{
		Sequence:          packet.Sequence,
		SourceClient:      packet.SourceClient,
		DestinationClient: packet.DestClient,
		TimeoutTimestamp:  packet.TimeoutTimestamp,
		Payloads:          payloads,
	}
}

func blockTime(header *types.Header) time.Time {
	return time.Unix(int64(header.Time), 0).UTC() //nolint:gosec // block times fit in int64
}

func (c *Client) IsPacketReceived(ctx context.Context, destClientID string, sequence uint64) (bool, error) {
	return c.commitmentExists(ctx, destClientID, sequence, hostv2.PacketReceiptKey(destClientID, sequence))
}

func (c *Client) IsPacketCommitted(ctx context.Context, sourceClientID string, sequence uint64) (bool, error) {
	return c.commitmentExists(ctx, sourceClientID, sequence, hostv2.PacketCommitmentKey(sourceClientID, sequence))
}

func (c *Client) commitmentExists(ctx context.Context, clientID string, sequence uint64, path []byte) (bool, error) {
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

// prevSequenceSends is the second field of the ibc.storage.IBCStore ERC-7201
// namespace, whose base is
// keccak256(uint256(keccak256("ibc.storage.IBCStore")) - 1) & ~0xff.
var prevSequenceSendsSlot = common.BigToHash(new(big.Int).Add(
	common.HexToHash("0x1260944489272988d9df285149b5aa1b0f48f2136d6f416159f840a3e0747600").Big(),
	big.NewInt(1),
))

// LatestPacketSequence returns the highest sequence ever assigned on clientID
// at height, read from the router's prevSequenceSends mapping because
// nextSequenceSend is internal and nothing exposes it. Sequences in use are
// 1..N, so zero means nothing has been sent on the client.
func (c *Client) LatestPacketSequence(ctx context.Context, clientID string, height uint64) (uint64, error) {
	// dynamically-sized mapping keys are hashed unpadded, unlike value-type keys
	slot := crypto.Keccak256Hash([]byte(clientID), prevSequenceSendsSlot[:])

	word, err := c.eth.StorageAt(ctx, c.routerAddress, slot, heightToBigInt(height))
	if err != nil {
		return 0, errors.Wrapf(
			err,
			"reading prevSequenceSends for client %s on chain %s at height %d",
			clientID, c.chainID, height,
		)
	}

	sequence := new(big.Int).SetBytes(word)
	if !sequence.IsUint64() {
		return 0, errors.Errorf(
			"prevSequenceSends slot for client %s on chain %s holds %s, "+
				"which is not a sequence: the ibc.storage.IBCStore layout has moved",
			clientID, c.chainID, sequence,
		)
	}

	return sequence.Uint64(), nil
}

// getCommitment reads the raw commitment slot and answers zero for an absent
// one. queryPacketCommitment is the friendlier call but reverts through
// multicall's delegatecall, and a probe has to read "settled" as a value rather
// than as a revert. It is a view either way, which the generated bindings
// expose as a call rather than as calldata, so its pack goes through the abi
// while the multicall wrapping it goes through the binding.
const commitmentMethod = "getCommitment"

// routerCalls packs router calldata through the generated bindings, so a
// signature change breaks the build rather than an eth_call.
var routerCalls = mustRouterCalls()

func mustRouterCalls() *ics26router.ContractTransactor {
	bound, err := ics26router.NewContractTransactor(common.Address{}, nil)
	if err != nil {
		panic(errors.Wrap(err, "constructing ics26 router binding"))
	}

	return bound
}

// routerCalldata runs a binding call with sending disabled and returns the
// input it would have submitted.
func routerCalldata(call func(*bind.TransactOpts) (*types.Transaction, error)) ([]byte, error) {
	opts := &bind.TransactOpts{
		Nonce:    new(big.Int),
		Signer:   func(_ common.Address, tx *types.Transaction) (*types.Transaction, error) { return tx, nil },
		GasLimit: 1,
		GasPrice: big.NewInt(1),
		NoSend:   true,
	}

	tx, err := call(opts)
	if err != nil {
		return nil, err
	}

	return tx.Data(), nil
}

// commitmentProbeChunk is the number of commitment probes per multicall.
// Memory expansion inside multicall is quadratic in the results it copies, so
// gas binds near 10,000 probes under geth's default 50M cap; 1000 keeps a 14x
// margin at a 256 KB request payload.
const commitmentProbeChunk = 1000

// PacketCommitments returns the subset of sequences whose packet commitment is
// still live on clientID at height. It is all or nothing: a truncated set reads
// as "everything else settled", which writes off packets whose funds are still
// in escrow, so any failure returns nil rather than a partial slice.
func (c *Client) PacketCommitments(
	ctx context.Context,
	clientID string,
	sequences []uint64,
	height uint64,
) ([]uint64, error) {
	var live []uint64

	for chunk := range slices.Chunk(sequences, commitmentProbeChunk) {
		found, err := c.probeCommitments(ctx, clientID, chunk, height)
		if err != nil {
			return nil, err
		}

		live = append(live, found...)
	}

	return live, nil
}

func (c *Client) probeCommitments(
	ctx context.Context,
	clientID string,
	sequences []uint64,
	height uint64,
) ([]uint64, error) {
	calls := make([][]byte, len(sequences))

	for i, sequence := range sequences {
		call, err := c.routerABI.Pack(
			commitmentMethod,
			crypto.Keccak256Hash(hostv2.PacketCommitmentKey(clientID, sequence)),
		)
		if err != nil {
			return nil, errors.Wrapf(
				err,
				"packing %s for client %s sequence %d",
				commitmentMethod, clientID, sequence,
			)
		}

		calls[i] = call
	}

	input, err := routerCalldata(func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return routerCalls.Multicall(opts, calls)
	})
	if err != nil {
		return nil, errors.Wrapf(err, "packing multicall for client %s on chain %s", clientID, c.chainID)
	}

	// an explicit height, never the latest tag: behind a load-balanced endpoint
	// the tag resolves on whichever node serves the call, and one that has not
	// caught up reads a live commitment as absent, which means settled
	output, err := c.eth.CallContract(
		ctx,
		ethereum.CallMsg{To: &c.routerAddress, Data: input},
		heightToBigInt(height),
	)
	if err != nil {
		return nil, errors.Wrapf(
			err,
			"probing %d commitments for client %s on chain %s at height %d",
			len(sequences), clientID, c.chainID, height,
		)
	}

	unpacked, err := c.routerABI.Unpack("multicall", output)
	if err != nil {
		return nil, errors.Wrapf(err, "unpacking multicall for client %s on chain %s", clientID, c.chainID)
	}
	if len(unpacked) != 1 {
		return nil, errors.Errorf(
			"multicall for client %s on chain %s returned %d values, expected 1",
			clientID, c.chainID, len(unpacked),
		)
	}

	results, ok := unpacked[0].([][]byte)
	if !ok || len(results) != len(sequences) {
		return nil, errors.Errorf(
			"multicall for client %s on chain %s returned %d %T results, expected %d commitments",
			clientID, c.chainID, len(results), unpacked[0], len(sequences),
		)
	}

	var live []uint64
	for i, result := range results {
		if len(result) != common.HashLength {
			return nil, errors.Errorf(
				"%s for client %s sequence %d on chain %s returned %d bytes, expected %d",
				commitmentMethod, clientID, sequences[i], c.chainID, len(result), common.HashLength,
			)
		}

		if common.BytesToHash(result) != (common.Hash{}) {
			live = append(live, sequences[i])
		}
	}

	return live, nil
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
	query, err := c.packetLogQuery(eventName, []any{clientID}, []any{sequence})
	if err != nil {
		return nil, err
	}

	logs, err := c.eth.FilterLogs(ctx, query)
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
		c.logger.Error("Found packet tx but could not determine its sender", "txHash", log.TxHash, "err", err)
	}

	return &v2.Tx{
		Hash:           log.TxHash.String(),
		Timestamp:      time.Unix(int64(header.Time), 0).UTC(),
		RelayerAddress: sender,
	}, nil
}

// subscriptionLogBuffer matches the buffer abigen's generated watchers use.
const subscriptionLogBuffer = 128

// SubscribeSendPackets streams SendPacket events for clientIDs to out. It is one
// shot: the subscription ends on the first transport, decode or header failure
// and does not reconnect.
func (c *Client) SubscribeSendPackets(
	ctx context.Context,
	clientIDs []string,
	out chan<- v2.PacketEvent,
) (v2.Subscription, error) {
	switch {
	case c.ws == nil:
		return nil, errors.Errorf("no websocket endpoint configured for chain %s", c.chainID)
	case len(clientIDs) == 0:
		return nil, errors.Errorf("no client ids to subscribe to on chain %s", c.chainID)
	}

	query, err := c.packetLogQuery(sendPacketEvent, toAnySlice(clientIDs), nil)
	if err != nil {
		return nil, err
	}

	logs := make(chan types.Log, subscriptionLogBuffer)

	sub, err := c.ws.SubscribeFilterLogs(ctx, query, logs)
	if err != nil {
		return nil, errors.Wrapf(err, "subscribing to %s logs on chain %s", sendPacketEvent, c.chainID)
	}

	blockTimeAt := c.blockTimer()

	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()

		for {
			select {
			case log := <-logs:
				packetEvent, err := c.sendPacketEvent(ctx, log, blockTimeAt)
				if err != nil {
					// this error may have been due to a block header http
					// fetch failure, however that data is required for send
					// packets (in order to make finality decisions within the
					// relaying pipeline), so we are still always returning
					// this error and force the caller to reconnect instead of
					// returning a
					// zero value for the time.
					return err
				}

				select {
				case out <- packetEvent:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

func (c *Client) sendPacketEvent(
	ctx context.Context,
	log types.Log,
	blockTimeAt func(context.Context, uint64) (time.Time, error),
) (v2.PacketEvent, error) {
	sendPacket, err := c.router.ParseSendPacket(log)
	if err != nil {
		return v2.PacketEvent{}, errors.Wrapf(
			err,
			"parsing send packet event from tx %s on chain %s",
			log.TxHash,
			c.chainID,
		)
	}

	timestamp, err := blockTimeAt(ctx, log.BlockNumber)
	if err != nil {
		return v2.PacketEvent{}, errors.Wrapf(
			err,
			"fetching block time at block %d on chain %s",
			log.BlockNumber,
			c.chainID,
		)
	}

	return v2.PacketEvent{
		Height:    log.BlockNumber,
		BlockTime: timestamp,
		Kind:      v2.KindSendPacket,
		Packet:    toPacket(sendPacket.Packet),
		TxHash:    log.TxHash.String(),
		Removed:   log.Removed,
	}, nil
}

// blockTimer memoizes the last block looked up, which is nearly always a hit
// because logs arrive in block order. It reads headers over HTTP so header
// traffic stays off the websocket connection.
func (c *Client) blockTimer() func(ctx context.Context, blockNumber uint64) (time.Time, error) {
	var (
		cachedNumber uint64
		cached       time.Time
	)

	return func(ctx context.Context, blockNumber uint64) (time.Time, error) {
		if !cached.IsZero() && cachedNumber == blockNumber {
			return cached, nil
		}

		header, err := c.eth.HeaderByNumber(ctx, new(big.Int).SetUint64(blockNumber))
		if err != nil {
			return time.Time{}, errors.Wrapf(err, "getting header %d on chain %s", blockNumber, c.chainID)
		}

		cachedNumber, cached = blockNumber, blockTime(header)

		return cached, nil
	}
}

func toAnySlice[T any](values []T) []any {
	out := make([]any, len(values))
	for i, value := range values {
		out[i] = value
	}

	return out
}

// packetLogQuery builds the router log filter for a packet event. An empty
// clientIDs or sequences leaves that topic position a wildcard.
func (c *Client) packetLogQuery(eventName string, clientIDs, sequences []any) (ethereum.FilterQuery, error) {
	topics, err := abi.MakeTopics([]any{c.routerABI.Events[eventName].ID}, clientIDs, sequences)
	if err != nil {
		return ethereum.FilterQuery{}, errors.Wrapf(
			err,
			"creating %s topics for clients %v sequences %v",
			eventName, clientIDs, sequences,
		)
	}

	return ethereum.FilterQuery{
		Addresses: []common.Address{c.routerAddress},
		Topics:    topics,
	}, nil
}

// FindSendPackets returns the SendPacket events for sequences on clientID. The
// block range is unbounded because clearing knows the sequences but not the
// heights they landed at, exactly as findPacketTx queries today.
func (c *Client) FindSendPackets(
	ctx context.Context,
	clientID string,
	sequences []uint64,
) ([]v2.PacketEvent, error) {
	if len(sequences) == 0 {
		return nil, nil
	}

	query, err := c.packetLogQuery(sendPacketEvent, []any{clientID}, toAnySlice(sequences))
	if err != nil {
		return nil, err
	}

	logs, err := c.eth.FilterLogs(ctx, query)
	if err != nil {
		return nil, errors.Wrapf(err, "filtering %s logs on chain %s", sendPacketEvent, c.chainID)
	}

	blockTimeAt, err := c.blockTimes(ctx, logs)
	if err != nil {
		return nil, err
	}

	events := make([]v2.PacketEvent, 0, len(logs))

	for _, log := range logs {
		event, err := c.sendPacketEvent(ctx, log, blockTimeAt)
		if err != nil {
			return nil, err
		}

		events = append(events, event)
	}

	return events, nil
}

// headerFetchLimit bounds the concurrent header reads one log batch issues, so
// recovering a backlog does not arrive at a provider as a burst.
const headerFetchLimit = 8

// blockTimes resolves the distinct block times behind logs up front. One
// eth_getLogs covers a whole chunk of sequences, but every block under it is
// its own round trip, so fetching them one at a time is what makes a recovery
// pass slow; these are independent reads and run as such.
func (c *Client) blockTimes(
	ctx context.Context,
	logs []types.Log,
) (func(context.Context, uint64) (time.Time, error), error) {
	seen := make(map[uint64]struct{}, len(logs))

	var numbers []uint64

	for _, log := range logs {
		if _, ok := seen[log.BlockNumber]; ok {
			continue
		}

		seen[log.BlockNumber] = struct{}{}
		numbers = append(numbers, log.BlockNumber)
	}

	// indexed writes rather than a shared map, so the results need no lock
	times := make([]time.Time, len(numbers))

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(headerFetchLimit)

	for i, number := range numbers {
		group.Go(func() error {
			header, err := c.eth.HeaderByNumber(groupCtx, new(big.Int).SetUint64(number))
			if err != nil {
				return errors.Wrapf(err, "getting header %d on chain %s", number, c.chainID)
			}

			times[i] = blockTime(header)

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, err
	}

	at := make(map[uint64]time.Time, len(numbers))
	for i, number := range numbers {
		at[number] = times[i]
	}

	return func(_ context.Context, blockNumber uint64) (time.Time, error) {
		blockTimeAt, ok := at[blockNumber]
		if !ok {
			return time.Time{}, errors.Errorf(
				"no header fetched for block %d on chain %s", blockNumber, c.chainID,
			)
		}

		return blockTimeAt, nil
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

var (
	blockFinalized = big.NewInt(rpc.FinalizedBlockNumber.Int64())
	blockLatest    = big.NewInt(rpc.LatestBlockNumber.Int64())
)

func heightToBigInt(height uint64) *big.Int {
	switch height {
	case v2.LatestBlock:
		return blockLatest
	case v2.FinalizedBlock:
		return blockFinalized
	default:
		return new(big.Int).SetUint64(height)
	}
}
