// Package validate implements `ibc config validate`; structural errors exit 64, unreachable --live RPC exits 65.
package validate

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/link/e2e/stub/internal/cfg"
	"github.com/cosmos/ibc/link/e2e/stub/internal/exitcode"
	"github.com/cosmos/ibc/link/e2e/stub/internal/rpcsafe"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/internal/config"
)

const liveDialTimeout = 5 * time.Second

func Command(flags *config.FlagSet) *cobra.Command {
	var live bool
	cmd := &cobra.Command{
		Use:          "validate",
		Short:        "validate a compiled IBC Link config (optionally probing each chain RPC with --live)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(cmd, flags, live)
		},
	}
	cmd.Flags().BoolVar(&live, "live", false, "also dial each chain RPC and fail (exit 65) if any is unreachable")
	return cmd
}

func run(cmd *cobra.Command, flags *config.FlagSet, live bool) error {
	c, err := cfg.Setup(flags)
	if err != nil {
		path, pathErr := flags.ConfigPath()
		if pathErr != nil {
			path = flags.Config
		}
		_ = config.PrintJSON(wire.ValidateResult{
			Valid:    false,
			Warnings: []string{},
			Errors:   []wire.ValidationError{{Path: path, Msg: err.Error()}},
		})
		return exitcode.New(wire.ExitConfigInvalid, fmt.Errorf("config invalid: %w", err))
	}

	if errs := Check(c); len(errs) > 0 {
		_ = config.PrintJSON(wire.ValidateResult{Valid: false, Warnings: []string{}, Errors: errs})
		return exitcode.New(wire.ExitConfigInvalid, fmt.Errorf("config invalid: %d structural error(s)", len(errs)))
	}

	chains := resolvedChainIDs(c)

	if live {
		if errs := pingChains(cmd.Context(), c); len(errs) > 0 {
			// Valid structure but unreachable RPC: Valid:true on stdout, exit 65 (not 64).
			_ = config.PrintJSON(
				wire.ValidateResult{Valid: true, ResolvedChains: chains, Warnings: []string{}, Errors: errs},
			)
			return exitcode.New(
				wire.ExitRPCUnreachable,
				fmt.Errorf("live check failed: %d chain(s) unreachable", len(errs)),
			)
		}
	}

	_ = config.PrintJSON(wire.ValidateResult{Valid: true, ResolvedChains: chains, Warnings: []string{}})
	return nil
}

