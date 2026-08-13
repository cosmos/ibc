// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/service/signer"
	"github.com/cosmos/ibc/link/keyfile"
)

var (
	flagKeysShowPrivate      bool
	flagKeysImportPrivateKey string
	flagKeysPopulateConfig   bool
)

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

	cmdKeysImport = &cobra.Command{
		Use:   "import [type] [name]",
		Short: "Import a signing key",
		Long:  "Import a private key into <ibc-home>/keys/<name>",
		Args:  cobra.ExactArgs(2),
		RunE:  keysImport,
	}

	cmdKeysShow = &cobra.Command{
		Use:   "show [name]",
		Short: "Show key details",
		Long:  "Show key details from <ibc-home>/keys/<name>; optionally print the private key",
		Args:  cobra.ExactArgs(1),
		RunE:  keysShow,
	}

	cmdKeysList = &cobra.Command{
		Use:   "list",
		Short: "List all registered keys",
		Long:  "Lists every key from <ibc-home>/keys/",
		RunE:  keysList,
	}
)

//nolint:goconst // cli usage
func keysNew(_ *cobra.Command, args []string) error {
	globalFlags.SkipConfigValidation()

	cfg, err := setupHomeWithConfig()
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
		if flagKeysPopulateConfig {
			return fmt.Errorf("--populate-config requires a key name")
		}

		// for ephemeral keys we print the key to stdout including the private key
		return printKey(key, true, nil)
	}

	keyPath, err := signer.KeyFilePath(globalFlags.Home, args[1])
	if err != nil {
		return err
	}

	if err := key.StoreToFile(keyPath); err != nil {
		switch {
		case !os.IsExist(err):
			return err
		case !flagKeysPopulateConfig:
			return fmt.Errorf("key already exists: %s", keyPath)
		}

		// --populate-config: an already-existing key is fine, load what's
		// actually on disk instead of erroring
		key, err = signer.LocalKeyFromFile(keyPath)
		if err != nil {
			return err
		}
	}

	if flagKeysPopulateConfig {
		if err := addSignerToConfig(cfg, args[1]); err != nil {
			return err
		}
	}

	// note that we don't print the private key here to avoid leaking it to the user
	return printKey(key, false, map[string]any{
		"path": keyPath,
	})
}

func keysShow(_ *cobra.Command, args []string) error {
	globalFlags.SkipConfigValidation()

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

	return printKey(key, flagKeysShowPrivate, map[string]any{
		"path": keyPath,
	})
}

func keysList(_ *cobra.Command, _ []string) error {
	globalFlags.SkipConfigValidation()

	_, err := setupHomeWithConfig()
	if err != nil {
		return err
	}

	keyPath, err := config.ExpandHome(filepath.Join(globalFlags.Home, "keys"))
	if err != nil {
		return err
	}

	keys, err := signer.LocalKeysFromDirectory(keyPath)
	if err != nil {
		return err
	}

	out := make([]map[string]any, 0, len(keys))

	for _, key := range keys {
		out = append(out, map[string]any{
			"name": key.Name(),
			"type": key.Type(),
			"path": key.Path,
		})
	}

	return config.PrintJSON(out)
}

func keysImport(_ *cobra.Command, args []string) error {
	globalFlags.SkipConfigValidation()

	cfg, err := setupHomeWithConfig()
	if err != nil {
		return err
	}

	keyType, err := signer.ParseKeyType(args[0])
	switch {
	case err != nil:
		return err
	case keyType == keyfile.EDDSA:
		return fmt.Errorf("importing eddsa keys is not supported")
	}

	keyPath, err := signer.KeyFilePath(globalFlags.Home, args[1])
	if err != nil {
		return err
	}

	_, err = os.Stat(keyPath)
	switch {
	case err == nil && !flagKeysPopulateConfig:
		return fmt.Errorf("key already exists: %s", keyPath)
	case err == nil:
		// --populate-config: reuse the existing keystore entry instead of
		// erroring, no need to re-supply --private-key for it
		key, loadErr := signer.LocalKeyFromFile(keyPath)
		if loadErr != nil {
			return loadErr
		}
		if configErr := addSignerToConfig(cfg, args[1]); configErr != nil {
			return configErr
		}
		return printKey(key, false, map[string]any{
			"path": keyPath,
		})
	case !os.IsNotExist(err):
		return err
	}

	if flagKeysImportPrivateKey == "" {
		return fmt.Errorf("--private-key is required")
	}

	privateKey, err := signer.DecodeHex(flagKeysImportPrivateKey)
	if err != nil {
		return fmt.Errorf("decode private key: %w", err)
	}

	key, err := signer.NewLocalSecp256k1Signer(privateKey)
	if err != nil {
		return fmt.Errorf("create ecdsa key: %w", err)
	}

	if err := key.StoreToFile(keyPath); err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("key already exists: %s", keyPath)
		}
		return err
	}

	if flagKeysPopulateConfig {
		if err := addSignerToConfig(cfg, args[1]); err != nil {
			return err
		}
	}

	return printKey(key, false, map[string]any{
		"path": keyPath,
	})
}

// addSignerToConfig appends a local signer entry for the given key to the
// config file, so a freshly created/imported key is immediately usable as a
// `signers:` alias without a manual edit.
func addSignerToConfig(cfg config.Config, alias string) error {
	if _, ok := cfg.Signer(alias); ok {
		return nil
	}

	cfg.Signers = append(cfg.Signers, config.SignerConfig{
		Alias: alias,
		Type:  config.SignerLocal,
		File:  alias,
	})

	configPath, err := globalFlags.ConfigPath()
	if err != nil {
		return err
	}

	return cfg.StoreToFileWithComments(configPath)
}

func printKey(key signer.LocalKey, showPrivate bool, extra map[string]any) error {
	kv := map[string]any{
		"type":      key.Type(),
		"publicKey": toHex(key.PublicKey()),
	}

	if key.Type() == keyfile.ECDSA {
		addr, err := signer.PublicKeyToEVMAddress(key.PublicKey())
		if err != nil {
			return err
		}

		kv["evmAddress"] = addr
	}

	if showPrivate {
		pk := key.PrivateKey()
		kv["privateKey"] = toHex(pk)
		kv["privateKeyBase64"] = toBase64(pk)
	}

	for k, v := range extra {
		kv[k] = v
	}

	return config.PrintJSON(kv)
}

func toHex(b []byte) string {
	return "0x" + hex.EncodeToString(b)
}

func toBase64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}
