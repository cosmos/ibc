// Package evm implements the chain client for EVM chains.
package evm

import (
	"context"
	"log/slog"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains/evm/contracts/ics26router"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

const sendPacketEvent = "SendPacket"

// ETHClient go-ethereum methods used by Client.
type ETHClient interface {
	bind.ContractFilterer
	bind.ContractBackend

	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
}

// Client implements chains.Client for EVM chains.
type Client struct {
	chainID string

	eth ETHClient

	routerAddress  common.Address
	routerFilterer *ics26router.ContractFilterer
	routerABI      *abi.ABI
	router         *ics26router.Contract

	logger *slog.Logger
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

	routerFilterer, err := ics26router.NewContractFilterer(routerAddress, eth)
	if err != nil {
		return nil, errors.Wrap(err, "creating ics26 router filterer")
	}

	routerABI, err := ics26router.ContractMetaData.GetAbi()
	if err != nil {
		return nil, errors.Wrap(err, "getting ics26 router abi")
	}

	router, err := ics26router.NewContract(routerAddress, eth)
	if err != nil {
		return nil, errors.Wrap(err, "creating ics26 router contract")
	}

	if _, ok := routerABI.Events[sendPacketEvent]; !ok {
		return nil, errors.Errorf("event %q not found in ics26 router abi", sendPacketEvent)
	}

	return &Client{
		chainID: chainID,
		eth:     eth,

		routerAddress:  routerAddress,
		routerFilterer: routerFilterer,
		routerABI:      routerABI,
		router:         router,

		logger: slog.With("module", "chains", "chainType", "evm", "chainID", chainID),
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

	receipt, err := c.eth.TransactionReceipt(ctx, txHash)
	if err != nil {
		return nil, errors.Wrapf(err, "getting receipt for tx %s on chain %s", txHash, c.chainID)
	}

	var packets []channeltypesv2.Packet

	for _, log := range receipt.Logs {
		switch {
		case log == nil, len(log.Topics) == 0:
			continue
		case log.Address != c.routerAddress:
			continue
		case log.Topics[0] != c.routerABI.Events[sendPacketEvent].ID:
			continue
		}

		sendPacket, errParse := c.routerFilterer.ParseSendPacket(*log)
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
