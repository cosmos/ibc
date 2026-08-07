// Package chains defines chain-agnostic clients for reading chain state.
package chains

import (
	"context"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains/evm"
	"github.com/cosmos/ibc/link/internal/config"

	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// Client provides chain state queries.
type Client interface {
	ChainID() string

	// TxPacketEvents reads every packet event emitted by txHash.
	TxPacketEvents(ctx context.Context, txHash []byte) ([]v2.PacketEvent, error)

	// IsPacketReceived reports whether a packet receipt exists on the
	// destination chain.
	IsPacketReceived(ctx context.Context, destClientID string, sequence uint64) (bool, error)

	// IsPacketCommitted reports whether the packet commitment still exists on
	// the source chain; the commitment is deleted once the packet is acked or
	// timed out.
	IsPacketCommitted(ctx context.Context, sourceClientID string, sequence uint64) (bool, error)

	FindRecvTx(ctx context.Context, destClientID string, sequence uint64) (*v2.Tx, error)
	FindAckTx(ctx context.Context, sourceClientID string, sequence uint64) (*v2.Tx, error)
	FindTimeoutTx(ctx context.Context, sourceClientID string, sequence uint64) (*v2.Tx, error)

	// PacketWriteAckStatus extracts the write acknowledgement status for a
	// packet from its recv transaction.
	PacketWriteAckStatus(
		ctx context.Context,
		recvTxHash string,
		sequence uint64,
		sourceClientID string,
		destClientID string,
	) (v2.WriteAckStatus, error)

	// IsTxFinalized reports whether a transaction's block is finalized. A nil
	// finalityOffset uses the chain's native finality (the tx's block is at or
	// below the finalized head); otherwise the tx is finalized once
	// finalityOffset blocks are mined on top of its block.
	IsTxFinalized(ctx context.Context, txHash string, finalityOffset *uint64) (bool, error)

	// IsTimestampFinalized reports whether the chain has finalized a block at
	// or after the timestamp. A nil finalityOffset compares against the
	// finalized head; otherwise against the block finalityOffset blocks behind
	// the latest.
	IsTimestampFinalized(ctx context.Context, timestamp time.Time, finalityOffset *uint64) (bool, error)

	// WaitForChain blocks until the chain's latest block time catches up to
	// the current time.
	WaitForChain(ctx context.Context) error

	// GetBlockHeader returns the header at height, used by the attestor to
	// read block timestamps for state attestations.
	GetBlockHeader(ctx context.Context, height uint64) (v2.BlockHeader, error)

	// GetCommitment reads the IBC commitment stored at pathHash at height,
	// used by the attestor to independently verify packet/ack/receipt claims.
	GetCommitment(ctx context.Context, height uint64, pathHash [32]byte) ([32]byte, error)

	// GetCounterparty returns the client ID this chain's router has
	// registered as clientID's on-chain counterparty.
	GetCounterparty(ctx context.Context, clientID string) (string, error)

	// GetAttestationSet returns clientID's on-chain attestor addresses and
	// minimum required signature count.
	GetAttestationSet(ctx context.Context, clientID string) (addresses []string, minRequiredSigs uint8, err error)
}

var _ Client = (*evm.Client)(nil)

// ClientSet holds the chain clients for all configured chains.
type ClientSet struct {
	clients map[string]Client
}

func NewClientSet(clients map[string]Client) *ClientSet {
	if clients == nil {
		clients = make(map[string]Client)
	}

	return &ClientSet{clients: clients}
}

func NewClientSetFromConfig(cfg config.Config) (*ClientSet, error) {
	clients := make(map[string]Client, len(cfg.Chains))

	for _, chain := range cfg.Chains {
		if chain.Type() != config.ChainTypeEVM {
			return nil, errors.Errorf("unsupported chain type for chain %q", chain.ChainID)
		}

		client, err := evm.New(chain.ChainID, chain.EVM.RPC, chain.EVM.ICS26Router)
		if err != nil {
			return nil, errors.Wrapf(err, "creating evm client for chain %q", chain.ChainID)
		}

		clients[chain.ChainID] = client
	}

	return NewClientSet(clients), nil
}

func (s *ClientSet) Get(chainID string) (Client, bool) {
	client, ok := s.clients[chainID]
	return client, ok
}
