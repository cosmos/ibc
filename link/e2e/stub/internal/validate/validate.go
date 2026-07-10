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
	"github.com/cosmos/ibc/link/e2e/stub/internal/cosmos"
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

	// Chain ids must be present and unique — routes reference chains by id, so a missing/duplicate id
	// makes every reference ambiguous. chainType records each chain's family so the route checks below can
	// enforce a route type's source/destination families.
	chainIDs := map[string]bool{}
	chainType := map[string]string{}
	// chainEVMSignerKey records each EVM chain's configured signer key so a route with an EVM destination
	// can require it (the relayer cannot sign the destination effect without one).
	chainEVMSignerKey := map[string]string{}
	for i, ch := range c.Chains {
		// A chain with no RPC URL can never be dialed. This applies to every family.
		if ch.RPC.URL == "" {
			errs = append(errs, wire.ValidationError{
				Path: fmt.Sprintf("chains[%d].rpc.url", i),
				Msg:  "rpc url is empty",
			})
		}
		// Validate the provider per family: an EVM chain runs on anvil/besu/sandbox; a cosmos chain runs on
		// the sandbox provider and carries no numeric chain id (that is an EVM-only field).
		switch ch.Type {
		case wire.ChainTypeEVM:
			switch ch.Provider {
			case wire.ProviderAnvil, wire.ProviderBesu, wire.ProviderSandbox:
			case "":
				errs = append(
					errs,
					wire.ValidationError{Path: fmt.Sprintf("chains[%d].provider", i), Msg: "provider is empty"},
				)
			default:
				errs = append(
					errs,
					wire.ValidationError{
						Path: fmt.Sprintf("chains[%d].provider", i),
						Msg: fmt.Sprintf(
							"unsupported provider %q (POC supports %q, %q or %q)",
							ch.Provider,
							wire.ProviderAnvil,
							wire.ProviderBesu,
							wire.ProviderSandbox,
						),
					},
				)
			}
		case wire.ChainTypeCosmos:
			// A cosmos chain runs on the sandbox provider (the sandboxd node driven over its cosmos surfaces),
			// carries no EVM numeric chainId (that is an EVM-only field), and requires its cosmos chain-id, its
			// gRPC dial target (alongside the CometBFT RPC in rpc.url), the relayer/admin signer key, and the
			// user/faucet key a cosmos-source transfer burns from. Each is a hard requirement: without any one the
			// daemon could not connect, deploy, sign the destination effect, or submit a cosmos-source transfer.
			if ch.Provider != wire.ProviderSandbox {
				errs = append(
					errs,
					wire.ValidationError{
						Path: fmt.Sprintf("chains[%d].provider", i),
						Msg: fmt.Sprintf(
							"cosmos chain provider must be %q, got %q",
							wire.ProviderSandbox,
							ch.Provider,
						),
					},
				)
			}
			if ch.ChainID != 0 {
				errs = append(
					errs,
					wire.ValidationError{
						Path: fmt.Sprintf("chains[%d].chainId", i),
						Msg:  "cosmos chain must not set the EVM numeric chainId (use cosmosChainId)",
					},
				)
			}
			if ch.EVMSignerKey != "" {
				errs = append(
					errs,
					wire.ValidationError{
						Path: fmt.Sprintf("chains[%d].evmSignerKey", i),
						Msg:  "evmSignerKey is only valid on an EVM chain (cosmos chains sign with signerKey)",
					},
				)
			}
			if ch.CosmosChainID == "" {
				errs = append(
					errs,
					wire.ValidationError{
						Path: fmt.Sprintf("chains[%d].cosmosChainId", i),
						Msg:  "cosmos chain-id is empty",
					},
				)
			}
			if ch.GRPCURL == "" {
				errs = append(
					errs,
					wire.ValidationError{
						Path: fmt.Sprintf("chains[%d].grpcUrl", i),
						Msg:  "cosmos grpc url is empty",
					},
				)
			}
			if ch.SignerKey == "" {
				errs = append(
					errs,
					wire.ValidationError{
						Path: fmt.Sprintf("chains[%d].signerKey", i),
						Msg:  "cosmos chain requires a signer key",
					},
				)
			}
			// The faucet key is the Cosmos user whose native IFT balance a source transfer burns.
			if ch.FaucetKey == "" {
				errs = append(
					errs,
					wire.ValidationError{
						Path: fmt.Sprintf("chains[%d].faucetKey", i),
						Msg:  "cosmos chain requires a faucet key",
					},
				)
			}
		case "":
			errs = append(
				errs,
				wire.ValidationError{Path: fmt.Sprintf("chains[%d].type", i), Msg: "chain type is empty"},
			)
		default:
			errs = append(
				errs,
				wire.ValidationError{
					Path: fmt.Sprintf("chains[%d].type", i),
					Msg: fmt.Sprintf(
						"unsupported chain type %q (POC supports %q or %q)",
						ch.Type,
						wire.ChainTypeEVM,
						wire.ChainTypeCosmos,
					),
				},
			)
		}
		// Strict: the cosmos-only fields must not appear on a non-cosmos chain — a stray signer key or REST
		// url on an EVM chain is a config authoring error, surfaced rather than silently ignored.
		if ch.Type != wire.ChainTypeCosmos {
			if ch.CosmosChainID != "" {
				errs = append(
					errs,
					wire.ValidationError{
						Path: fmt.Sprintf("chains[%d].cosmosChainId", i),
						Msg:  "cosmosChainId is only valid on a cosmos chain",
					},
				)
			}
			if ch.GRPCURL != "" {
				errs = append(
					errs,
					wire.ValidationError{
						Path: fmt.Sprintf("chains[%d].grpcUrl", i),
						Msg:  "grpcUrl is only valid on a cosmos chain",
					},
				)
			}
			if ch.SignerKey != "" {
				errs = append(
					errs,
					wire.ValidationError{
						Path: fmt.Sprintf("chains[%d].signerKey", i),
						Msg:  "signerKey is only valid on a cosmos chain",
					},
				)
			}
			if ch.FaucetKey != "" {
				errs = append(
					errs,
					wire.ValidationError{
						Path: fmt.Sprintf("chains[%d].faucetKey", i),
						Msg:  "faucetKey is only valid on a cosmos chain",
					},
				)
			}
		}
		if ch.ID == "" {
			errs = append(errs, wire.ValidationError{Path: fmt.Sprintf("chains[%d].id", i), Msg: "chain id is empty"})
			continue
		}
		if chainIDs[ch.ID] {
			errs = append(
				errs,
				wire.ValidationError{
					Path: fmt.Sprintf("chains[%d].id", i),
					Msg:  fmt.Sprintf("duplicate chain id %q", ch.ID),
				},
			)
			continue
		}
		chainIDs[ch.ID] = true
		chainType[ch.ID] = ch.Type
		chainEVMSignerKey[ch.ID] = ch.EVMSignerKey
	}

	// The DB needs a usable sqlite file location; validate rejects paths the store cannot open.
	if err := wire.ValidateDB(c.DB); err != nil {
		errs = append(errs, wire.ValidationError{Path: "db.url", Msg: err.Error()})
	}

	routeIDs := map[string]bool{}
	// One cosmosToEvm route per source chain: deploy creates a single attestations client per cosmos
	// chain and discovery attributes a GMP packet to its route by that chain-level client (see
	// relay.cosmosSourceGMPRoute), so a second such route from one source could not be told apart —
	// reject it here rather than misattribute packets at relay time.
	cosmosToEvmSources := map[string]bool{}
	// The prototype deploys one native IFT bridge (denom + IBC client + counterparty contract) per Cosmos
	// chain. Both directions may share that bridge, but a second EVM counterparty would require a distinct
	// native denom/client pair and cannot safely reuse the chain-level deployment fixtures.
	cosmosCounterparty := map[string]string{}
	for i, r := range c.Relayer.Routes {
		p := fmt.Sprintf("relayer.routes[%d]", i)
		switch {
		case r.ID == "":
			errs = append(errs, wire.ValidationError{Path: p + ".id", Msg: "route id is empty"})
		case routeIDs[r.ID]:
			errs = append(errs, wire.ValidationError{Path: p + ".id", Msg: fmt.Sprintf("duplicate route id %q", r.ID)})
		default:
			routeIDs[r.ID] = true
		}
		if !chainIDs[r.Source] {
			errs = append(
				errs,
				wire.ValidationError{
					Path: p + ".source",
					Msg:  fmt.Sprintf("source %q names no defined chain", r.Source),
				},
			)
		}
		if !chainIDs[r.Destination] {
			errs = append(
				errs,
				wire.ValidationError{
					Path: p + ".destination",
					Msg:  fmt.Sprintf("destination %q names no defined chain", r.Destination),
				},
			)
		}
		// Reject any type outside the known set: an unrecognized (or empty/typo'd) type would otherwise be
		// relayed as a plain EVM<->EVM route, silently ignoring the attested flow the author asked for.
		knownType := true
		switch r.Type {
		case wire.RouteEVMToEVMAttested, wire.RouteEVMToCosmosAttested, wire.RouteCosmosToEVMAttested:
		default:
			knownType = false
			errs = append(errs, wire.ValidationError{
				Path: p + ".type",
				Msg:  fmt.Sprintf("unknown route type %q", r.Type),
			})
		}
		// Any EVM chain a route touches requires that chain's EVM signer key: the relayer signs destination
		// deliveries and source refunds from it, and dialChains rejects an EVM chain without one at startup.
		// Checked only once the endpoint resolves (a dangling reference is already flagged above), mirroring
		// how a cosmos chain's signerKey is required per-chain.
		for _, ep := range []struct{ path, id string }{{p + ".source", r.Source}, {p + ".destination", r.Destination}} {
			if chainIDs[ep.id] && chainType[ep.id] == wire.ChainTypeEVM && chainEVMSignerKey[ep.id] == "" {
				errs = append(errs, wire.ValidationError{
					Path: ep.path,
					Msg:  fmt.Sprintf("route endpoint %q is an EVM chain but declares no evmSignerKey", ep.id),
				})
			}
		}
		if r.Type == wire.RouteCosmosToEVMAttested {
			if cosmosToEvmSources[r.Source] {
				errs = append(errs, wire.ValidationError{
					Path: p + ".source",
					Msg: fmt.Sprintf(
						"chain %q already sources a %s route (one per source chain)",
						r.Source, wire.RouteCosmosToEVMAttested,
					),
				})
			}
			cosmosToEvmSources[r.Source] = true
		}
		var cosmosID, evmID, endpointPath string
		switch r.Type {
		case wire.RouteCosmosToEVMAttested:
			cosmosID, evmID, endpointPath = r.Source, r.Destination, p+".destination"
		case wire.RouteEVMToCosmosAttested:
			cosmosID, evmID, endpointPath = r.Destination, r.Source, p+".source"
		}
		if chainType[cosmosID] == wire.ChainTypeCosmos && chainType[evmID] == wire.ChainTypeEVM {
			if prior, ok := cosmosCounterparty[cosmosID]; ok && prior != evmID {
				const oneCounterparty = "one counterparty per Cosmos chain"
				errs = append(errs, wire.ValidationError{
					Path: endpointPath,
					Msg: fmt.Sprintf(
						"cosmos chain %q is already paired with EVM counterparty %q (%s)",
						cosmosID, prior,
						oneCounterparty,
					),
				})
			} else {
				cosmosCounterparty[cosmosID] = evmID
			}
		}
		// A route's type must agree with its endpoint families — the same derivation the harness binds
		// and validates topologies with (wire.RouteTypeFor). Enforced only when both endpoints resolve
		// and the type is in the known set (each already flagged above), so a misconfigured route fails
		// here rather than misbehaving in the relay loop, and each field carries at most one error.
		if knownType && chainIDs[r.Source] && chainIDs[r.Destination] {
			want, relays := wire.RouteTypeFor(chainType[r.Source], chainType[r.Destination])
			switch {
			case !relays:
				errs = append(errs, wire.ValidationError{
					Path: p + ".type",
					Msg: fmt.Sprintf(
						"no route type relays %s -> %s",
						chainType[r.Source], chainType[r.Destination],
					),
				})
			case r.Type != want:
				errs = append(errs, wire.ValidationError{
					Path: p + ".type",
					Msg: fmt.Sprintf(
						"route type %q does not match endpoint families (%s -> %s needs %q)",
						r.Type, chainType[r.Source], chainType[r.Destination], want,
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
		// A cosmos chain speaks CometBFT RPC: liveness is its /status answering (via the stub's cosmos
		// client). rpc.url is the CometBFT RPC; there is no EVM numeric chain id to cross-check.
		if ch.Type == wire.ChainTypeCosmos {
			if err := pingCosmos(ctx, ch); err != nil {
				errs = append(errs, wire.ValidationError{
					Path: fmt.Sprintf("chains[%d].rpc.url", i),
					Msg:  fmt.Sprintf("cosmos chain unreachable (%s): %s", rpcsafe.Endpoint(ch.RPC.URL), err),
				})
			}
			continue
		}
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

// pingCosmos connects to a cosmos chain over its CometBFT RPC (probing /status) as its liveness check, the
// cosmos analog of pingOne. It returns a fixed, URL-free error so a resolved-secret-bearing URL never
// reaches stdout.
func pingCosmos(ctx context.Context, ch wire.Chain) error {
	ctx, cancel := context.WithTimeout(ctx, liveDialTimeout)
	defer cancel()
	client, err := cosmos.Connect(ctx, ch.RPC.URL, ch.GRPCURL, ch.CosmosChainID, ch.SignerKey)
	if err != nil {
		return errDial
	}
	_ = client.Close()
	return nil
}
