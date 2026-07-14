// Package statusapi serves the daemon HTTP API from sqlite.
package stub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cosmos/ibc/link/cmd/configcmd"
	"github.com/cosmos/ibc/link/cmd/relayercmd"
)

func statusHandler(
	st *stubStore,
	cfg *configcmd.Config,
	discover func(context.Context, string, string) error,
) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+relayercmd.StatusPath, func(w http.ResponseWriter, r *http.Request) {
		pkts, err := st.Packets(r.Context(), r.URL.Query().Get(relayercmd.StatusQueryPacket))
		if err != nil {
			http.Error(w, fmt.Sprintf("status: %v", err), http.StatusInternalServerError)
			return
		}

		routeFilter := r.URL.Query().Get(relayercmd.StatusQueryRoute)
		packetOut := make([]relayercmd.PacketStatus, 0, len(pkts))
		for _, p := range pkts {
			if routeFilter != "" && p.RouteID != routeFilter {
				continue
			}
			packetOut = append(packetOut, relayercmd.PacketStatus{
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
		_ = writeJSON(w, relayercmd.Status{Packets: packetOut})
	})
	mux.HandleFunc("POST "+relayercmd.RelayPath, func(w http.ResponseWriter, r *http.Request) {
		var req relayercmd.RelayRequest
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
		_ = writeJSON(w, relayercmd.RelayResult{PacketIDs: ids})
	})
	mux.HandleFunc("GET "+relayercmd.HealthPath, func(w http.ResponseWriter, _ *http.Request) {
		// The driver health probe checks status code only (200, empty body).
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

func packetIDsFromSource(
	cfg *configcmd.Config,
	packets []storedPacket,
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
