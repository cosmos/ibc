// SPDX-License-Identifier: Apache-2.0

package attestor

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	hostv2 "github.com/cosmos/ibc-go/v11/modules/core/24-host/v2"
	attestordomain "github.com/cosmos/ibc/link/attestor"
	attestorevm "github.com/cosmos/ibc/link/attestor/evm"
	attestorevmibc "github.com/cosmos/ibc/link/attestor/evm/ibc"
	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/service/signer"
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
		return 0, errors.Wrapf(
			attestordomain.ErrNotFinalized,
			"latest height %d does not exceed finality offset %d",
			actualHeight,
			offset,
		)
	}

	return actualHeight - offset, nil
}

func (a *LocalAttestor) StateAttestation(ctx context.Context, height uint64) (attestordomain.Attestation, error) {
	if err := attestordomain.ValidateHeight(height); err != nil {
		return attestordomain.Attestation{}, errors.Wrapf(attestordomain.ErrInvalidInput, "%s", err)
	}

	latestHeight, err := a.LatestHeight(ctx)
	switch {
	case err != nil:
		return attestordomain.Attestation{}, errors.Wrapf(err, "get latest attestable height")
	case height > latestHeight:
		return attestordomain.Attestation{}, errors.Wrapf(
			attestordomain.ErrNotFinalized,
			"latest %d, requested %d",
			latestHeight,
			height,
		)
	}

	header, err := a.client.GetBlockHeader(ctx, height)
	if err != nil {
		return attestordomain.Attestation{}, errors.Wrapf(err, "get header at height %d", height)
	}

	attestedData, err := attestorevm.EncodeStateAttestation(height, uint64(header.Timestamp.Unix()))
	if err != nil {
		return attestordomain.Attestation{}, err
	}

	signature, err := attestorevm.SignABI(ctx, a.signer, attestorevm.TagStateAttestation, attestedData)
	if err != nil {
		return attestordomain.Attestation{}, fmt.Errorf("sign state attestation: %w", err)
	}

	return attestordomain.Attestation{
		Height:       height,
		Timestamp:    &header.Timestamp,
		AttestedData: attestedData,
		Signature:    signature,
	}, nil
}

func (a *LocalAttestor) PacketAttestation(
	ctx context.Context,
	req attestordomain.PacketAttestationRequest,
) (attestordomain.Attestation, error) {
	// 1. validate input
	if err := req.Validate(); err != nil {
		return attestordomain.Attestation{}, errors.Wrapf(attestordomain.ErrInvalidInput, "%s", err)
	}

	// 2. decode packets
	packets := make([]channeltypesv2.Packet, len(req.Packets))
	for i, encoded := range req.Packets {
		packet, err := attestorevmibc.DecodePacket(encoded)
		if err != nil {
			return attestordomain.Attestation{}, errors.Wrapf(
				attestordomain.ErrInvalidInput,
				"decode packet %d: %s",
				i,
				err,
			)
		}
		packets[i] = packet
	}

	// 3. ensure height is in within attestable range
	latestHeight, err := a.LatestHeight(ctx)
	switch {
	case err != nil:
		return attestordomain.Attestation{}, errors.Wrapf(err, "get latest attestable height")
	case req.Height > latestHeight:
		return attestordomain.Attestation{}, errors.Wrapf(
			attestordomain.ErrNotFinalized,
			"latest %d, requested %d",
			latestHeight,
			req.Height,
		)
	}

	// 4. convert packets to their "compact" form
	compacts := make([]attestorevm.PacketCompact, len(packets))
	for i, packet := range packets {
		compact, compactErr := a.packetCompact(ctx, req.Height, packet, req.CommitmentType)
		if compactErr != nil {
			return attestordomain.Attestation{}, errors.Wrapf(compactErr, "packet %d", i)
		}
		compacts[i] = compact
	}

	// 5. encode & sign the attested data
	attestedData, err := attestorevm.EncodePacketAttestation(req.Height, compacts)
	if err != nil {
		return attestordomain.Attestation{}, err
	}

	signature, err := attestorevm.SignABI(ctx, a.signer, attestorevm.TagPacketAttestation, attestedData)
	if err != nil {
		return attestordomain.Attestation{}, fmt.Errorf("sign packet attestation: %w", err)
	}

	return attestordomain.Attestation{
		Height:       req.Height,
		Timestamp:    nil,
		AttestedData: attestedData,
		Signature:    signature,
	}, nil
}

func (a *LocalAttestor) packetCompact(
	ctx context.Context,
	height uint64,
	packet channeltypesv2.Packet,
	commitmentType attestordomain.CommitmentType,
) (attestorevm.PacketCompact, error) {
	var path []byte
	switch commitmentType {
	case attestordomain.CommitmentTypePacket:
		path = hostv2.PacketCommitmentKey(packet.SourceClient, packet.Sequence)
	case attestordomain.CommitmentTypeAck:
		path = hostv2.PacketAcknowledgementKey(packet.DestinationClient, packet.Sequence)
	case attestordomain.CommitmentTypeReceipt:
		path = hostv2.PacketReceiptKey(packet.DestinationClient, packet.Sequence)
	default:
		return attestorevm.PacketCompact{}, errors.Wrapf(
			attestordomain.ErrInvalidInput,
			"unsupported commitment type %d",
			commitmentType,
		)
	}

	pathHash := [32]byte(crypto.Keccak256Hash(path))
	commitment, err := a.client.GetCommitment(ctx, height, pathHash)
	if err != nil {
		return attestorevm.PacketCompact{}, errors.Wrapf(err, "get commitment at height %d", height)
	}

	switch commitmentType {
	case attestordomain.CommitmentTypePacket:
		if commitment == ([32]byte{}) {
			return attestorevm.PacketCompact{}, errors.Wrapf(
				attestordomain.ErrCommitmentNotFound,
				"packet commitment for client %q sequence %d",
				packet.SourceClient,
				packet.Sequence,
			)
		}
		if expected := [32]byte(channeltypesv2.CommitPacket(packet)); commitment != expected {
			return attestorevm.PacketCompact{}, errors.Wrapf(
				attestordomain.ErrInvalidInput,
				"packet commitment mismatch: got %x, expected %x",
				commitment,
				expected,
			)
		}
	case attestordomain.CommitmentTypeAck:
		if commitment == ([32]byte{}) {
			return attestorevm.PacketCompact{}, errors.Wrapf(
				attestordomain.ErrCommitmentNotFound,
				"acknowledgement commitment for client %q sequence %d",
				packet.DestinationClient,
				packet.Sequence,
			)
		}
	case attestordomain.CommitmentTypeReceipt:
		if commitment != ([32]byte{}) {
			return attestorevm.PacketCompact{}, errors.Wrapf(
				attestordomain.ErrReceiptExists,
				"client %q sequence %d",
				packet.DestinationClient,
				packet.Sequence,
			)
		}
	}

	return attestorevm.PacketCompact{
		Path:       pathHash,
		Commitment: commitment,
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
