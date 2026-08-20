// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"database/sql"
	"log/slog"
	"slices"
	"strings"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/config"
)

// Store a unified, database-agnostic API for persistence.
type Store interface {
	Repository
	Migrator

	// Transact runs call in a transaction; call's Repository is bound to it and rolled back on error.
	Transact(ctx context.Context, call func(repo Repository) error) error

	Ping(ctx context.Context) error
	Close() error
}

// Repository represents database CRUD operations.
type Repository interface {
	GetRelayRequest(ctx context.Context, chainID string, txHash string) (*RelayRequest, error)

	// CreateRelayRequest records a relay request; duplicates are a noop.
	CreateRelayRequest(ctx context.Context, chainID string, txHash string) error

	// UpsertPacket records a packet. A new packet is inserted; an existing
	// NOT_SELECTED packet is refreshed with the input's metadata and status,
	// but only when the input status is NOT_SELECTED or PENDING, the latter
	// selecting it for relay. Existing packets in any other status are never
	// modified.
	UpsertPacket(ctx context.Context, input UpsertPacket) error

	ListPacketsBySourceTx(ctx context.Context, chainID string, txHash string) ([]Packet, error)

	ListPackets(ctx context.Context, filter PacketFilter, page Page) ([]Packet, error)

	// ListDispatchablePackets returns selected packets that have not reached a terminal status.
	ListDispatchablePackets(ctx context.Context) ([]Packet, error)

	UpdatePacketStatus(ctx context.Context, key PacketKey, status RelayStatus) error

	UpdatePacketRecvTx(ctx context.Context, key PacketKey, tx PacketTx) error
	ClearPacketRecvTx(ctx context.Context, key PacketKey) error

	UpdatePacketWriteAck(ctx context.Context, key PacketKey, ack WriteAck) error

	UpdatePacketAckTx(ctx context.Context, key PacketKey, tx PacketTx) error
	ClearPacketAckTx(ctx context.Context, key PacketKey) error

	UpdatePacketTimeoutTx(ctx context.Context, key PacketKey, tx PacketTx) error
	ClearPacketTimeoutTx(ctx context.Context, key PacketKey) error
}

// PacketKey uniquely identifies a packet.
type PacketKey struct {
	SourceChainID  string
	SourceClientID string
	Sequence       uint64
}

// PacketTx a relay transaction recorded on a packet.
type PacketTx struct {
	Hash           string
	Time           time.Time
	RelayerAddress string
}

// WriteAck the write acknowledgement observed for a packet.
type WriteAck struct {
	TxHash string
	TxTime time.Time
	Status WriteAckStatus
}

// Migrator abstracts schema migrations
type Migrator interface {
	MigrateUp() (int, error)
	MigrateDown() (int, error)
	MigrationStatus() ([]MigrationStatus, error)
}

// Repository errors
var (
	ErrNotFound = errors.New("not found")
)

// NewStore creates a new Store instance based on the database type.
func NewStore(ctx context.Context, cfg config.Config) (Store, error) {
	switch cfg.DB.Type {
	case config.DBTypeSQLite:
		return NewSqlite(cfg.DB.URL)
	case config.DBTypePostgres:
		return NewPostgres(ctx, cfg.DB.URL)
	default:
		return nil, errors.New("invalid database type")
	}
}

// ValidateConfigLive assumes the config.Config is valid,
// and checks if the database is reachable.
func ValidateConfigLive(cfg config.Config) error {
	// noop, don't create an empty sqlite db
	if cfg.DB.Type == config.DBTypeSQLite {
		return nil
	}

	// contains Ping()
	store, err := NewStore(context.Background(), cfg)
	if err != nil {
		return err
	}

	if errClose := store.Close(); errClose != nil {
		slog.Error("Failed to close database", "err", errClose)
	}

	return nil
}

// RelayRequest a request to relay a transaction's packets.
type RelayRequest struct {
	ID        int64
	ChainID   string
	TxHash    string
	CreatedAt time.Time
}

// RelayStatus the relay state of a packet.
type RelayStatus string

// Packet statuses. Declaring one registers it, so AllRelayStatuses cannot fall
// out of step with the set that exists.
var (
	RelayStatusNotSelected                = relayStatus("NOT_SELECTED")
	RelayStatusPending                    = relayStatus("PENDING")
	RelayStatusAwaitingSendFinality       = relayStatus("AWAITING_SEND_FINALITY")
	RelayStatusCheckRecvPacketDelivery    = relayStatus("CHECK_RECV_PACKET_DELIVERY")
	RelayStatusGetRecvPacket              = relayStatus("GET_RECV_PACKET")
	RelayStatusDeliverRecvPacket          = relayStatus("DELIVER_RECV_PACKET")
	RelayStatusWaitForWriteAck            = relayStatus("WAIT_FOR_WRITE_ACK")
	RelayStatusAwaitingWriteAckFinality   = relayStatus("AWAITING_WRITE_ACK_FINALITY")
	RelayStatusCheckAckPacketDelivery     = relayStatus("CHECK_ACK_PACKET_DELIVERY")
	RelayStatusGetAckPacket               = relayStatus("GET_ACK_PACKET")
	RelayStatusDeliverAckPacket           = relayStatus("DELIVER_ACK_PACKET")
	RelayStatusAwaitingTimeoutFinality    = relayStatus("AWAITING_TIMEOUT_FINALITY")
	RelayStatusCheckTimeoutPacketDelivery = relayStatus("CHECK_TIMEOUT_PACKET_DELIVERY")
	RelayStatusGetTimeoutPacket           = relayStatus("GET_TIMEOUT_PACKET")
	RelayStatusDeliverTimeoutPacket       = relayStatus("DELIVER_TIMEOUT_PACKET")
	RelayStatusCompleteWithAck            = relayStatus("COMPLETE_WITH_ACK")
	RelayStatusCompleteWithWriteAckError  = relayStatus("COMPLETE_WITH_WRITE_ACK_ERROR")
	RelayStatusCompleteWithTimeout        = relayStatus("COMPLETE_WITH_TIMEOUT")
	RelayStatusFailed                     = relayStatus("FAILED")
)

