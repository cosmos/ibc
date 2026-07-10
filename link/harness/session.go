package harness

import (
	"fmt"

	"github.com/cosmos/ibc/link/harness/chain"
	"github.com/cosmos/ibc/link/harness/ibclink"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/onchain"
)

// Session is a Harness plus a successful deployment and running daemon — the test surface for a live
// relayer flow (session.IFT, session.GMP, session.Chain handles; verification happens through the typed
// outcomes those return). It is a driver over the harness's world, not a resource owner: the daemon
// lives on the Harness's process ledger, and teardown is Harness.Shutdown. Like the rest of the harness
// surface, a Session is single-threaded by test convention.
type Session struct {
	h          *Harness
	deployment *wire.Deployment

	// readers is one on-chain Reader per chain (keyed by Chain.ID), built once from the deployment
	// the stub reported. The correlator (packets), the ift/gmp asserters, and the prepare/verify paths
	// all read through these, so the harness surface never touches a chain client for its independent reads.
	readers map[string]onchain.Reader

	// packets (the on-chain correlator) and the ift/gmp asserters are built once over the immutable
	// readers map at construction, since every IFT/GMP verify call reuses them. All three are consumed by
	// the typed outcomes, never exposed to tests.
	packets onchain.Packets
	ift     *onchain.IFT
	gmp     *onchain.GMP

	// submitters is one chain.AppSubmitter per chain (keyed by Chain.ID) — the write-side twin of
	// readers; see chain.AppSubmitter for the seam's contract.
	submitters map[string]chain.AppSubmitter
}

// IBCLink returns the ibc link driver bound to this run's compiled config.
func (r *Session) IBCLink() ibclink.Runner { return r.h.IBCLink() }

// reader returns the on-chain Reader for chainID, or a clear error if the run bound none.
func (r *Session) reader(chainID string) (onchain.Reader, error) {
	rdr, ok := r.readers[chainID]
	if !ok {
		return nil, fmt.Errorf("harness: no on-chain reader for chain %q", chainID)
	}
	return rdr, nil
}
