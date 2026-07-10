package wire

import (
	"fmt"
	"strings"
)

// AppType names which application flow an action drove. It is typed so the SUT and harness share the
// exact wire tokens ("IFT"/"GMP") rather than scattered literals. It is part of packet identity (a
// two-sided contract): the SUT must derive the same packet id the harness does for a transaction the
// harness submitted directly on-chain (see PacketID).
type AppType string

const (
	// AppTypeIFT is the wire token for fungible-token-transfer actions.
	AppTypeIFT AppType = "IFT" // fungible token transfer (MockIFT)
	// AppTypeGMP is the wire token for general-message-passing actions.
	AppTypeGMP AppType = "GMP" // general message passing (MockGMP)
)

// PacketID derives the deterministic packet id from a route id, the app type, and the source sequence.
// It is a WIRE CONTRACT: IFT/GMP are end-user transactions submitted directly on-chain (the SUT has no
// tx-submission verbs), so the relayer discovers a packet from chain state and must derive its id for a
// transaction it did not author. The harness (which submitted it) and the relayer (which discovers and
// completes it) compute this identically, so they converge on one row per (route, app, sequence) with
// no coordination.
//
// The app type is folded into the id because IFT and GMP keep independent sequence counters (each fixture
// has its own seq), so a route's IFT seq=1 and GMP seq=1 would otherwise collide on a single primary-key
// row. Format: <route>-<lowercase app>-<seq>.
func PacketID(routeID string, app AppType, seq uint64) string {
	return fmt.Sprintf("%s-%s-%d", routeID, strings.ToLower(string(app)), seq)
}
