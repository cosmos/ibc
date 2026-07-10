package harness

import (
	"context"
	"fmt"
	"slices"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

type Relay struct {
	SourceChainID string
	SourceTxHash  string
}

func (r *Session) Relay(ctx context.Context, in Relay) error {
	_, err := r.relay(ctx, in)
	return err
}

func (r *Session) relay(ctx context.Context, in Relay) (*wire.RelayResult, error) {
	req := wire.RelayRequest{SourceChainID: in.SourceChainID, SourceTxHash: in.SourceTxHash}
	return r.h.relayer.Relay(ctx, req)
}

func (r *Session) relayOutcome(ctx context.Context, sourceChain, packetID, sourceTxHash string) error {
	res, err := r.relay(ctx, Relay{SourceChainID: sourceChain, SourceTxHash: sourceTxHash})
	if err != nil {
		return err
	}
	if !slices.Contains(res.PacketIDs, packetID) {
		return fmt.Errorf("harness: relay result for packet %s did not include it: %v", packetID, res.PacketIDs)
	}
	return nil
}

func (o *IFTOutcome) Relay(ctx context.Context) error {
	return o.plan.run.relayOutcome(ctx, o.action.Source, o.action.ID(), o.action.SourceTxHash)
}

func (o *GMPOutcome) Relay(ctx context.Context) error {
	return o.plan.run.relayOutcome(ctx, o.action.Source, o.action.ID(), o.action.SourceTxHash)
}
