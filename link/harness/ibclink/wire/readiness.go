package wire

import "fmt"

const ReadinessEvent = "ready"

// This must be the first stdout line from `ibc relayer run`.
type Readiness struct {
	Event             string          `json:"event"`
	ConfigLoaded      bool            `json:"configLoaded"`
	DBReady           bool            `json:"dbReady"`
	ChainsConnected   []string        `json:"chainsConnected"`
	RelayerSubscribed bool            `json:"relayerSubscribed"`
	Status            ReadinessStatus `json:"status"`
}

type ReadinessStatus struct {
	HTTP string `json:"http"`
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
