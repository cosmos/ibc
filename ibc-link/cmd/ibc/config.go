package main

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/ibc-link/config"
)

// set in init()
// if true, perform extra validation checks like connecting to RPC,
// checking EVM addresses, etc.
// todo: not implemented!
var flagValidateLive bool

var (
	cmdConfig = &cobra.Command{
		Use:   "config",
		Short: "Config commands",
		PersistentPreRun: func(_ *cobra.Command, _ []string) {
			fmt.Printf("Using home: %s\n", globalFlags.Home)
		},
	}

	cmdConfigNew = &cobra.Command{
		Use:   "new",
		Short: "Create new config file",
		RunE:  configNew,
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

	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("config file %s already exists", configPath)
	}

	config := config.DefaultConfig()

	if err := config.StoreToFile(configPath); err != nil {
		return errors.Wrap(err, "unable to write file")
	}

	fmt.Printf("Config file created at %s\n", configPath)

	return nil
}

func configValidate(_ *cobra.Command, _ []string) error {
	if flagValidateLive {
		return errors.New("--live is not implemented")
	}

	// todo resolve config file
	// todo validate config

	return nil
}
