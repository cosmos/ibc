package store

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/config"

	pgx "github.com/jackc/pgx/v5"
)

// Store a unified, database-agnostic API for persistence.
type Store interface {
	Repository
	Migrator

	// ExecTx runs fn in a transaction; fn's Repository is bound to it and rolled back on error.
	ExecTx(ctx context.Context, fn func(Repository) error) error

	Ping(ctx context.Context) error
	Close() error
}

// Repository represents database CRUD operations.
type Repository interface {
	GetRelayRequest(ctx context.Context, chainID string, txHash string) (*RelayRequest, error)

	// CreateRelayRequest records a relay request; duplicates are a noop.
	CreateRelayRequest(ctx context.Context, chainID string, txHash string) error

	// CreatePacket records a packet; duplicate packets are a noop.
	CreatePacket(ctx context.Context, input CreatePacket) error

	ListPacketsBySourceTx(ctx context.Context, chainID string, txHash string) ([]Packet, error)
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

// Packet statuses
const (
	RelayStatusPending                     RelayStatus = "PENDING"
	RelayStatusAwaitingSendFinality        RelayStatus = "AWAITING_SEND_FINALITY"
	RelayStatusCheckRecvPacketDelivery     RelayStatus = "CHECK_RECV_PACKET_DELIVERY"
	RelayStatusGetRecvPacket               RelayStatus = "GET_RECV_PACKET"
	RelayStatusDeliverRecvPacket           RelayStatus = "DELIVER_RECV_PACKET"
	RelayStatusWaitForWriteAck             RelayStatus = "WAIT_FOR_WRITE_ACK"
	RelayStatusAwaitingWriteAckFinality    RelayStatus = "AWAITING_WRITE_ACK_FINALITY"
	RelayStatusCheckAckPacketDelivery      RelayStatus = "CHECK_ACK_PACKET_DELIVERY"
	RelayStatusGetAckPacket                RelayStatus = "GET_ACK_PACKET"
	RelayStatusDeliverAckPacket            RelayStatus = "DELIVER_ACK_PACKET"
	RelayStatusAwaitingTimeoutFinality     RelayStatus = "AWAITING_TIMEOUT_FINALITY"
	RelayStatusCheckTimeoutPacketDelivery  RelayStatus = "CHECK_TIMEOUT_PACKET_DELIVERY"
	RelayStatusGetTimeoutPacket            RelayStatus = "GET_TIMEOUT_PACKET"
	RelayStatusDeliverTimeoutPacket        RelayStatus = "DELIVER_TIMEOUT_PACKET"
	RelayStatusCompleteWithAck             RelayStatus = "COMPLETE_WITH_ACK"
	RelayStatusCompleteWithWriteAckSuccess RelayStatus = "COMPLETE_WITH_WRITE_ACK_SUCCESS"
	RelayStatusCompleteWithWriteAckError   RelayStatus = "COMPLETE_WITH_WRITE_ACK_ERROR"
	RelayStatusCompleteWithTimeout         RelayStatus = "COMPLETE_WITH_TIMEOUT"
	RelayStatusFailed                      RelayStatus = "FAILED"
)

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

// CreatePacket the fields callers provide when recording a packet; the
// remaining Packet fields are database-assigned or set later in the lifecycle.
type CreatePacket struct {
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

func (t CreatePacket) Validate() error {
	switch {
	case t.Status == "":
		return errors.New("status is required")
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
