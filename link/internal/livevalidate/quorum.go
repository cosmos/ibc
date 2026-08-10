package livevalidate

import (
	"context"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/relay/proofgen/attestation"
	"github.com/cosmos/ibc/link/internal/service/attestor"
)

// checkAttestorQuorum confirms every attestation-type client end of every
// configured connection can currently satisfy its attestor quorum against
// on-chain state, without constructing ProofGenerators. Client ends of
// other light-client types are skipped, not attestation's concern.
func checkAttestorQuorum(
	ctx context.Context,
	cfg config.Config,
	clientSet *chains.ClientSet,
	attestors []attestor.Attestor,
) error {
	for _, conn := range cfg.Relayer.Connections {
		for _, end := range []struct {
			self, counterparty config.ClientEnd
		}{
			{conn.ClientA, conn.ClientB},
			{conn.ClientB, conn.ClientA},
		} {
			if end.self.Type != config.ClientTypeAttestation {
				continue
			}

			_, _, err := attestation.MatchAttestors(ctx, end.self, end.counterparty, clientSet, attestors)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
