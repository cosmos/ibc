// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/pkg/errors"
)

type yamlScalar interface {
	~string | ~bool | ~int | ~uint | ~int64
}

func LoadFromFile(path string, validate bool) (Config, error) {
	config := DefaultConfig()

	bz, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	// ast tree
	originalYAML, err := parser.ParseBytes(bz, parser.ParseComments)
	if err != nil {
		return Config{}, err
	}

	opts := []yaml.DecodeOption{
		yaml.DisallowUnknownField(),
		yaml.CustomUnmarshaler(unmarshalScalarWithEnv[string]),
		yaml.CustomUnmarshaler(unmarshalScalarWithEnv[ChainType]),
		yaml.CustomUnmarshaler(unmarshalScalarWithEnv[AttestorType]),
		yaml.CustomUnmarshaler(unmarshalScalarWithEnv[bool]),
		yaml.CustomUnmarshaler(unmarshalScalarWithEnv[int]),
		yaml.CustomUnmarshaler(unmarshalScalarWithEnv[uint]),
		yaml.CustomUnmarshaler(unmarshalScalarWithEnv[time.Duration]),
	}

	if err := unmarshalConfigAST(originalYAML, &config, opts); err != nil {
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

func unmarshalConfigAST(file *ast.File, config *Config, opts []yaml.DecodeOption) error {
	if len(file.Docs) != 1 || file.Docs[0].Body == nil {
		return errors.New("expected a single config YAML document")
	}

	return yaml.NodeToValue(file.Docs[0].Body, config, opts...)
}

// decodes yaml's fields w/ or w/o env vars like `hello: ${HELLO}`
func unmarshalScalarWithEnv[S yamlScalar](value *S, data []byte) error {
	// peek to detect if the value is an env var w/o writing to *S
	var tmp string
	if err := yaml.Unmarshal(data, &tmp); err != nil {
		return err
	}

	if expanded := os.ExpandEnv(tmp); expanded != tmp {
		data = []byte(expanded)
	}

	return yaml.Unmarshal(data, value)
}

func storeConfig(c Config, path string, comments map[string]string) error {
	if err := EnsureDirectory(path); err != nil {
		return err
	}

	latest, err := yaml.ValueToNode(c)
	if err != nil {
		return err
	}

	if c.originalYAML != nil {
		if err = preserveConfigEnv(latest, c.originalYAML); err != nil {
			return err
		}
	}

	bz, err := yaml.MarshalWithOptions(latest, yaml.WithComment(toCommentMap(comments)))
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

// preserveConfigEnv restores env-backed values at their original YAML paths.
// Callers must preserve existing list order; appending entries is supported.
func preserveConfigEnv(latest ast.Node, original *ast.File) error {
	var value any

	decoder := yaml.NewDecoder(strings.NewReader(""))

	// Initialize the decoder with the whole document so aliases resolve too.
	if err := decoder.DecodeFromNode(original.Docs[0].Body, &value); err != nil {
		return err
	}

	for _, node := range ast.FilterFile(ast.MappingValueType, original) {
		entry := node.(*ast.MappingValueNode)
		if err := decoder.DecodeFromNode(entry.Value, &value); err != nil {
			return err
		}

		raw, ok := value.(string)
		if !ok {
			continue
		}

		hasEnv := false
		os.Expand(raw, func(string) string { hasEnv = true; return "" })
		if !hasEnv {
			continue
		}

		parent, err := yaml.PathString(entry.GetPath()[:strings.LastIndex(entry.GetPath(), ".")])
		if err != nil {
			return err
		}

		patch, err := yaml.ValueToNode(map[string]string{entry.Key.GetToken().Value: raw})
		if err != nil {
			return err
		}

		file := &ast.File{
			Docs: []*ast.DocumentNode{
				{Body: latest},
			},
		}

		// Merging one field also restores placeholders omitted by omitempty.
		if err := parent.MergeFromNode(file, patch); err != nil {
			return err
		}
	}

	return nil
}
