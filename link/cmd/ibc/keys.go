package main

import (
	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/service/signer"
)

var flagKeysShowPrivate bool

var (
	cmdKeys = &cobra.Command{
		Use:   "keys",
		Short: "Key management commands",
	}

	cmdKeysNew = &cobra.Command{
		Use:   "new [type] [name]",
		Short: "Create a new signing key",
		Long:  "Saves key into <ibc-home>/keys/<name> or prints to stdout if name is not provided",
		Args:  cobra.RangeArgs(1, 2),
		RunE:  keysNew,
	}

	cmdKeysShow = &cobra.Command{
		Use:   "show [name]",
		Short: "Show key details",
		Args:  cobra.ExactArgs(1),
		RunE:  keysShow,
	}
)

func keysNew(_ *cobra.Command, args []string) error {
	_, err := setupHomeWithConfig()
	if err != nil {
		return err
	}

	keyType, err := signer.ParseKeyType(args[0])
	if err != nil {
		return err
	}

	saveKey := len(args) > 1

	if !saveKey {
		return config.PrintJSON(map[string]any{
			"keyType": keyType,
			"stdout":  true,
			"pubKey":  "TODO",
		})
	}

	keyPath, err := signer.KeyFilePath(globalFlags.Home, args[1])
	if err != nil {
		return err
	}

	return config.PrintJSON(map[string]any{
		"keyType": keyType,
		"stdout":  true,
		"pubKey":  "TODO",
		"keyPath": keyPath,
	})
}

func keysShow(_ *cobra.Command, args []string) error {
	_, err := setupHomeWithConfig()
	if err != nil {
		return err
	}

	keyPath, err := signer.KeyFilePath(globalFlags.Home, args[0])
	if err != nil {
		return err
	}

	return config.PrintJSON(map[string]any{
		"stdout":  true,
		"pubKey":  "TODO",
		"keyPath": keyPath,
		"privKey": "TODO",
	})
}
