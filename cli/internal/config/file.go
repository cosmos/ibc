// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"io"
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
	bz, err := c.asFileBytes(nil)
	if err != nil {
		return err
	}

	_, err = out.Write(bz)
	return err
}

func (c Config) WriteYAMLWithComments(out io.Writer, comments map[string]string) error {
	bz, err := c.asFileBytes(comments)
	if err != nil {
		return err
	}

	_, err = out.Write(bz)
	return err
}

func (c Config) asFileBytes(comments map[string]string) ([]byte, error) {
	astFile, err := c.ast()
	if err != nil {
		return nil, err
	}

	// todo
	// replace rendered config values with original ENV variables to preserve secrets
	replaceIf := func(_, overlay *ast.Node) bool {
		return hasEnvValue(overlay)
	}

	astResultFile, err := astMerge(astFile, c.originalYAML, replaceIf)
	switch {
	case err != nil:
		return nil, err
	case len(astResultFile.Docs) != 1 || astResultFile.Docs[0].Body == nil:
		return nil, fmt.Errorf("expected 1 document, got %d", len(astResultFile.Docs))
	}

	body := astResultFile.Docs[0].Body

	return yaml.MarshalWithOptions(body, yaml.WithComment(toCommentMap(comments)))
}

func (c Config) ast() (*ast.File, error) {
	node, err := yaml.ValueToNode(c)
	if err != nil {
		return nil, err
	}

	return &ast.File{
		Docs: []*ast.DocumentNode{
			{Body: node},
		},
	}, nil
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

func astMerge(
	source, overlay *ast.File,
	condition func(original, latest *ast.Node) bool,
) (*ast.File, error) {
	// todo walk for each node and if condition is true, replace source with overlay
	return nil, nil
}

func hasEnvValue(node *ast.Node) bool {
	if node == nil {
		return false
	}

	// todo some magic to say: "ok this is node is [string, int, uint, float, time, ...] AND its value is ENV"
	// env means value contains ${var} or $var

	return false
}
