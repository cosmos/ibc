package ibcrelay

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/link/cmd/configcmd"

	internalconfig "github.com/cosmos/ibc/link/internal/config"
)

const (
	dbURLPath       = "db.url"
	liveDialTimeout = 5 * time.Second
)

// ConfigValidate returns the config-validation handler for the IBC relayer.
func ConfigValidate(flags *internalconfig.FlagSet) configcmd.ValidateHandler {
	return func(cmd *cobra.Command, _ []string, options configcmd.ValidateOptions) error {
		return runConfigValidate(cmd, flags, options.Live)
	}
}

func runConfigValidate(cmd *cobra.Command, flags *internalconfig.FlagSet, live bool) error {
	c, err := setupConfig(flags)
	if err != nil {
		path, pathErr := flags.ConfigPath()
		if pathErr != nil {
			path = flags.Config
		}
		_ = printIndentedJSON(cmd.OutOrStdout(), configcmd.ValidateResult{
			Valid:    false,
			Warnings: []string{},
			Errors:   []configcmd.ValidationError{{Path: path, Msg: err.Error()}},
		})
		return fmt.Errorf("config invalid: %w", err)
	}

	if errs := checkConfig(c); len(errs) > 0 {
		_ = printIndentedJSON(
			cmd.OutOrStdout(),
			configcmd.ValidateResult{Valid: false, Warnings: []string{}, Errors: errs},
		)
		return fmt.Errorf("config invalid: %d structural error(s)", len(errs))
	}

	chains := resolvedChainIDs(c)

	if live {
		if errs := pingChains(cmd.Context(), c); len(errs) > 0 {
			_ = printIndentedJSON(
				cmd.OutOrStdout(),
				configcmd.ValidateResult{Valid: true, ResolvedChains: chains, Warnings: []string{}, Errors: errs},
			)
			return fmt.Errorf("live check failed: %d chain(s) unreachable", len(errs))
		}
	}

	_ = printIndentedJSON(
		cmd.OutOrStdout(),
		configcmd.ValidateResult{Valid: true, ResolvedChains: chains, Warnings: []string{}},
	)
	return nil
}