func Check(c *wire.ConfigYAML) []wire.ValidationError {
	var errs []wire.ValidationError
	chainIDs := map[string]bool{}
	chainTypes := map[string]string{}
	chainSignerKeys := map[string]string{}

	for i, ch := range c.Chains {
		if ch.RPC.URL == "" {
			errs = append(errs, wire.ValidationError{
				Path: fmt.Sprintf("chains[%d].rpc.url", i),
				Msg:  "rpc url is empty",
			})
		}
		switch ch.Type {
		case wire.ChainTypeEVM:
			switch ch.Provider {
			case wire.ProviderAnvil, wire.ProviderBesu:
			case "":
				errs = append(errs, wire.ValidationError{
					Path: fmt.Sprintf("chains[%d].provider", i),
					Msg:  "provider is empty",
				})
			default:
				errs = append(errs, wire.ValidationError{
					Path: fmt.Sprintf("chains[%d].provider", i),
					Msg: fmt.Sprintf(
						"unsupported provider %q (POC supports %q or %q)",
						ch.Provider, wire.ProviderAnvil, wire.ProviderBesu,
					),
				})
			}
		case "":
			errs = append(errs, wire.ValidationError{
				Path: fmt.Sprintf("chains[%d].type", i),
				Msg:  "chain type is empty",
			})
		default:
			errs = append(errs, wire.ValidationError{
				Path: fmt.Sprintf("chains[%d].type", i),
				Msg:  fmt.Sprintf("unsupported chain type %q (POC supports %q)", ch.Type, wire.ChainTypeEVM),
			})
		}
		if ch.ID == "" {
			errs = append(errs, wire.ValidationError{Path: fmt.Sprintf("chains[%d].id", i), Msg: "chain id is empty"})
			continue
		}
		if chainIDs[ch.ID] {
			errs = append(errs, wire.ValidationError{
				Path: fmt.Sprintf("chains[%d].id", i),
				Msg:  fmt.Sprintf("duplicate chain id %q", ch.ID),
			})
			continue
		}
		chainIDs[ch.ID] = true
		chainTypes[ch.ID] = ch.Type
		chainSignerKeys[ch.ID] = ch.EVMSignerKey
	}

	if err := wire.ValidateDB(c.DB); err != nil {
		errs = append(errs, wire.ValidationError{Path: "db.url", Msg: err.Error()})
	}

	routeIDs := map[string]bool{}
	for i, route := range c.Relayer.Routes {
		path := fmt.Sprintf("relayer.routes[%d]", i)
		switch {
		case route.ID == "":
			errs = append(errs, wire.ValidationError{Path: path + ".id", Msg: "route id is empty"})
		case routeIDs[route.ID]:
			errs = append(errs, wire.ValidationError{
				Path: path + ".id",
				Msg:  fmt.Sprintf("duplicate route id %q", route.ID),
			})
		default:
			routeIDs[route.ID] = true
		}
		if !chainIDs[route.Source] {
			errs = append(errs, wire.ValidationError{
				Path: path + ".source",
				Msg:  fmt.Sprintf("source %q names no defined chain", route.Source),
			})
		}
		if !chainIDs[route.Destination] {
			errs = append(errs, wire.ValidationError{
				Path: path + ".destination",
				Msg:  fmt.Sprintf("destination %q names no defined chain", route.Destination),
			})
		}

		knownType := route.Type == wire.RouteEVMToEVMAttested
		if !knownType {
			errs = append(errs, wire.ValidationError{
				Path: path + ".type",
				Msg:  fmt.Sprintf("unknown route type %q", route.Type),
			})
		}
		for _, endpoint := range []struct{ path, id string }{
			{path + ".source", route.Source},
			{path + ".destination", route.Destination},
		} {
			if chainIDs[endpoint.id] && chainTypes[endpoint.id] == wire.ChainTypeEVM &&
				chainSignerKeys[endpoint.id] == "" {
				errs = append(errs, wire.ValidationError{
					Path: endpoint.path,
					Msg:  fmt.Sprintf("route endpoint %q is an EVM chain but declares no evmSignerKey", endpoint.id),
				})
			}
		}
		if knownType && chainIDs[route.Source] && chainIDs[route.Destination] {
			want, relays := wire.RouteTypeFor(chainTypes[route.Source], chainTypes[route.Destination])
			switch {
			case !relays:
				errs = append(errs, wire.ValidationError{
					Path: path + ".type",
					Msg: fmt.Sprintf(
						"no route type relays %s -> %s",
						chainTypes[route.Source], chainTypes[route.Destination],
					),
				})
			case route.Type != want:
				errs = append(errs, wire.ValidationError{
					Path: path + ".type",
					Msg: fmt.Sprintf(
						"route type %q does not match endpoint families (%s -> %s needs %q)",
						route.Type, chainTypes[route.Source], chainTypes[route.Destination], want,
					),
				})
			}
		}
	}
	return errs
}

func resolvedChainIDs(c *wire.ConfigYAML) []string {
	ids := make([]string, 0, len(c.Chains))
	for _, ch := range c.Chains {
		ids = append(ids, ch.ID)
	}
	return ids
}

func pingChains(ctx context.Context, c *wire.ConfigYAML) []wire.ValidationError {
	var errs []wire.ValidationError
	for i, ch := range c.Chains {
		got, err := pingOne(ctx, ch.RPC.URL)
		if err != nil {
			errs = append(errs, wire.ValidationError{
				Path: fmt.Sprintf("chains[%d].rpc.url", i),
				Msg:  fmt.Sprintf("rpc unreachable (%s): %s", rpcsafe.Endpoint(ch.RPC.URL), err),
			})
			continue
		}
		if ch.ChainID != 0 && got != ch.ChainID {
			errs = append(errs, wire.ValidationError{
				Path: fmt.Sprintf("chains[%d].chainId", i),
				Msg: fmt.Sprintf(
					"chain id mismatch (%s): config declares %d, node reports %d",
					rpcsafe.Endpoint(ch.RPC.URL),
					ch.ChainID,
					got,
				),
			})
		}
	}
	return errs
}

func pingOne(ctx context.Context, rawURL string) (uint64, error) {
	ctx, cancel := context.WithTimeout(ctx, liveDialTimeout)
	defer cancel()

	client, err := ethclient.DialContext(ctx, rawURL)
	if err != nil {
		return 0, errDial
	}
	defer client.Close()

	id, err := client.ChainID(ctx)
	if err != nil {
		return 0, errDial
	}
	return id.Uint64(), nil
}

// Fixed URL-free probe error; caller reports rpcsafe.Endpoint separately so secrets never reach stdout.
var errDial = fmt.Errorf("dial/chain-id probe failed")
