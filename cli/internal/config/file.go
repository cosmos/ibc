// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
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
// otherwise silently mean plaintext.
func rejectNullTLS(bz []byte) error {
	file, err := parser.ParseBytes(bz, 0)
	if err != nil {
		return err
	}

	var found nullTLSFinder
	for _, doc := range file.Docs {
		ast.Walk(&found, doc)
	}

	if found.path != "" {
		return errors.Errorf(
			"%s: empty; use `tls: {}` for TLS with default settings, or remove it for plaintext",
			strings.TrimPrefix(found.path, "$."),
		)
	}

	return nil
}

// nullTLSFinder records the path of the first null `tls` value it visits.
type nullTLSFinder struct {
	path string
}

func (f *nullTLSFinder) Visit(node ast.Node) ast.Visitor {
	if f.path != "" {
		return nil
	}

	if mv, ok := node.(*ast.MappingValueNode); ok && mv.Key.String() == "tls" && mv.Value.Type() == ast.NullType {
		f.path = mv.Value.GetPath()
		return nil
	}

	return f
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
