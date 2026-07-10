package wire

import "fmt"

// ReadinessEvent is the value the daemon's readiness line carries in its "event" field. It is a
// constant (not a free string) because the harness keys off it to distinguish the readiness JSON
// from any later structured line the daemon might print to stdout.
const ReadinessEvent = "ready"

// Readiness is the FIRST line `ibc relayer run` writes to stdout. The relayer is a long-lived
// daemon, so the harness cannot use the one-shot exit-code contract to learn it came up; instead it
// reads and parses this single JSON line as the SEMANTIC readiness signal (no blind sleep). Every
// boolean is a precondition the daemon asserts it has satisfied before serving, and Status.HTTP hands
// the harness the dynamically-allocated status-API address to poll thereafter.
//
// The stub marshals this type and the harness parses the same type, so the readiness wire format cannot
// drift between them.
type Readiness struct {
	Event             string          `json:"event"`             // always ReadinessEvent ("ready")
	ConfigLoaded      bool            `json:"configLoaded"`      // config parsed + ${NAME} resolved
	DBReady           bool            `json:"dbReady"`           // relayer DB schema is available
	ChainsConnected   []string        `json:"chainsConnected"`   // Chain.IDs the daemon dialed successfully
	RelayerSubscribed bool            `json:"relayerSubscribed"` // event subscription on each route source is live
	Status            ReadinessStatus `json:"status"`
}

// ReadinessStatus carries the daemon's status-API address. HTTP is host:port with a DYNAMIC port
// (never a fixed one), so the harness must learn it here rather than assume it — this is what lets
// many daemons run in parallel without port collisions.
type ReadinessStatus struct {
	HTTP string `json:"http"` // status-API listen address, e.g. "127.0.0.1:<dynamic>"
}

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
