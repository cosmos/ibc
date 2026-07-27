// Package relayercmd owns the relayer command's CLI and transport contract.
package relayercmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Handler implements a relayer subcommand.
type Handler func(*cobra.Command, []string) error

// NewCommand constructs the relayer command with its behavior injected by the executable.
func NewCommand(handler Handler) *cobra.Command {
	cmd := &cobra.Command{Use: "relayer", Short: "Relayer commands"}
	run := &cobra.Command{
		Use:          "run",
		Short:        "Run the relayer",
		SilenceUsage: true,
		RunE:         handler,
	}
	cmd.AddCommand(run)
	return cmd
}

// ReadinessEvent identifies the first relayer stdout event.
const ReadinessEvent = "ready"

// Readiness is the first stdout line from relayer run.
type Readiness struct {
	Event             string          `json:"event"`
	ConfigLoaded      bool            `json:"configLoaded"`
	DBReady           bool            `json:"dbReady"`
	ChainsConnected   []string        `json:"chainsConnected"`
	RelayerSubscribed bool            `json:"relayerSubscribed"`
	Status            ReadinessStatus `json:"status"`
}

// ReadinessStatus contains relayer service endpoints.
type ReadinessStatus struct {
	HTTP string `json:"http"`
}

// Validate checks the readiness event contract.
func (r Readiness) Validate() error {
	if r.Event != ReadinessEvent {
		return fmt.Errorf("event = %q, want %q", r.Event, ReadinessEvent)
	}
	if !r.ConfigLoaded {
		return fmt.Errorf("configLoaded is false")
	}
	if !r.DBReady {
		return fmt.Errorf("dbReady is false")
	}
	if !r.RelayerSubscribed {
		return fmt.Errorf("relayerSubscribed is false")
	}
	if r.Status.HTTP == "" {
		return fmt.Errorf("status.http is empty")
	}
	return nil
}

// RelayRequest identifies source traffic for manual relay.
type RelayRequest struct {
	SourceChainID string `json:"sourceChainId"`
	SourceTxHash  string `json:"sourceTxHash"`
}

// PacketState is the acceptance-test view of a packet's relay state.
type PacketState string

const (
	// PacketPending has no terminal on-chain effect yet.
	PacketPending PacketState = "pending"
	// PacketComplete was delivered successfully.
	PacketComplete PacketState = "complete"
	// PacketTimedOut was refunded after timeout.
	PacketTimedOut PacketState = "timed_out"
	// PacketErrorAck was delivered with an error acknowledgement.
	PacketErrorAck PacketState = "error_ack"
)

// PacketStatus describes one packet's relay state.
type PacketStatus struct {
	PacketID     string      `json:"packetId"`
	RouteID      string      `json:"routeId"`
	Sequence     uint64      `json:"sequence"`
	State        PacketState `json:"state"`
	SourceTxHash string      `json:"sourceTxHash"`
	EffectTxHash string      `json:"effectTxHash,omitempty"`
}
