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

// RoutePacketID constructs the stable synthetic IBC packet identifier.
func RoutePacketID(routeID string, seq uint64) string {
	return fmt.Sprintf("%s-%d", routeID, seq)
}

// HealthPath is the relayer health endpoint.
const HealthPath = "/health"

const (
	// ReadinessEvent identifies the first relayer stdout event.
	ReadinessEvent = "ready"
	// RelayPath is the manual relay endpoint.
	RelayPath = "/relay"
	// StatusPath is the packet status endpoint.
	StatusPath = "/status"
	// StatusQueryRoute filters status by route.
	StatusQueryRoute = "route"
	// StatusQueryPacket filters status by packet.
	StatusQueryPacket = "packet"
)

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

// RelayResult reports packets discovered for a manual relay request.
type RelayResult struct {
	PacketIDs []string `json:"packetIds"`
}

// StatusQuery filters relayer status; the zero value asks for everything.
type StatusQuery struct {
	RouteID  string
	PacketID string
}

// PacketState is the persisted synthetic packet state.
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

// Status contains the matching synthetic packets.
type Status struct {
	Packets []PacketStatus `json:"packets"`
}

// PacketStatus describes one synthetic packet.
type PacketStatus struct {
	PacketID     string      `json:"packetId"`
	RouteID      string      `json:"routeId"`
	Sequence     uint64      `json:"sequence"`
	State        PacketState `json:"state"`
	SourceTxHash string      `json:"sourceTxHash"`
	EffectTxHash string      `json:"effectTxHash,omitempty"`
	Reason       string      `json:"reason,omitempty"`
}

// Packet returns a status by packet ID.
func (s *Status) Packet(packetID string) (PacketStatus, bool) {
	for _, packet := range s.Packets {
		if packet.PacketID == packetID {
			return packet, true
		}
	}
	return PacketStatus{}, false
}
