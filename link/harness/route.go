package harness

import (
	"errors"
	"fmt"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

func requireRoute(h *Harness, id string) (wire.Route, error) {
	if id == "" {
		return wire.Route{}, errors.New("harness: route is required")
	}
	r, ok := h.topo.Config.Route(id)
	if !ok {
		return wire.Route{}, fmt.Errorf("harness: unknown route %q", id)
	}
	return r, nil
}
