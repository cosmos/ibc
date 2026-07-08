// Package relayer implements the relayer business logic.
package relayer

import (
	"context"
	"encoding/hex"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/store"
)

// Service represents relayer business logic.
type Service struct {
	logger *slog.Logger
	cfg    config.RelayerConfig
	store  Store
	chains ChainClientManager
}

// ChainClientManager resolves chain clients by chain id.
type ChainClientManager interface {
	GetClient(ctx context.Context, chainID string) (chains.Client, error)
}

// Store narrows store.Store to what the relayer needs.
type Store interface {
	GetRelayRequest(ctx context.Context, chainID string, txHash string) (*store.RelayRequest, error)
	ListTransfersBySourceTx(ctx context.Context, chainID string, txHash string) ([]store.Transfer, error)
	ExecTx(ctx context.Context, fn func(store.Repository) error) error
}

// Relay errors
var (
	ErrInvalidInput = errors.New("invalid input")
	ErrNotFound     = errors.New("not found")
)

// TransferState coarse-grained relay state exposed via the API.
type TransferState int

// Transfer states
const (
	StateUnknown TransferState = iota
	StatePending
	StateComplete
	StateFailed
)

// TxInfo identifies a transaction on a chain.
type TxInfo struct {
	TxHash  string
	ChainID string
}

// PacketStatus the relay status of a single packet.
type PacketStatus struct {
	State          TransferState
	SequenceNumber uint64
	SourceClientID string
	SendTx         TxInfo
	RecvTx         *TxInfo
	AckTx          *TxInfo
	TimeoutTx      *TxInfo
}

// New Service constructor.
func New(cfg config.RelayerConfig, st Store, clientManager ChainClientManager) *Service {
	return &Service{
		logger: slog.With("service", "relayer"),
		cfg:    cfg,
		store:  st,
		chains: clientManager,
	}
}

// Relay records a request to relay the packets produced by the given transaction
// and tracks a transfer for every packet it sent. The request and its transfers
// are recorded atomically. Repeated submissions of the same transaction are a noop.
func (s *Service) Relay(ctx context.Context, chainID, txHash string) error {
	txHash, err := validateRelayArgs(chainID, txHash)
	if err != nil {
		return err
	}

	client, err := s.chains.GetClient(ctx, chainID)
	if err != nil {
		return errors.Wrapf(ErrInvalidInput, "unsupported chain %q", chainID)
	}

	hashBytes, err := hex.DecodeString(strings.TrimPrefix(txHash, "0x"))
	if err != nil {
		return errors.Wrapf(ErrInvalidInput, "decoding txHash %q", txHash)
	}

	events, err := client.TxPacketEvents(ctx, [][]byte{hashBytes})
	if err != nil {
		return errors.Wrap(err, "extracting packet events")
	}

	transfers := s.transfersFromEvents(chainID, txHash, events)

	err = s.store.ExecTx(ctx, func(repo store.Repository) error {
		if errCreate := repo.CreateRelayRequest(ctx, chainID, txHash); errCreate != nil {
			return errors.Wrap(errCreate, "creating relay request")
		}

		for _, transfer := range transfers {
			if errCreate := repo.CreateTransfer(ctx, transfer); errCreate != nil {
				return errors.Wrapf(errCreate, "creating transfer for packet %d", transfer.PacketSequenceNumber)
			}
		}

		return nil
	})
	if err != nil {
		return errors.Wrap(err, "recording relay request")
	}

	s.logger.Info("Recorded relay request", "chainID", chainID, "txHash", txHash, "packets", len(transfers))

	return nil
}

