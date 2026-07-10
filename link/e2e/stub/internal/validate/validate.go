// Package validate implements `ibc config validate [--live]`: structural checks on the compiled
// config plus an optional liveness probe of each chain RPC. Structural problems exit 64 with a
// machine-readable error list; an unreachable --live RPC exits 65. Stable JSON goes to stdout; human
// logs go to stderr (via cobra's error path).
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

// liveDialTimeout bounds each per-chain liveness probe so one wedged RPC cannot hang the command.
const liveDialTimeout = 5 * time.Second

// Command builds the `config validate` leaf command.
func Command(flags *config.FlagSet) *cobra.Command {
	var live bool
	cmd := &cobra.Command{
		Use:          "validate",
		Short:        "validate a compiled IBC Link config (optionally probing each chain RPC with --live)",
		SilenceUsage: true, // a validation failure is a logic error, not a usage error — don't dump usage
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
		// A read/parse failure can't be located at a specific field, so report it as one structural
		// error against the file path and exit 64 like any other invalid config.
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
			// Structure is valid; the config is just not operationally reachable right now. Report
			// Valid:true with the reachability detail in Errors and exit 65 (distinct from structural 64).
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

// Check runs the structural validation rules and returns every problem found (empty == valid). It is
// pure — no network, no environment — so it runs in the every-PR fast lane and is unit-testable.
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

// resolvedChainIDs returns the configured chain ids in declared order — the "resolvedChains" the
// validate result reports back so the harness can confirm the stub saw the chains it expected.
func resolvedChainIDs(c *wire.ConfigYAML) []string {
	ids := make([]string, 0, len(c.Chains))
	for _, ch := range c.Chains {
		ids = append(ids, ch.ID)
	}
	return ids
}

// pingChains dials each chain's RPC and asks for its chain id (eth_chainId). A dial or call failure
// marks that chain unreachable; a node that answers but reports a different chain id than the config
// declares is flagged as a mismatch (a classic ops misconfiguration: the RPC URL points at the wrong
// network). Both are live-check failures (exit 65) — distinct from a structural-invalid config (64).
// The endpoint reported in the error is sanitized to scheme://host so a resolved-${NAME} credential
// embedded in the URL never lands in the stub's stdout (conventions §7).
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
		// Only assert the id when the config declares one (ChainID==0 means "unspecified"). A reachable
		// node on the wrong network is a real, silent operational fault — surface it here rather than
		// letting deploy submit transactions signed for the wrong chain id and fail cryptically later.
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

// pingOne dials rawURL and returns the node's reported chain id. A dial or eth_chainId failure returns
// errDial — a fixed, URL-free error so a resolved-secret-bearing URL never reaches stdout.
func pingOne(ctx context.Context, rawURL string) (uint64, error) {
	ctx, cancel := context.WithTimeout(ctx, liveDialTimeout)
	defer cancel()

	client, err := ethclient.DialContext(ctx, rawURL)
	if err != nil {
		return 0, errDial
	}
	defer client.Close()

	// eth_chainId is the cheapest "is this an answering EVM RPC" probe — and its result is what the
	// caller compares against the configured chain id.
	id, err := client.ChainID(ctx)
	if err != nil {
		return 0, errDial
	}
	return id.Uint64(), nil
}

// errDial is a fixed, URL-free error so the raw dial error (which can embed the full RPC URL, and thus
// a resolved secret) never reaches stdout. The sanitized endpoint is reported separately by the caller.
var errDial = fmt.Errorf("dial/chain-id probe failed")
