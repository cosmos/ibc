package main

import (
	"context"
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/relay/proofgen"
	"github.com/cosmos/ibc/link/internal/service/attestor"
	"github.com/cosmos/ibc/link/internal/service/signer"
	"github.com/cosmos/ibc/link/internal/store"
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
)

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
		if err := store.ValidateConfigLive(cfg); err != nil {
			return errors.Wrap(err, "config live validation")
		}

		clientSet, err := chains.NewClientSetFromConfig(cfg)
		if err != nil {
			return errors.Wrap(err, "building chain clients for live validation")
		}

		if errLive := chains.ValidateConnectionsLive(cmd.Context(), cfg, clientSet); errLive != nil {
			return errors.Wrap(errLive, "connection live validation")
		}

		var localAttestors *attestor.Service
		if len(cfg.Attestors.Locals()) > 0 {
			signers, errSigners := signerSetFor(cmd.Context(), cfg)
			if errSigners != nil {
				return errors.Wrap(errSigners, "building signers for live validation")
			}

			localAttestors, err = attestor.NewFromConfig(cfg, clientSet, signers)
			if err != nil {
				return errors.Wrap(err, "building local attestors for live validation")
			}
		}

		if _, err := proofgen.NewSetFromConfig(cmd.Context(), cfg, clientSet, localAttestors); err != nil {
			return errors.Wrap(err, "attestor quorum live validation")
		}
	}

	// todo: it still logs store's log, we need to add config.logging{} params
	// to truly suppress logging (in followup PRs)
	if !globalFlags.Quiet {
		return config.PrintJSON(map[string]any{"status": "valid"})
	}

	return nil
}

// signerSetFor builds a signer.Set from config, needed to stand up local
// attestors for live validation.
func signerSetFor(ctx context.Context, cfg config.Config) (*signer.Set, error) {
	set := signer.NewSet()

	for _, signerConfig := range cfg.Signers {
		s, alias, err := signer.NewSignerFromConfig(ctx, signerConfig)
		if err != nil {
			return nil, err
		}

		set.Set(alias, s)
	}

	return set, nil
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
