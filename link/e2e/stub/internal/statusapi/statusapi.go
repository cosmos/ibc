// Package statusapi serves the daemon's HTTP API from the sqlite store over its dynamic listen address.
package statusapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cosmos/ibc/link/e2e/stub/internal/jsonout"
	"github.com/cosmos/ibc/link/e2e/stub/internal/store"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

// Handler serves the status, relay, and health API. discover resolves one source transaction on demand.
func Handler(
	st *store.Store,
	cfg *wire.ConfigYAML,
	discover func(context.Context, string, string) error,
) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+wire.StatusPath, func(w http.ResponseWriter, r *http.Request) {
		pkts, err := st.Packets(r.Context(), r.URL.Query().Get(wire.StatusQueryPacket))
		if err != nil {
			http.Error(w, fmt.Sprintf("status: %v", err), http.StatusInternalServerError)
			return
		}

		routeFilter := r.URL.Query().Get(wire.StatusQueryRoute)
		packetOut := make([]wire.PacketStatus, 0, len(pkts))
		for _, p := range pkts {
			if routeFilter != "" && p.RouteID != routeFilter {
				continue
			}
			packetOut = append(packetOut, wire.PacketStatus{
				PacketID:     p.PacketID,
				RouteID:      p.RouteID,
				Sequence:     p.Sequence,
				State:        p.State,
				SourceTxHash: p.SourceTxHash,
				EffectTxHash: p.EffectTxHash,
				Reason:       p.Reason,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = jsonout.Write(w, wire.Status{Packets: packetOut}) // same wire framing as the other JSON surfaces
	})
	mux.HandleFunc("POST "+wire.RelayPath, func(w http.ResponseWriter, r *http.Request) {
		var req wire.RelayRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("relay: decode request: %v", err), http.StatusBadRequest)
			return
		}
		if req.SourceChainID == "" || req.SourceTxHash == "" {
			http.Error(w, "relay: sourceChainId and sourceTxHash are required", http.StatusBadRequest)
			return
		}

		pkts, err := st.PacketsBySourceTx(r.Context(), req.SourceTxHash)
		if err != nil {
			http.Error(w, fmt.Sprintf("relay: %v", err), http.StatusInternalServerError)
			return
		}
		ids, canonicalHash := packetIDsFromSource(cfg, pkts, req.SourceChainID)
		var discoverErr error
		if len(ids) == 0 {
			discoverErr = discover(r.Context(), req.SourceChainID, req.SourceTxHash)
			pkts, err = st.PacketsBySourceTx(r.Context(), req.SourceTxHash)
			if err != nil {
				http.Error(w, fmt.Sprintf("relay: %v", err), http.StatusInternalServerError)
				return
			}
			ids, canonicalHash = packetIDsFromSource(cfg, pkts, req.SourceChainID)
		}
		if len(ids) == 0 {
			if discoverErr != nil {
				http.Error(
					w,
					fmt.Sprintf(
						"relay: discovery failed while resolving sourceChainId %q sourceTxHash %q: %v",
						req.SourceChainID,
						req.SourceTxHash,
						discoverErr,
					),
					http.StatusBadGateway,
				)
				return
			}
			http.Error(
				w,
				fmt.Sprintf(
					"relay: no packet found for sourceChainId %q sourceTxHash %q",
					req.SourceChainID,
					req.SourceTxHash,
				),
				http.StatusNotFound,
			)
			return
		}
		if err := st.RequestRelay(r.Context(), req.SourceChainID, canonicalHash); err != nil {
			http.Error(w, fmt.Sprintf("relay: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = jsonout.Write(w, wire.RelayResult{PacketIDs: ids})
	})
	mux.HandleFunc("GET "+wire.HealthPath, func(w http.ResponseWriter, _ *http.Request) {
		// The 200 is the whole health contract; the harness probe checks the status code only.
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

func packetIDsFromSource(
	cfg *wire.ConfigYAML,
	packets []store.Packet,
	sourceChainID string,
) ([]string, string) {
	ids := make([]string, 0, len(packets))
	var canonicalHash string
	for _, p := range packets {
		route, ok := cfg.Route(p.RouteID)
		if ok && route.Source == sourceChainID {
			ids = append(ids, p.PacketID)
			if canonicalHash == "" {
				canonicalHash = p.SourceTxHash
			}
		}
	}
	return ids, canonicalHash
}
