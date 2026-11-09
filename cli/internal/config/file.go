// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/pkg/errors"

	ibcyaml "github.com/cosmos/ibc/cli/internal/yaml"
)

func LoadFromFile(path string, validate bool) (Config, error) {
	config := DefaultConfig()

	bz, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	// locate env references, so writing the config back keeps them instead of their values
	envPaths, err := ibcyaml.FindEnvPaths(bz)
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
	config.envPaths = envPaths

	return config, nil
}

// EnvPaths list all config paths that contain env references.
func (c Config) EnvPaths() []ibcyaml.EnvPath {
	return slices.Clone(c.envPaths)
}

// FilterEnvPaths keeps only the paths that match the filter.
// Useful to preserve rendered values of non-secret fields the program sets.
func (c Config) FilterEnvPaths(filter func(ibcyaml.EnvPath) bool) Config {
	var filtered []ibcyaml.EnvPath

	for _, path := range c.envPaths {
		if filter(path) {
			filtered = append(filtered, path)
		}
	}

	c.envPaths = filtered

	return c
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
	bz, err := c.asFileBytes(nil)
	if err != nil {
		return err
	}

	_, err = out.Write(bz)
	return err
}

// WriteYAMLWithComments writes c as YAML with comments attached.
func (c Config) WriteYAMLWithComments(out io.Writer, comments map[string]string) error {
	bz, err := c.asFileBytes(comments)
	if err != nil {
		return err
	}

	_, err = out.Write(bz)
	return err
}

// asFileBytes renders c as YAML, putting env var references from the loaded file back
// in place of their expanded values so secrets are not written out. See FilterEnvPaths.
func (c Config) asFileBytes(comments map[string]string) ([]byte, error) {
	tree, err := ibcyaml.MaskEnvPaths(c, c.envPaths)
	if err != nil {
		return nil, err
	}

	return yaml.MarshalWithOptions(tree, yaml.WithComment(toCommentMap(comments)))
}

func (c Config) store(path string, comments map[string]string) error {
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

	if err := c.WriteYAMLWithComments(tmp, comments); err != nil {
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
