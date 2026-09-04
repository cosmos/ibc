// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/internal/livevalidate"
	"github.com/cosmos/ibc/cli/internal/pkg/logging"
)

// set in init()
var (
	// if true, perform extra validation checks like connecting to RPC,
	// checking EVM addresses, etc.
	flagConfigValidateLive bool

	// if true, output the config file to stdout
	flagConfigNewOut bool
)

var (
	cmdConfig = &cobra.Command{
		Use:   "config",
		Short: "Config commands",
	}

	cmdConfigNew = &cobra.Command{
		Use:     "new",
		Aliases: []string{"create", "init"},
		Short:   "Create new config file",
		RunE:    configNew,
	}

	cmdConfigValidate = &cobra.Command{
		Use:       "validate [target: relayer|attestor]",
		Short:     "Validate the config",
		Long:      configValidateLong,
		ValidArgs: []string{configTargetRelayer, configTargetAttestor},
		Args:      cobra.MatchAll(cobra.MaximumNArgs(1), cobra.OnlyValidArgs),
		RunE:      configValidate,
	}

	cmdConfigAddChain = &cobra.Command{
		Use:   "add-chain",
		Short: "Add a chain entry to the config",
		RunE:  configAddChain,
	}
)

const (
	configTargetRelayer  = "relayer"
	configTargetAttestor = "attestor"
)

const configValidateLong = `Validate config structure and cross-references.

Optional [target]:
  "relayer"   check fields required by "ibc relayer run"
  "attestor"  check fields required by "ibc attestor run"

Use --live for live components probe (database, RPC endpoints, etc...)`

var (
	flagConfigAddChainID       string
	flagConfigAddChainRPC      string
	flagConfigAddChainWS       string
	flagConfigAddChainRouter   string
	flagConfigAddChainDeployer string
)

func configAddChain(_ *cobra.Command, _ []string) error {
	globalFlags.SkipConfigValidation()

	cfg, err := setupHomeWithConfig()
	if err != nil {
		return err
	}

	if _, ok := cfg.Chain(flagConfigAddChainID); ok {
		return errors.Errorf("chain %q already exists in config", flagConfigAddChainID)
	}

	cfg.Chains = append(cfg.Chains, config.ChainConfig{
		ChainID: flagConfigAddChainID,
		EVM: &config.EVMChainConfig{
			RPC:         flagConfigAddChainRPC,
			WS:          flagConfigAddChainWS,
			ICS26Router: flagConfigAddChainRouter,
		},
		Deployer: flagConfigAddChainDeployer,
	})

	configPath, err := globalFlags.ConfigPath()
	if err != nil {
		return err
	}

	return cfg.StoreToFileWithComments(configPath)
}

func configNew(_ *cobra.Command, _ []string) error {
	configPath, err := globalFlags.ConfigPath()
	if err != nil {
		return err
	}

	cfg := config.DefaultConfig()

	if flagConfigNewOut {
		return config.PrintYAML(cfg)
	}

	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("config file %s already exists", configPath)
	}

	if err := cfg.StoreToFile(configPath); err != nil {
		return errors.Wrap(err, "unable to write file")
	}

	fmt.Printf("Config file created at %s\n", configPath)

	return nil
}

func configValidate(cmd *cobra.Command, args []string) error {
	// also calls cfg.Validate()
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return err
	}

	state := map[string]any{
		keyStatus: "valid",
		keyPath:   cfg.OriginalFilePath(),
	}

	// extra validation for the target
	if len(args) > 0 {
		target := args[0]
		switch target {
		case configTargetRelayer:
			err = cfg.RelayerSufficiency()
		case configTargetAttestor:
			err = cfg.AttestorSufficiency()
		default:
			err = errors.New("invalid target")
		}

		if err != nil {
			return errors.Wrap(err, target)
		}

		state[target] = "valid"
	}

	// todo replace with `ibc config probe`
	if flagConfigValidateLive {
		if err := livevalidate.Validate(cmd.Context(), cfg); err != nil {
			return err
		}
	}

	if globalFlags.Quiet {
		return nil
	}

	return config.PrintJSON(state)
}

// setupHomeWithConfig changes process directory to `--home` and parses the config
func setupHomeWithConfig() (config.Config, error) {
	home, err := config.ExpandHome(globalFlags.Home)
	if err != nil {
		return config.Config{}, errors.Wrap(err, "home")
	}

	slog.Debug("Using home", "home", home)

	configPath, err := globalFlags.ConfigPath()
	if err != nil {
		return config.Config{}, errors.Wrap(err, "unable to get config path")
	}

	// ensure --home exists
	if err = config.EnsureDirectory(configPath); err != nil {
		return config.Config{}, errors.Wrapf(err, "unable to create home directory %s", home)
	}

	if err = os.Chdir(home); err != nil {
		return config.Config{}, errors.Wrapf(err, "unable to change working directory to %s", home)
	}

	cfg, err := config.LoadFromFile(configPath, globalFlags.ValidateConfig())
	if err != nil {
		return config.Config{}, errors.Wrap(err, "unable to load the config")
	}

	if globalFlags.DB != "" {
		cfg.DB, err = config.DBConfigFromURL(globalFlags.DB)
		if err != nil {
			return config.Config{}, errors.Wrap(err, "invalid --db")
		}
	}

	// config logging applies per-field unless the flag set it explicitly
	json := globalFlags.LogJSON
	if !rootCmd.PersistentFlags().Changed("log-json") {
		json = cfg.Logging.JSON
	}

	level := globalFlags.LogLevel
	if !rootCmd.PersistentFlags().Changed("log-level") && cfg.Logging.Level != "" {
		level = cfg.Logging.Level
	}

	slog.SetDefault(logging.Default(json, level))

	return cfg, nil
}
