// SPDX-License-Identifier: Apache-2.0

package livevalidate

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
)

// validateConnectionsLive queries each configured connection's two chains to
// confirm the on-chain registered counterparty actually matches clientA/
// clientB, in both directions.
func validateConnectionsLive(ctx context.Context, cfg config.Config, clients *chains.ClientSet) error {
	for _, conn := range cfg.Relayer.Connections {
		err := func() error {
			for _, end := range []struct {
				label              string
				self, counterparty config.ClientEnd
			}{
				{"clientA", conn.ClientA, conn.ClientB},
				{"clientB", conn.ClientB, conn.ClientA},
			} {
				client, ok := clients.Get(end.self.ChainID)
				if !ok {
					return errors.Errorf("%s: no chain client for %q", end.label, end.self.ChainID)
				}

				onChainCounterpartyID, err := client.GetCounterparty(ctx, end.self.ClientID)
				if err != nil {
					return errors.Wrapf(
						err, "%s: querying on-chain counterparty for client %q", end.label, end.self.ClientID,
					)
				}

				if onChainCounterpartyID != end.counterparty.ClientID {
					return errors.Errorf(
						"%s: on-chain counterparty %q does not match configured counterparty %q",
						end.label, onChainCounterpartyID, end.counterparty.ClientID,
					)
				}
			}

			return nil
		}()
		if err != nil {
			return errors.Wrapf(err, "connection %q", conn.Alias)
		}
	}

	return nil
}
