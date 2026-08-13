// SPDX-License-Identifier: Apache-2.0

// Package relayer implements the relayer business logic.
package relayer

import (
	"cmp"
	"context"
	"encoding/hex"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/relay/dispatch"
	"github.com/cosmos/ibc/link/internal/store"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// Service represents relayer business logic.
type Service struct {
	logger *slog.Logger
	cfg    config.Config
	store  Store
	chains ChainClients

	dispatcher *dispatch.RelayDispatcher
}

// ChainClients resolves chain clients by chain id.
type ChainClients interface {
	Get(chainID string) (chains.Client, bool)
}

// Store queries used by the relayer gRPC handlers.
type Store interface {
	GetRelayRequest(ctx context.Context, chainID string, txHash string) (*store.RelayRequest, error)
	ListPacketsBySourceTx(ctx context.Context, chainID string, txHash string) ([]store.Packet, error)
	Transact(ctx context.Context, call func(store.Repository) error) error
}

// Relay errors
var (
	ErrInvalidInput       = errors.New("invalid input")
	ErrFailedPrecondition = errors.New("failed precondition")
	ErrNotFound           = errors.New("not found")
)

// PacketState relay state.
type PacketState int

// Packet states
const (
	StateUnspecified PacketState = iota
	StateNotSelected
	StatePending
	StateSucceeded
	// StateTimedOut completed with a timeout refund on the source chain.
	StateTimedOut
	// StateRejected completed with an error acknowledgement on the source chain.
	StateRejected
	// StateRelayFailed cannot be processed because of a permanent error.
	StateRelayFailed
)

// TxInfo a transaction on a chain.
type TxInfo struct {
	TxHash  string
	ChainID string
}

// PacketStatus the relay status of a packet.
type PacketStatus struct {
	State          PacketState
	SequenceNumber uint64
	SourceClientID string
	SendTx         TxInfo
	RecvTx         *TxInfo
	AckTx          *TxInfo
	TimeoutTx      *TxInfo
}

// PacketSelector identifies a packet in a source transaction.
type PacketSelector struct {
	SourceClientID string
	SequenceNumber uint64
}

// SelectionMode controls which packets a relay request selects for relay.
type SelectionMode int

// Selection modes.
const (
	SelectionUnspecified SelectionMode = iota
	SelectionAll
	SelectionExplicit
)

// RelayRequest is an explicit packet selection for one source transaction.
type RelayRequest struct {
	ChainID   string
	TxHash    string
	Selection SelectionMode
	Packets   []PacketSelector
}

// New Service constructor. dispatcher may be nil for a service that only
// serves the gRPC API, with no background dispatch loop.
func New(cfg config.Config, st Store, clients ChainClients, dispatcher *dispatch.RelayDispatcher) *Service {
	return &Service{
		logger:     slog.With("service", "relayer"),
		cfg:        cfg,
		store:      st,
		chains:     clients,
		dispatcher: dispatcher,
	}
}

// Start begins the background relay dispatch loop. A no-op if dispatcher is nil.
func (s *Service) Start() error {
	if s.dispatcher == nil {
		return nil
	}

	return s.dispatcher.Start()
}

// Stop cancels the background relay dispatch loop and blocks until it has exited.
// A no-op if dispatcher is nil.
func (s *Service) Stop() error {
	if s.dispatcher == nil {
		return nil
	}

	return s.dispatcher.Stop()
}

func (s *Service) Relay(ctx context.Context, request RelayRequest) error {
	switch {
	case request.Selection == SelectionUnspecified:
		return errors.Wrap(ErrInvalidInput, "packet selection is required")
	case request.Selection == SelectionAll && len(request.Packets) > 0:
		return errors.Wrap(ErrInvalidInput, "packet selectors require explicit selection")
	case request.Selection == SelectionExplicit && len(request.Packets) == 0:
		return errors.Wrap(ErrInvalidInput, "selected packet list is empty")
	case request.Selection != SelectionAll && request.Selection != SelectionExplicit:
		return errors.Wrap(ErrInvalidInput, "invalid packet selection")
	}

	txHash, err := s.validateRelayArgs(request.ChainID, request.TxHash)
	if err != nil {
		return err
	}
	chainID := request.ChainID

	client, ok := s.chains.Get(chainID)
	if !ok {
		return errors.Wrapf(ErrNotFound, "client for chain %q", chainID)
	}

	hashBytes, err := hex.DecodeString(strings.TrimPrefix(txHash, "0x"))
	if err != nil {
		return errors.Wrapf(ErrInvalidInput, "decoding txHash %q", txHash)
	}

	events, err := client.TxPacketEvents(ctx, hashBytes)
	if err != nil {
		if errors.Is(err, ethereum.NotFound) {
			return errors.Wrapf(ErrNotFound, "no packets found: transaction %s on chain %s", txHash, chainID)
		}

		return errors.Wrap(err, "extracting packet events")
	}

	observedPackets, relayablePackets := s.packetsFromEvents(chainID, txHash, events)

	selected := slices.Collect(maps.Keys(relayablePackets))
	if request.Selection == SelectionExplicit {
		selected = request.Packets
		for _, selector := range selected {
			if _, ok := observedPackets[selector]; !ok {
				return errors.Wrapf(
					ErrInvalidInput,
					"packet %s/%d is absent from transaction",
					selector.SourceClientID,
					selector.SequenceNumber,
				)
			}
			if _, ok := relayablePackets[selector]; !ok {
				return errors.Wrapf(
					ErrFailedPrecondition,
					"packet %s/%d is not configured for relaying",
					selector.SourceClientID,
					selector.SequenceNumber,
				)
			}
		}
	}

	for _, selector := range selected {
		packet := relayablePackets[selector]
		packet.Status = store.RelayStatusPending
		relayablePackets[selector] = packet
	}

	err = s.store.Transact(ctx, func(repo store.Repository) error {
		if errCreate := repo.CreateRelayRequest(ctx, chainID, txHash); errCreate != nil {
			return errors.Wrap(errCreate, "creating relay request")
		}

		for _, selector := range sortedPacketSelectors(relayablePackets) {
			packet := relayablePackets[selector]
			if errUpsert := repo.UpsertPacket(ctx, packet); errUpsert != nil {
				return errors.Wrapf(errUpsert, "upserting packet %d", packet.PacketSequenceNumber)
			}
		}

		return nil
	})
	if err != nil {
		return errors.Wrap(err, "recording relay request")
	}

	s.logger.Info(
		"Recorded relay request",
		"chainID", chainID,
		"txHash", txHash,
		"packets", len(relayablePackets),
		"selected", len(selected),
	)

	return nil
}

// packetsFromEvents extracts the transaction's send packets. observed holds
// every send packet; relayable holds the subset this instance has a client and
// route configured for, as not-selected upsert inputs.
func (s *Service) packetsFromEvents(
	chainID string,
	txHash string,
	events []v2.PacketEvent,
) (observed map[PacketSelector]struct{}, relayable map[PacketSelector]store.UpsertPacket) {
	observed = make(map[PacketSelector]struct{})
	relayable = make(map[PacketSelector]store.UpsertPacket)

	for _, event := range events {
		if event.Kind != v2.KindSendPacket {
			continue
		}

		selector := PacketSelector{
			SourceClientID: event.Packet.SourceClient,
			SequenceNumber: event.Packet.Sequence,
		}
		observed[selector] = struct{}{}

		client, ok := s.cfg.Relayer.Client(chainID, event.Packet.SourceClient)
		if !ok {
			s.logger.Warn(
				"Skipping packet from unconfigured client",
				"chainID", chainID,
				"clientID", event.Packet.SourceClient,
				"sequence", event.Packet.Sequence,
			)

			continue
		}
		if _, routed := s.cfg.Relayer.RouteBySourceClient(client.Alias); !routed {
			s.logger.Warn(
				"Skipping packet without configured relay route",
				"chainID", chainID,
				"clientID", event.Packet.SourceClient,
				"sequence", event.Packet.Sequence,
			)

			continue
		}
		if event.Packet.DestinationClient != client.CounterpartyClientID {
			s.logger.Warn(
				"Skipping packet with unconfigured destination client",
				"chainID", chainID,
				"clientID", event.Packet.SourceClient,
				"destinationClientID", event.Packet.DestinationClient,
				"sequence", event.Packet.Sequence,
			)

			continue
		}

		relayable[selector] = store.UpsertPacket{
			Status:                    store.RelayStatusNotSelected,
			SourceChainID:             chainID,
			DestinationChainID:        client.CounterpartyChainID,
			SourceTxHash:              txHash,
			SourceTxTime:              event.BlockTime,
			PacketSequenceNumber:      event.Packet.Sequence,
			PacketSourceClientID:      event.Packet.SourceClient,
			PacketDestinationClientID: event.Packet.DestinationClient,
			PacketTimeoutTimestamp:    unixTime(event.Packet.TimeoutTimestamp),
		}
	}

	return observed, relayable
}

// sortedPacketSelectors fixes the upsert order so concurrent relay requests
// for the same transaction cannot deadlock on row locks.
func sortedPacketSelectors(packets map[PacketSelector]store.UpsertPacket) []PacketSelector {
	return slices.SortedFunc(maps.Keys(packets), func(a, b PacketSelector) int {
		return cmp.Or(cmp.Compare(a.SourceClientID, b.SourceClientID), cmp.Compare(a.SequenceNumber, b.SequenceNumber))
	})
}

func (s *Service) Status(ctx context.Context, chainID, txHash string) ([]PacketStatus, error) {
	txHash, err := s.validateRelayArgs(chainID, txHash)
	if err != nil {
		return nil, err
	}

	switch _, errGet := s.store.GetRelayRequest(ctx, chainID, txHash); {
	case errors.Is(errGet, store.ErrNotFound):
		return nil, errors.Wrap(ErrNotFound, "transaction not submitted to relayer")
	case errGet != nil:
		return nil, errors.Wrap(errGet, "getting relay request")
	}

	packets, err := s.store.ListPacketsBySourceTx(ctx, chainID, txHash)
	if err != nil {
		return nil, errors.Wrap(err, "listing packets")
	}

	statuses := make([]PacketStatus, len(packets))
	for i, packet := range packets {
		statuses[i] = PacketStatus{
			State:          mapPacketState(packet.Status),
			SequenceNumber: packet.PacketSequenceNumber,
			SourceClientID: packet.PacketSourceClientID,
			SendTx:         TxInfo{TxHash: packet.SourceTxHash, ChainID: packet.SourceChainID},
			RecvTx:         toTxInfo(packet.RecvTxHash, packet.DestinationChainID),
			AckTx:          toTxInfo(packet.AckTxHash, packet.SourceChainID),
			TimeoutTx:      toTxInfo(packet.TimeoutTxHash, packet.SourceChainID),
		}
	}

	return statuses, nil
}

// validateRelayArgs validates the tx hash for the chain's type and applies
// consistent casing so lookups are case-insensitive.
func (s *Service) validateRelayArgs(chainID, txHash string) (string, error) {
	switch {
	case chainID == "":
		return "", errors.Wrap(ErrInvalidInput, "chainID is required")
	case txHash == "":
		return "", errors.Wrap(ErrInvalidInput, "txHash is required")
	}

	chain, ok := s.cfg.Chain(chainID)
	if !ok {
		return "", errors.Wrapf(ErrInvalidInput, "unsupported chain %q", chainID)
	}

	switch chain.Type() {
	case config.ChainTypeEVM:
		var hash common.Hash
		if err := hash.UnmarshalText([]byte(txHash)); err != nil {
			return "", errors.Wrapf(ErrInvalidInput, "txHash %q is not a valid evm transaction hash", txHash)
		}

		return hash.Hex(), nil
	default:
		return "", errors.Wrapf(ErrInvalidInput, "unsupported chain type for chain %q", chainID)
	}
}

func mapPacketState(status store.RelayStatus) PacketState {
	switch status {
	case store.RelayStatusNotSelected:
		return StateNotSelected
	case store.RelayStatusCompleteWithAck:
		return StateSucceeded
	case store.RelayStatusCompleteWithTimeout:
		return StateTimedOut
	case store.RelayStatusCompleteWithWriteAckError:
		return StateRejected
	case store.RelayStatusFailed:
		return StateRelayFailed
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
