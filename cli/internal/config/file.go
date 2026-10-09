// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/pkg/errors"
)

func LoadFromFile(path string, validate bool) (Config, error) {
	config := DefaultConfig()

	bz, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	// substitute ENV variables
	expanded := os.ExpandEnv(string(bz))

	opts := []yaml.DecodeOption{
		yaml.DisallowUnknownField(),
	}

	if err := yaml.UnmarshalWithOptions([]byte(expanded), &config, opts...); err != nil {
		return Config{}, err
	}

	if err := rejectNullTLS([]byte(expanded)); err != nil {
		return Config{}, err
	}

	if validate {
		if err := config.Validate(); err != nil {
			return Config{}, err
		}
	}

	config.originalFilePath = path

	return config, nil
}

// rejectNullTLS fails on a `tls:` key with no value. It decodes to the same
// nil as an absent block, so a block whose fields are all commented out would
// otherwise silently drop its settings, and for a bare host:port or a signer
// target, TLS itself. It walks the generically decoded document
// so quoted keys, tags and aliases resolve exactly as they do for the config.
func rejectNullTLS(bz []byte) error {
	var doc any
	if err := yaml.Unmarshal(bz, &doc); err != nil {
		return err
	}

	if path := findNullTLS(doc, ""); path != "" {
		return errors.Errorf(
			"%s: empty; use `tls: {}` for default TLS settings, or remove the key",
			path,
		)
	}

	return nil
}

// findNullTLS returns the path of the first null `tls` value under node, in
// key order so the result is deterministic.
func findNullTLS(node any, path string) string {
	switch n := node.(type) {
	case map[string]any:
		for _, key := range slices.Sorted(maps.Keys(n)) {
			child := key
			if path != "" {
				child = path + "." + key
			}

			if key == "tls" && n[key] == nil {
				return child
			}

			if found := findNullTLS(n[key], child); found != "" {
				return found
			}
		}
	case []any:
		for i, v := range n {
			if found := findNullTLS(v, fmt.Sprintf("%s[%d]", path, i)); found != "" {
				return found
			}
		}
	}

	return ""
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

func storeConfig(c Config, path string, comments map[string]string) error {
	if err := EnsureDirectory(path); err != nil {
		return err
	}

	bz, err := yaml.MarshalWithOptions(c, yaml.WithComment(toCommentMap(comments)))
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
	if _, err := tmp.Write(bz); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmp.Name(), path)
}
