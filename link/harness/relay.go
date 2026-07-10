package harness

import (
	"context"
	"fmt"
	"slices"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

// Relay identifies one source transaction to submit to the relayer's manual relay endpoint. Field
// names mirror wire.RelayRequest, the request this becomes.
type Relay struct {
	SourceChainID string
	SourceTxHash  string
}

// Relay submits a manual relay request for one source transaction.
func (r *Session) Relay(ctx context.Context, in Relay) error {
	_, err := r.relay(ctx, in)
	return err
}

func (r *Session) relay(ctx context.Context, in Relay) (*wire.RelayResult, error) {
	req := wire.RelayRequest{SourceChainID: in.SourceChainID, SourceTxHash: in.SourceTxHash}
	return r.h.relayer.Relay(ctx, req)
}

// relayOutcome submits the manual relay request for one submitted action's source tx and cross-checks
// that the daemon matched the action's packet — a miss is a wire-contract violation.
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

// Relay requests manual relay for this IFT outcome and verifies the daemon matched this packet.
func (o *IFTOutcome) Relay(ctx context.Context) error {
	return o.plan.run.relayOutcome(ctx, o.action.Source, o.action.ID(), o.action.SourceTxHash)
}

// Relay requests manual relay for this GMP outcome and verifies the daemon matched this packet.
func (o *GMPOutcome) Relay(ctx context.Context) error {
	return o.plan.run.relayOutcome(ctx, o.action.Source, o.action.ID(), o.action.SourceTxHash)
}
