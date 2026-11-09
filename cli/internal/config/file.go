// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/parser"
	"github.com/pkg/errors"

	ibcyaml "github.com/cosmos/ibc/cli/internal/yaml"
)

func LoadFromFile(path string, validate bool) (Config, error) {
	config := DefaultConfig()

	bz, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	// ast tree, will be used to preserve ENV vars when updating/printing
	originalYAML, err := parser.ParseBytes(bz, parser.ParseComments)
	if err != nil {
		return Config{}, err
	}

	// expand ENV vars
	expandedBytes := []byte(os.ExpandEnv(string(bz)))

	opts := []yaml.DecodeOption{
		yaml.DisallowUnknownField(),
	}

	if err := yaml.UnmarshalWithOptions(expandedBytes, &config, opts...); err != nil {
		return Config{}, err
	}

	if validate {
		if err := config.Validate(); err != nil {
			return Config{}, err
		}
	}

	config.originalFilePath = path
	config.originalYAML = originalYAML

	return config, nil
}

// KeyFileFallbacks returns the paths tried for a local signer key file.
func KeyFileFallbacks(keyPath string) []string {
	fallbacks := []string{keyPath}

	// absolute path, no fallbacks needed
	if filepath.IsAbs(keyPath) {
		return fallbacks
	}

	// forgot to add .json extension
	if !strings.HasSuffix(keyPath, ".json") {
		keyPath = fmt.Sprintf("%s.json", keyPath)

		fallbacks = append(fallbacks, keyPath)
	}

	// forgot to add keys/ directory
	if !strings.Contains(keyPath, "keys/") {
		keyPath = filepath.Join("keys", keyPath)

		fallbacks = append(fallbacks, keyPath)
	}

	return fallbacks
}

func (c Config) WriteYAML(out io.Writer) error {
	bz, err := c.asFileBytes(nil, nil)
	if err != nil {
		return err
	}

	_, err = out.Write(bz)
	return err
}

// WriteYAMLWithComments writes c as YAML with comments attached.
// See asFileBytes for forceUpdatePaths.
func (c Config) WriteYAMLWithComments(out io.Writer, comments map[string]string, forceUpdatePaths ...string) error {
	bz, err := c.asFileBytes(comments, forceUpdatePaths)
	if err != nil {
		return err
	}

	_, err = out.Write(bz)
	return err
}

// asFileBytes renders c as YAML, putting env var references from the loaded file back
// in place of their expanded values so secrets are not written out.
// Paths in forceUpdatePaths (e.g. `$.chains[*].evm.ics26Router`) keep their new non-empty value
// instead; use them only for non-secret values the program sets.
func (c Config) asFileBytes(comments map[string]string, forceUpdatePaths []string) ([]byte, error) {
	astFile, err := ibcyaml.ValueToFile(c)
	if err != nil {
		return nil, err
	}

	astResultFile, err := ibcyaml.Merge(astFile, c.originalYAML, ibcyaml.PreserveEnv(forceUpdatePaths...))
	switch {
	case err != nil:
		return nil, err
	case len(astResultFile.Docs) != 1 || astResultFile.Docs[0].Body == nil:
		return nil, fmt.Errorf("expected 1 document, got %d", len(astResultFile.Docs))
	}

	body := astResultFile.Docs[0].Body

	return yaml.MarshalWithOptions(body, yaml.WithComment(toCommentMap(comments)))
}

func (c Config) store(path string, comments map[string]string, forceUpdatePaths []string) error {
	err := EnsureDirectory(path)
	if err != nil {
		return err
	}

	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
		path, err = filepath.EvalSymlinks(path)
		if err != nil {
			return err
		}
	} else if !os.IsNotExist(statErr) {
		return statErr
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return err
	}

	defer func() { _ = os.Remove(tmp.Name()) }()

	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}

	if err := c.WriteYAMLWithComments(tmp, comments, forceUpdatePaths...); err != nil {
		_ = tmp.Close()
		return err
	}

	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmp.Name(), path)
}

func fileExistsInAny(path ...string) error {
	for _, p := range path {
		if err := fileExists(p); err == nil {
			return nil
		}
	}

	return errors.New("file not found")
}

func fileExists(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return errors.Errorf("path is a directory")
	}

	return nil
}
