package main

import (
	"encoding/hex"

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

//nolint:goconst // cli usage
func keysNew(_ *cobra.Command, args []string) error {
	_, err := setupHomeWithConfig()
	if err != nil {
		return err
	}

	keyType, err := signer.ParseKeyType(args[0])
	if err != nil {
		return err
	}

	key, err := signer.GenerateLocalKey(keyType)
	if err != nil {
		return err
	}

	saveKey := len(args) > 1

	if !saveKey {
		// for ephemeral keys we print the key to stdout including the private key
		return config.PrintJSON(map[string]any{
			"keyType": keyType,
			"pubKey":  toHex(key.PublicKey()),
			"privKey": toHex(key.PrivateKey()),
		})
	}

	keyPath, err := signer.KeyFilePath(globalFlags.Home, args[1])
	if err != nil {
		return err
	}

	if err := key.StoreToFile(keyPath); err != nil {
		return err
	}

	// note that we don't print the private key here to avoid leaking it to the user
	return config.PrintJSON(map[string]any{
		"keyType":   keyType,
		"publicKey": toHex(key.PublicKey()),
		"keyPath":   keyPath,
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

	key, err := signer.LocalKeyFromFile(keyPath)
	if err != nil {
		return err
	}

	kv := map[string]any{
		"keyType":   key.Type(),
		"publicKey": toHex(key.PublicKey()),
		"keyPath":   keyPath,
	}

	if flagKeysShowPrivate {
		kv["privateKey"] = toHex(key.PrivateKey())
	}

	return config.PrintJSON(kv)
}

func toHex(b []byte) string {
	return "0x" + hex.EncodeToString(b)
}
