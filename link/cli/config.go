// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/livevalidate"
)

// set in init()
var (
	// if true, perform extra validation checks like connecting to RPC,
	// checking EVM addresses, etc.
	flagConfigValidateLive bool

	// if true, fail on unknown fields in the config file
	flagConfigValidateStrict bool

	// if true, output the config file to stdout
	flagConfigNewOut bool
)

var (
	cmdConfig = &cobra.Command{
		Use:              "config",
		Short:            "Config commands",
		PersistentPreRun: printConfigHome,
	}

	cmdConfigNew = &cobra.Command{
		Use:     "new",
		Aliases: []string{"create", "init"},
		Short:   "Create new config file",
		RunE:    configNew,
	}

	cmdConfigValidate = &cobra.Command{
		Use:   "validate",
		Short: "Validate the config",
		RunE:  configValidate,
	}

	cmdConfigAddChain = &cobra.Command{
		Use:   "add-chain",
		Short: "Add a chain entry to the config",
		RunE:  configAddChain,
	}
)

var (
	flagConfigAddChainID       string
	flagConfigAddChainRPC      string
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
		ChainID:  flagConfigAddChainID,
		EVM:      &config.EVMChainConfig{RPC: flagConfigAddChainRPC, ICS26Router: flagConfigAddChainRouter},
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

func configValidate(cmd *cobra.Command, _ []string) error {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return errors.Wrap(err, "setup home with config")
	}

	if flagConfigValidateLive {
		if err := livevalidate.Validate(cmd.Context(), cfg); err != nil {
			return err
		}
	}

	// todo: it still logs store's log, we need to add config.logging{} params
	// to truly suppress logging (in followup PRs)
	if !globalFlags.Quiet {
		return config.PrintJSON(map[string]any{useStatus: "valid"})
	}

	return nil
}

func printConfigHome(_ *cobra.Command, _ []string) {
	if !globalFlags.Quiet {
		fmt.Printf("Using home: %s\n", globalFlags.Home)
	}
}

// setupHomeWithConfig changes process directory to `--home` and parses the config
func setupHomeWithConfig() (config.Config, error) {
	home, err := config.ExpandHome(globalFlags.Home)
	if err != nil {
		return config.Config{}, errors.Wrap(err, "home")
	}

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

	cfg, err := config.LoadFromFile(configPath, globalFlags.ValidateConfig(), flagConfigValidateStrict)
	if err != nil {
		return config.Config{}, err
	}

	// allow db override
	if globalFlags.DB != "" {
		cfg.DB, err = config.DBConfigFromURL(globalFlags.DB)
		if err != nil {
			return config.Config{}, errors.Wrap(err, "invalid --db")
		}
	}

	return cfg, nil
}
