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

	// CreateTransfer records a transfer; duplicate packets are a noop.
	CreateTransfer(ctx context.Context, transfer Transfer) error

	ListTransfersBySourceTx(ctx context.Context, chainID string, txHash string) ([]Transfer, error)
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

// RelayRequest is a request to relay the packets produced by a transaction.
type RelayRequest struct {
	ID        int64
	ChainID   string
	TxHash    string
	CreatedAt time.Time
}

// TransferStatus the relay state of a transfer.
type TransferStatus string

// Transfer statuses
const (
	TransferStatusPending                     TransferStatus = "PENDING"
	TransferStatusAwaitingSendFinality        TransferStatus = "AWAITING_SEND_FINALITY"
	TransferStatusCheckRecvPacketDelivery     TransferStatus = "CHECK_RECV_PACKET_DELIVERY"
	TransferStatusGetRecvPacket               TransferStatus = "GET_RECV_PACKET"
	TransferStatusDeliverRecvPacket           TransferStatus = "DELIVER_RECV_PACKET"
	TransferStatusWaitForWriteAck             TransferStatus = "WAIT_FOR_WRITE_ACK"
	TransferStatusAwaitingWriteAckFinality    TransferStatus = "AWAITING_WRITE_ACK_FINALITY"
	TransferStatusCheckAckPacketDelivery      TransferStatus = "CHECK_ACK_PACKET_DELIVERY"
	TransferStatusGetAckPacket                TransferStatus = "GET_ACK_PACKET"
	TransferStatusDeliverAckPacket            TransferStatus = "DELIVER_ACK_PACKET"
	TransferStatusAwaitingTimeoutFinality     TransferStatus = "AWAITING_TIMEOUT_FINALITY"
	TransferStatusCheckTimeoutPacketDelivery  TransferStatus = "CHECK_TIMEOUT_PACKET_DELIVERY"
	TransferStatusGetTimeoutPacket            TransferStatus = "GET_TIMEOUT_PACKET"
	TransferStatusDeliverTimeoutPacket        TransferStatus = "DELIVER_TIMEOUT_PACKET"
	TransferStatusCompleteWithAck             TransferStatus = "COMPLETE_WITH_ACK"
	TransferStatusCompleteWithWriteAckSuccess TransferStatus = "COMPLETE_WITH_WRITE_ACK_SUCCESS"
	TransferStatusCompleteWithWriteAckError   TransferStatus = "COMPLETE_WITH_WRITE_ACK_ERROR"
	TransferStatusCompleteWithTimeout         TransferStatus = "COMPLETE_WITH_TIMEOUT"
	TransferStatusFailed                      TransferStatus = "FAILED"
)

// WriteAckStatus the on-chain execution result carried by a write acknowledgement.
type WriteAckStatus string

// Write ack statuses
const (
	WriteAckStatusSuccess WriteAckStatus = "SUCCESS"
	WriteAckStatusError   WriteAckStatus = "ERROR"
	WriteAckStatusUnknown WriteAckStatus = "UNKNOWN"
)

// Transfer a packet tracked through its relay lifecycle.
type Transfer struct {
	ID        int64
	CreatedAt time.Time
	UpdatedAt time.Time

	Status     TransferStatus
	StatusText *string

	SourceChainID         string
	DestinationChainID    string
	SourceTxHash          string
	SourceTxTime          time.Time
	SourceTxFinalizedTime *time.Time

	PacketSequenceNumber      uint64
	PacketSourceClientID      string
	PacketDestinationClientID string
	PacketTimeoutTimestamp    time.Time

	RecvTxHash           *string
	RecvTxTime           *time.Time
	RecvTxRelayerAddress *string

	WriteAckTxHash          *string
	WriteAckTxTime          *time.Time
	WriteAckTxFinalizedTime *time.Time
	WriteAckStatus          *WriteAckStatus

	AckTxHash           *string
	AckTxTime           *time.Time
	AckTxRelayerAddress *string

	TimeoutTxHash           *string
	TimeoutTxTime           *time.Time
	TimeoutTxRelayerAddress *string
}

func (t Transfer) Validate() error {
	switch {
	case t.SourceChainID == "":
		return errors.New("source chain id is required")
	case t.DestinationChainID == "":
		return errors.New("destination chain id is required")
	case t.SourceTxHash == "":
		return errors.New("source tx hash is required")
	case t.SourceTxTime.IsZero():
		return errors.New("source tx time is required")
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
