package attestor

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/service/signer"

	evm "github.com/cosmos/ibc/link/internal/service/attestor/evm"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// LocalAttestor provides attestation data from the local process.
// Right now we support only EVM attestations. Then porting Cosmos/Solana attestors, we'd need to refactor
// LocalAttestor into LocalEVMAttestor, LocalCosmosAttestor, LocalSolanaAttestor, etc.
type LocalAttestor struct {
	chainID        string
	name           string
	finalityOffset uint

	client chains.Client
	signer signer.Signer

	logger *slog.Logger
}

var _ Attestor = &LocalAttestor{}

func NewLocal(cfg config.AttestationConfig, client chains.Client, backingSigner signer.Signer) (*LocalAttestor, error) {
	switch {
	case cfg.ChainID == "":
		return nil, fmt.Errorf("chainID required")
	case cfg.Name == "":
		return nil, fmt.Errorf("name required")
	case client == nil:
		return nil, fmt.Errorf("client required")
	case client.ChainID() != cfg.ChainID:
		return nil, fmt.Errorf("client chainID mismatch: got %s, want %s", client.ChainID(), cfg.ChainID)
	case backingSigner == nil:
		return nil, fmt.Errorf("signer required")
	case backingSigner.Type() != signer.ECDSA:
		return nil, fmt.Errorf("ECDSA signer required, got %s", backingSigner.Type())
	}

	fqn := attestorFQN("local", cfg.ChainID, cfg.Name)
	logger := slog.With("module", "attestor", "name", fqn)

	return &LocalAttestor{
		chainID:        cfg.ChainID,
		name:           cfg.Name,
		finalityOffset: cfg.FinalityOffset,

		client: client,
		signer: backingSigner,

		logger: logger,
	}, nil
}

// LatestHeight returns the highest block number that is *attestable*.
// If finality offset is zero, returns the "finalized" block.
// Otherwise, returns the "latest" block minus the offset.
func (a *LocalAttestor) LatestHeight(ctx context.Context) (uint64, error) {
	actualHeight := uint64(v2.FinalizedBlock)
	if a.finalityOffset > 0 {
		actualHeight = v2.LatestBlock
	}

	// TODO: cache last N headers to avoid extra RPC calls
	header, err := a.client.GetBlockHeader(ctx, actualHeight)
	if err != nil {
		return 0, err
	}

	actualHeight = header.Height
	offset := uint64(a.finalityOffset)
	if offset >= actualHeight {
		return 0, nil
	}

	return actualHeight - offset, nil
}

func (a *LocalAttestor) StateAttestation(ctx context.Context, height uint64) (Attestation, error) {
	latestHeight, err := a.LatestHeight(ctx)
	switch {
	case err != nil:
		return Attestation{}, errors.Wrapf(err, "get latest attestable height")
	case height > latestHeight:
		return Attestation{}, errors.Wrapf(ErrNotFinalized, "latest %d, requested %d", latestHeight, height)
	}

	header, err := a.client.GetBlockHeader(ctx, height)
	if err != nil {
		return Attestation{}, errors.Wrapf(err, "get header at height %d", height)
	}

	attestedData, err := evm.EncodeStateAttestation(height, uint64(header.Timestamp.Unix()))
	if err != nil {
		return Attestation{}, err
	}

	signature, err := evm.SignABI(ctx, a.signer, evm.TagStateAttestation, attestedData)
	if err != nil {
		return Attestation{}, fmt.Errorf("sign state attestation: %w", err)
	}

	return Attestation{
		Height:       height,
		Timestamp:    &header.Timestamp,
		AttestedData: attestedData,
		Signature:    signature,
	}, nil
}

func (a *LocalAttestor) PacketAttestation(ctx context.Context, req PacketAttestationRequest) (Attestation, error) {
	if count := len(req.Packets); count == 0 || count > MaxPacketsPerAttestation {
		return Attestation{}, errors.Wrapf(
			ErrInvalidInput,
			"packet count %d is outside allowed range 1..%d",
			count,
			MaxPacketsPerAttestation,
		)
	}

	packets := make([]v2.Packet, len(req.Packets))
	for i, encoded := range req.Packets {
		packet, err := evm.DecodePacket(encoded)
		if err != nil {
			return Attestation{}, errors.Wrapf(ErrInvalidInput, "decode packet %d: %s", i, err)
		}
		packets[i] = packet
	}

	latestHeight, err := a.LatestHeight(ctx)
	switch {
	case err != nil:
		return Attestation{}, errors.Wrapf(err, "get latest attestable height")
	case req.Height > latestHeight:
		return Attestation{}, errors.Wrapf(ErrNotFinalized, "latest %d, requested %d", latestHeight, req.Height)
	}

	_ = packets // used when implementing commitment retrieval

	// TODO(5): Add an EVM client method to read getCommitment(pathHash) at exactly req.Height.
	//   a.client.GetCommitment(ctx, req.Height, <pathHash>)
	// TODO(6): For each packet, derive its raw ICS-24 path and keccak256 path hash.
	// TODO(7): Enforce type semantics: packet must exist and equal the recomputed commitment,
	// acknowledgement must exist, and receipt must be absent; fail the batch atomically.
	// TODO(8): Preserve request order and build PacketCompact{path, commitment} entries.
	// TODO(9): Add EVM ABI encoding for PacketAttestation{height, packets}.
	// TODO(10): Generalize SignABI to use the packet domain tag 0x02, then sign the encoded data.
	// TODO(11): Return height, nil timestamp, encoded attested data, and the normalized signature.
	return Attestation{
		Height: req.Height,
	}, nil
}

// name and alias are identical for local attestors
func (a *LocalAttestor) Name() string    { return a.name }
func (a *LocalAttestor) Alias() string   { return a.name }
func (a *LocalAttestor) ChainID() string { return a.chainID }
func (a *LocalAttestor) IsLocal() bool   { return true }

func attestorFQN(connection, chainID, name string) string {
	return fmt.Sprintf("%s-%s-%s", chainID, connection, name)
}
