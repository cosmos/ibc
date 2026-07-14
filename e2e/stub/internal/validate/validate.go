// Package validate implements `ibc config validate`; structural errors exit 64, unreachable --live RPC exits 65.
package validate

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/e2e/internal/harness/ibclink/wire"
	"github.com/cosmos/ibc/e2e/stub/internal/cfg"
	"github.com/cosmos/ibc/e2e/stub/internal/exitcode"
)

const (
	dbURLPath       = "db.url"
	liveDialTimeout = 5 * time.Second
)

func Command(flags *cfg.FlagSet) *cobra.Command {
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

func run(cmd *cobra.Command, flags *cfg.FlagSet, live bool) error {
	c, err := cfg.Setup(flags)
	if err != nil {
		path, pathErr := flags.ConfigPath()
		if pathErr != nil {
			path = flags.Config
		}
		_ = cfg.PrintJSON(wire.ValidateResult{
			Valid:    false,
			Warnings: []string{},
			Errors:   []wire.ValidationError{{Path: path, Msg: err.Error()}},
		})
		return exitcode.New(wire.ExitConfigInvalid, fmt.Errorf("config invalid: %w", err))
	}

	if errs := Check(c); len(errs) > 0 {
		_ = cfg.PrintJSON(wire.ValidateResult{Valid: false, Warnings: []string{}, Errors: errs})
		return exitcode.New(wire.ExitConfigInvalid, fmt.Errorf("config invalid: %d structural error(s)", len(errs)))
	}

	chains := resolvedChainIDs(c)

	if live {
		if errs := pingChains(cmd.Context(), c); len(errs) > 0 {
			// Valid structure but unreachable RPC: Valid:true on stdout, exit 65 (not 64).
			_ = cfg.PrintJSON(
				wire.ValidateResult{Valid: true, ResolvedChains: chains, Warnings: []string{}, Errors: errs},
			)
			return exitcode.New(
				wire.ExitRPCUnreachable,
				fmt.Errorf("live check failed: %d chain(s) unreachable", len(errs)),
			)
		}
	}

	_ = cfg.PrintJSON(wire.ValidateResult{Valid: true, ResolvedChains: chains, Warnings: []string{}})
	return nil
}

func Check(c *wire.ConfigYAML) []wire.ValidationError {
	var errs []wire.ValidationError
	signerAliases := map[string]bool{}
	for i, configured := range c.Signers {
		path := fmt.Sprintf("signers[%d]", i)
		switch {
		case configured.Alias == "":
			errs = append(errs, wire.ValidationError{Path: path + ".alias", Msg: "signer alias is empty"})
		case signerAliases[configured.Alias]:
			errs = append(errs, wire.ValidationError{
				Path: path + ".alias",
				Msg:  fmt.Sprintf("duplicate signer alias %q", configured.Alias),
			})
		default:
			signerAliases[configured.Alias] = true
		}
		switch configured.Type {
		case wire.SignerTypeLocal:
			if configured.File == "" {
				errs = append(errs, wire.ValidationError{
					Path: path + ".file",
					Msg:  "local signer file is empty",
				})
			}
		case "":
			errs = append(errs, wire.ValidationError{Path: path + ".type", Msg: "signer type is empty"})
		default:
			errs = append(errs, wire.ValidationError{
				Path: path + ".type",
				Msg: fmt.Sprintf(
					"unsupported signer type %q (stub supports %q)",
					configured.Type,
					wire.SignerTypeLocal,
				),
			})
		}
	}

	chainIDs := map[string]bool{}
	chainTypes := map[string]string{}

	for i, ch := range c.Chains {
		if ch.RPC.URL == "" {
			errs = append(errs, wire.ValidationError{
				Path: fmt.Sprintf("chains[%d].rpc.url", i),
				Msg:  "rpc url is empty",
			})
		}
		switch ch.Type {
		case wire.ChainTypeEVM:
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
		if ch.EVMSigner != "" && !signerAliases[ch.EVMSigner] {
			errs = append(errs, wire.ValidationError{
				Path: fmt.Sprintf("chains[%d].evmSigner", i),
				Msg:  fmt.Sprintf("EVM relay signer alias %q names no configured signer", ch.EVMSigner),
			})
		}
		if ch.TestAppSigner != "" && !signerAliases[ch.TestAppSigner] {
			errs = append(errs, wire.ValidationError{
				Path: fmt.Sprintf("chains[%d].testAppSigner", i),
				Msg:  fmt.Sprintf("test-app signer alias %q names no configured signer", ch.TestAppSigner),
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
	}

	if err := wire.ValidateDB(c.DB); err != nil {
		errs = append(errs, wire.ValidationError{Path: dbURLPath, Msg: err.Error()})
	}

	routeIDs := map[string]bool{}
	routeEndpoints := map[string]bool{}
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

		if route.Type != wire.RouteEVMToEVMAttested {
			errs = append(errs, wire.ValidationError{
				Path: path + ".type",
				Msg:  fmt.Sprintf("unknown route type %q", route.Type),
			})
		}
		for _, endpoint := range []string{route.Source, route.Destination} {
			if chainIDs[endpoint] && chainTypes[endpoint] == wire.ChainTypeEVM {
				routeEndpoints[endpoint] = true
			}
		}
	}
	for i, ch := range c.Chains {
		if !routeEndpoints[ch.ID] || ch.Type != wire.ChainTypeEVM {
			continue
		}
		if ch.EVMSigner == "" {
			errs = append(errs, wire.ValidationError{
				Path: fmt.Sprintf("chains[%d].evmSigner", i),
				Msg:  "EVM relay signer alias is empty",
			})
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
				Msg:  fmt.Sprintf("rpc unreachable (%s): %s", ch.RPC.URL, err),
			})
			continue
		}
		if ch.ChainID != 0 && got != ch.ChainID {
			errs = append(errs, wire.ValidationError{
				Path: fmt.Sprintf("chains[%d].chainId", i),
				Msg: fmt.Sprintf(
					"chain id mismatch (%s): config declares %d, node reports %d",
					ch.RPC.URL,
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
		return 0, fmt.Errorf("dial RPC %q: %w", rawURL, err)
	}
	defer client.Close()

	id, err := client.ChainID(ctx)
	if err != nil {
		return 0, fmt.Errorf("read chain ID from RPC %q: %w", rawURL, err)
	}
	return id.Uint64(), nil
}
