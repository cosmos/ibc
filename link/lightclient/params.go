// SPDX-License-Identifier: Apache-2.0

package lightclient

import (
	"github.com/goccy/go-yaml"
	"github.com/pkg/errors"
)

// RawParams stores client-specific configuration for a ProverFactory.
type RawParams struct {
	raw []byte
}

// NewRawParams wraps an encoded YAML document.
func NewRawParams(raw []byte) *RawParams {
	return &RawParams{raw: append([]byte(nil), raw...)}
}

// UnmarshalYAML captures the params block without interpreting it.
func (p *RawParams) UnmarshalYAML(b []byte) error {
	if p == nil {
		return errors.New("lightclient: UnmarshalYAML on nil RawParams")
	}

	p.raw = append([]byte(nil), b...)

	return nil
}

// MarshalYAML writes the captured block back out unchanged.
func (p RawParams) MarshalYAML() ([]byte, error) {
	return p.raw, nil
}

// IsEmpty reports whether params were configured.
func (p *RawParams) IsEmpty() bool {
	return p == nil || len(p.raw) == 0
}

// Decode strictly unmarshals params into v. Empty params leave v unchanged.
func (p *RawParams) Decode(v any) error {
	if p.IsEmpty() {
		return nil
	}

	if err := yaml.UnmarshalWithOptions(p.raw, v, yaml.DisallowUnknownField()); err != nil {
		return errors.Wrap(err, "decoding params")
	}

	return nil
}

// Bytes returns the captured document.
func (p *RawParams) Bytes() []byte {
	if p == nil {
		return nil
	}

	return p.raw
}
