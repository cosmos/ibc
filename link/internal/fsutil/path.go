// SPDX-License-Identifier: Apache-2.0

// Package fsutil contains shared filesystem path helpers.
package fsutil

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandHome converts a path beginning with ~ to an absolute path.
func ExpandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	if path == "~" {
		return home, nil
	}

	return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
}

// EnsureDirectory creates the parent directory of a file path when needed.
func EnsureDirectory(path string) error {
	dir := filepath.Dir(path)
	if dir == "." {
		return nil
	}

	return os.MkdirAll(dir, 0o755)
}

// KeyFileFallbacks returns the paths tried for a local signer key file.
func KeyFileFallbacks(keyPath string) []string {
	fallbacks := []string{keyPath}

	if filepath.IsAbs(keyPath) {
		return fallbacks
	}

	if !strings.HasSuffix(keyPath, ".json") {
		keyPath += ".json"
		fallbacks = append(fallbacks, keyPath)
	}

	if !strings.Contains(keyPath, "keys/") {
		fallbacks = append(fallbacks, filepath.Join("keys", keyPath))
	}

	return fallbacks
}