// transfersFromEvents converts send packet events into transfers, skipping
// packets from clients that are not configured.
func (s *Service) transfersFromEvents(chainID, txHash string, events []chains.PacketEvent) []store.Transfer {
	var transfers []store.Transfer

	for _, event := range events {
		if event.Kind != chains.KindSendPacket {
			continue
		}

		destinationChainID, ok := s.cfg.CounterpartyChainID(chainID, event.Packet.SourceClient)
		if !ok {
			s.logger.Warn(
				"Skipping packet from unconfigured client",
				"chainID", chainID,
				"clientID", event.Packet.SourceClient,
				"sequence", event.Packet.Sequence,
			)

			continue
		}

		transfers = append(transfers, store.Transfer{
			SourceChainID:             chainID,
			DestinationChainID:        destinationChainID,
			SourceTxHash:              txHash,
			SourceTxTime:              event.BlockTime,
			PacketSequenceNumber:      event.Packet.Sequence,
			PacketSourceClientID:      event.Packet.SourceClient,
			PacketDestinationClientID: event.Packet.DestClient,
			PacketTimeoutTimestamp:    unixTime(event.Packet.TimeoutTimestamp),
		})
	}

	return transfers
}

// Status returns the per-packet relay status for a previously relayed transaction.
func (s *Service) Status(ctx context.Context, chainID, txHash string) ([]PacketStatus, error) {
	txHash, err := validateRelayArgs(chainID, txHash)
	if err != nil {
		return nil, err
	}

	switch _, errGet := s.store.GetRelayRequest(ctx, chainID, txHash); {
	case errors.Is(errGet, store.ErrNotFound):
		return nil, errors.Wrap(ErrNotFound, "transaction not submitted to relayer")
	case errGet != nil:
		return nil, errors.Wrap(errGet, "getting relay request")
	}

	transfers, err := s.store.ListTransfersBySourceTx(ctx, chainID, txHash)
	if err != nil {
		return nil, errors.Wrap(err, "listing transfers")
	}

	statuses := make([]PacketStatus, len(transfers))
	for i, transfer := range transfers {
		statuses[i] = PacketStatus{
			State:          mapTransferState(transfer.Status),
			SequenceNumber: transfer.PacketSequenceNumber,
			SourceClientID: transfer.PacketSourceClientID,
			SendTx:         TxInfo{TxHash: transfer.SourceTxHash, ChainID: transfer.SourceChainID},
			RecvTx:         toTxInfo(transfer.RecvTxHash, transfer.DestinationChainID),
			AckTx:          toTxInfo(transfer.AckTxHash, transfer.SourceChainID),
			TimeoutTx:      toTxInfo(transfer.TimeoutTxHash, transfer.SourceChainID),
		}
	}

	return statuses, nil
}

var txHashPattern = regexp.MustCompile(`^0x[0-9a-f]{64}$`)

// validateRelayArgs validates the chain id and normalizes the tx hash to its
// canonical lowercase form so lookups are case-insensitive.
func validateRelayArgs(chainID, txHash string) (string, error) {
	txHash = strings.ToLower(txHash)

	switch {
	case chainID == "":
		return "", errors.Wrap(ErrInvalidInput, "chainID is required")
	case txHash == "":
		return "", errors.Wrap(ErrInvalidInput, "txHash is required")
	case !txHashPattern.MatchString(txHash):
		return "", errors.Wrapf(ErrInvalidInput, "txHash %q is not a 0x-prefixed 32-byte hex string", txHash)
	}

	return txHash, nil
}

func mapTransferState(status store.TransferStatus) TransferState {
	switch status {
	case store.TransferStatusCompleteWithAck,
		store.TransferStatusCompleteWithTimeout,
		store.TransferStatusCompleteWithWriteAckSuccess,
		store.TransferStatusCompleteWithWriteAckError:
		return StateComplete
	case store.TransferStatusFailed:
		return StateFailed
	default:
		return StatePending
	}
}

func toTxInfo(txHash *string, chainID string) *TxInfo {
	if txHash == nil {
		return nil
	}

	return &TxInfo{TxHash: *txHash, ChainID: chainID}
}

func unixTime(seconds uint64) time.Time {
	return time.Unix(int64(seconds), 0).UTC() //nolint:gosec // timeout timestamps fit in int64
}