func checkConfig(c *configcmd.Config) []configcmd.ValidationError {
	var errs []configcmd.ValidationError
	signerAliases := map[string]bool{}
	for i, configured := range c.Signers {
		path := fmt.Sprintf("signers[%d]", i)
		switch {
		case configured.Alias == "":
			errs = append(errs, configcmd.ValidationError{Path: path + ".alias", Msg: "signer alias is empty"})
		case signerAliases[configured.Alias]:
			errs = append(errs, configcmd.ValidationError{
				Path: path + ".alias",
				Msg:  fmt.Sprintf("duplicate signer alias %q", configured.Alias),
			})
		default:
			signerAliases[configured.Alias] = true
		}
		switch configured.Type {
		case configcmd.SignerTypeLocal:
			if configured.File == "" {
				errs = append(errs, configcmd.ValidationError{
					Path: path + ".file",
					Msg:  "local signer file is empty",
				})
			}
		case "":
			errs = append(errs, configcmd.ValidationError{Path: path + ".type", Msg: "signer type is empty"})
		default:
			errs = append(errs, configcmd.ValidationError{
				Path: path + ".type",
				Msg: fmt.Sprintf(
					"unsupported signer type %q (ibcrelay supports %q)",
					configured.Type,
					configcmd.SignerTypeLocal,
				),
			})
		}
	}

	chainIDs := map[string]bool{}
	chainTypes := map[string]string{}

	for i, ch := range c.Chains {
		if ch.RPC.URL == "" {
			errs = append(errs, configcmd.ValidationError{
				Path: fmt.Sprintf("chains[%d].rpc.url", i),
				Msg:  "rpc url is empty",
			})
		}
		switch ch.Type {
		case configcmd.ChainTypeEVM:
			if err := validateICS26Router(ch.ICS26Router); err != nil {
				errs = append(errs, configcmd.ValidationError{
					Path: fmt.Sprintf("chains[%d].ics26Router", i),
					Msg:  err.Error(),
				})
			}
		case "":
			errs = append(errs, configcmd.ValidationError{
				Path: fmt.Sprintf("chains[%d].type", i),
				Msg:  "chain type is empty",
			})
		default:
			errs = append(errs, configcmd.ValidationError{
				Path: fmt.Sprintf("chains[%d].type", i),
				Msg:  fmt.Sprintf("unsupported chain type %q; expected %q", ch.Type, configcmd.ChainTypeEVM),
			})
		}
		if ch.EVMSigner != "" && !signerAliases[ch.EVMSigner] {
			errs = append(errs, configcmd.ValidationError{
				Path: fmt.Sprintf("chains[%d].evmSigner", i),
				Msg:  fmt.Sprintf("EVM relay signer alias %q names no configured signer", ch.EVMSigner),
			})
		}
		if ch.ID == "" {
			errs = append(
				errs,
				configcmd.ValidationError{Path: fmt.Sprintf("chains[%d].id", i), Msg: "chain id is empty"},
			)
			continue
		}
		if chainIDs[ch.ID] {
			errs = append(errs, configcmd.ValidationError{
				Path: fmt.Sprintf("chains[%d].id", i),
				Msg:  fmt.Sprintf("duplicate chain id %q", ch.ID),
			})
			continue
		}
		chainIDs[ch.ID] = true
		chainTypes[ch.ID] = ch.Type
	}

	if err := configcmd.ValidateDB(c.DB); err != nil {
		errs = append(errs, configcmd.ValidationError{Path: dbURLPath, Msg: err.Error()})
	}

	routeIDs := map[string]bool{}
	routeEndpoints := map[string]bool{}
	for i, route := range c.Relayer.Routes {
		path := fmt.Sprintf("relayer.routes[%d]", i)
		switch {
		case route.ID == "":
			errs = append(errs, configcmd.ValidationError{Path: path + ".id", Msg: "route id is empty"})
		case routeIDs[route.ID]:
			errs = append(errs, configcmd.ValidationError{
				Path: path + ".id",
				Msg:  fmt.Sprintf("duplicate route id %q", route.ID),
			})
		default:
			routeIDs[route.ID] = true
		}
		if !chainIDs[route.Source] {
			errs = append(errs, configcmd.ValidationError{
				Path: path + ".source",
				Msg:  fmt.Sprintf("source %q names no defined chain", route.Source),
			})
		}
		if !chainIDs[route.Destination] {
			errs = append(errs, configcmd.ValidationError{
				Path: path + ".destination",
				Msg:  fmt.Sprintf("destination %q names no defined chain", route.Destination),
			})
		}
		if route.SourceClient == "" {
			errs = append(errs, configcmd.ValidationError{
				Path: path + ".sourceClient",
				Msg:  "sourceClient is empty",
			})
		}
		if route.DestClient == "" {
			errs = append(errs, configcmd.ValidationError{
				Path: path + ".destClient",
				Msg:  "destClient is empty",
			})
		}

		if route.Type != configcmd.RouteEVMToEVMAttested {
			errs = append(errs, configcmd.ValidationError{
				Path: path + ".type",
				Msg:  fmt.Sprintf("unknown route type %q", route.Type),
			})
		}
		for _, endpoint := range []string{route.Source, route.Destination} {
			if chainIDs[endpoint] && chainTypes[endpoint] == configcmd.ChainTypeEVM {
				routeEndpoints[endpoint] = true
			}
		}
	}
	for i, ch := range c.Chains {
		if !routeEndpoints[ch.ID] || ch.Type != configcmd.ChainTypeEVM {
			continue
		}
		if ch.EVMSigner == "" {
			errs = append(errs, configcmd.ValidationError{
				Path: fmt.Sprintf("chains[%d].evmSigner", i),
				Msg:  "EVM relay signer alias is empty",
			})
		}
	}
	return errs
}

func validateICS26Router(addr string) error {
	switch {
	case addr == "":
		return fmt.Errorf("ics26Router is empty")
	case !strings.HasPrefix(addr, "0x") || !common.IsHexAddress(addr):
		return fmt.Errorf("ics26Router must be 0x-prefixed 20-byte hex")
	default:
		return nil
	}
}

func resolvedChainIDs(c *configcmd.Config) []string {
	ids := make([]string, 0, len(c.Chains))
	for _, ch := range c.Chains {
		ids = append(ids, ch.ID)
	}
	return ids
}

func pingChains(ctx context.Context, c *configcmd.Config) []configcmd.ValidationError {
	var errs []configcmd.ValidationError
	for i, ch := range c.Chains {
		got, err := pingOne(ctx, ch.RPC.URL)
		if err != nil {
			errs = append(errs, configcmd.ValidationError{
				Path: fmt.Sprintf("chains[%d].rpc.url", i),
				Msg:  fmt.Sprintf("rpc unreachable (%s): %s", ch.RPC.URL, err),
			})
			continue
		}
		if ch.ChainID != 0 && got != ch.ChainID {
			errs = append(errs, configcmd.ValidationError{
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
