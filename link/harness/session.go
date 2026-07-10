package harness

import (
	"fmt"

	"github.com/cosmos/ibc/link/harness/chain"
	"github.com/cosmos/ibc/link/harness/ibclink"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/internal/onchain"
)

type Session struct {
	h          *Harness
	deployment *wire.Deployment

	readers    map[string]onchain.Reader
	packets    onchain.Packets
	ift        *onchain.IFT
	gmp        *onchain.GMP
	submitters map[string]chain.AppSubmitter
}

func (r *Session) IBCLink() ibclink.Runner { return r.h.IBCLink() }

func (r *Session) reader(chainID string) (onchain.Reader, error) {
	rdr, ok := r.readers[chainID]
	if !ok {
		return nil, fmt.Errorf("harness: no on-chain reader for chain %q", chainID)
	}
	return rdr, nil
}