var allRelayStatuses []RelayStatus

func relayStatus(name string) RelayStatus {
	status := RelayStatus(name)
	allRelayStatuses = append(allRelayStatuses, status)

	return status
}

// AllRelayStatuses lets callers derive groupings rather than restate them.
func AllRelayStatuses() []RelayStatus {
	return slices.Clone(allRelayStatuses)
}

// WriteAckStatus the execution result carried by a write ack.
type WriteAckStatus string

// Write ack statuses
const (
	WriteAckStatusSuccess WriteAckStatus = "SUCCESS"
	WriteAckStatusError   WriteAckStatus = "ERROR"
	WriteAckStatusUnknown WriteAckStatus = "UNKNOWN"
)

// Packet a packet tracked through its relay lifecycle.
type Packet struct {
	ID        int64
	CreatedAt time.Time
	UpdatedAt time.Time

	Status RelayStatus

	SourceChainID      string
	DestinationChainID string
	SourceTxHash       string
	SourceTxTime       time.Time

	PacketSequenceNumber      uint64
	PacketSourceClientID      string
	PacketDestinationClientID string
	PacketTimeoutTimestamp    time.Time

	RecvTxHash           *string
	RecvTxTime           *time.Time
	RecvTxRelayerAddress *string

	WriteAckTxHash *string
	WriteAckTxTime *time.Time
	WriteAckStatus *WriteAckStatus

	AckTxHash           *string
	AckTxTime           *time.Time
	AckTxRelayerAddress *string

	TimeoutTxHash           *string
	TimeoutTxTime           *time.Time
	TimeoutTxRelayerAddress *string
}

// UpsertPacket the fields callers provide when recording a packet; the
// remaining Packet fields are database-assigned or set later in the lifecycle.
type UpsertPacket struct {
	Status                    RelayStatus
	SourceChainID             string
	DestinationChainID        string
	SourceTxHash              string
	SourceTxTime              time.Time
	PacketSequenceNumber      uint64
	PacketSourceClientID      string
	PacketDestinationClientID string
	PacketTimeoutTimestamp    time.Time
}

func (t UpsertPacket) Validate() error {
	switch {
	case t.Status == "":
		return errors.New("status is required")
	case t.Status != RelayStatusNotSelected && t.Status != RelayStatusPending:
		return errors.New("status must be NOT_SELECTED or PENDING")
	case t.SourceChainID == "":
		return errors.New("source chain id is required")
	case t.DestinationChainID == "":
		return errors.New("destination chain id is required")
	case t.SourceTxHash == "":
		return errors.New("source tx hash is required")
	case t.SourceTxTime.IsZero():
		return errors.New("source tx time is required")
	case t.PacketSequenceNumber == 0:
		return errors.New("packet sequence number is required")
	case t.PacketSourceClientID == "":
		return errors.New("packet source client id is required")
	case t.PacketDestinationClientID == "":
		return errors.New("packet destination client id is required")
	case t.PacketTimeoutTimestamp.IsZero():
		return errors.New("packet timeout timestamp is required")
	}

	return nil
}

// cast db-specific errors to repository errors
func errNormalize(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sql.ErrNoRows), errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	default:
		return err
	}
}

// PacketFilter narrows a ListPackets query. Every field is optional except
// Statuses, which is always applied -- pass every status for no filtering.
type PacketFilter struct {
	Statuses            []RelayStatus
	SourceChainID       *string
	DestinationChainID  *string
	SourceClientID      *string
	DestinationClientID *string
	SourceTxHash        *string
	SequenceNumber      *uint64
	CreatedFrom         *time.Time
	CreatedTo           *time.Time
}

// Page bounds a ListPackets result. Limit and Offset are applied as given;
// defaults, caps, and has-more probing are the caller's concern.
type Page struct {
	Limit  int64
	Offset int64
}

// statusList renders statuses for the query. Relay statuses contain no commas,
// so delimiting is unambiguous; an empty set matches nothing, as intended.
func (f PacketFilter) statusList() *string {
	statuses := make([]string, len(f.Statuses))
	for i, status := range f.Statuses {
		statuses[i] = string(status)
	}

	list := strings.Join(statuses, ",")

	return &list
}

// sequenceFilter converts to the signed type the generated params use.
func (f PacketFilter) sequenceFilter() *int64 {
	if f.SequenceNumber == nil {
		return nil
	}

	sequence := int64(*f.SequenceNumber) //nolint:gosec // IBC sequences fit in int64

	return &sequence
}
