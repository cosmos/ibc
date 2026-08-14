// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"

	"github.com/goccy/go-yaml"
	"github.com/pkg/errors"
)

// LoadOptions controls config decoding and validation.
type LoadOptions struct {
	SkipValidation        bool
	DisallowUnknownFields bool
}

// LoadFile loads a config file after expanding environment variables.
func LoadFile(path string, opts LoadOptions) (Config, error) {
	config := Default()

	bz, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	expanded := os.ExpandEnv(string(bz))

	decodeOpts := []yaml.DecodeOption{}
	if opts.DisallowUnknownFields {
		decodeOpts = append(decodeOpts, yaml.DisallowUnknownField())
	}

	if err := yaml.UnmarshalWithOptions([]byte(expanded), &config, decodeOpts...); err != nil {
		return Config{}, err
	}

	if !opts.SkipValidation {
		if err := config.Validate(); err != nil {
			return Config{}, errors.Wrap(err, "validation failed")
		}
	}

	return config, nil
}

// MarshalYAML encodes v using Link's canonical YAML codec.
func MarshalYAML(v any) ([]byte, error) {
	return yaml.Marshal(v)
}

// MarshalYAMLWithComments encodes v with line comments keyed by YAML path.
func MarshalYAMLWithComments(v any, comments map[string]string) ([]byte, error) {
	cm := make(yaml.CommentMap, len(comments))
	for path, text := range comments {
		cm[path] = []*yaml.Comment{yaml.LineComment(" " + text)}
	}
	return yaml.MarshalWithOptions(v, yaml.WithComment(cm))
}
