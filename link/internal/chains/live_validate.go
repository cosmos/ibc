package chains

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/config"
)

// ValidateConnectionsLive queries each configured connection's two chains to
// confirm the on-chain registered counterparty actually matches clientA/
// clientB, in both directions.
func ValidateConnectionsLive(ctx context.Context, cfg config.Config, clients *ClientSet) error {
	for _, conn := range cfg.Relayer.Connections {
		if err := validateConnectionLive(ctx, conn, clients); err != nil {
			return errors.Wrapf(err, "connection %q", conn.Alias)
		}
	}

	return nil
}

func validateConnectionLive(ctx context.Context, conn config.ConnectionConfig, clients *ClientSet) error {
	for _, end := range []struct {
		label              string
		self, counterparty config.ClientConfig
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
			return errors.Wrapf(err, "%s: querying on-chain counterparty for client %q", end.label, end.self.ClientID)
		}

		if onChainCounterpartyID != end.counterparty.ClientID {
			return errors.Errorf(
				"%s: on-chain counterparty %q does not match configured counterparty %q",
				end.label, onChainCounterpartyID, end.counterparty.ClientID,
			)
		}
	}

	return nil
}
