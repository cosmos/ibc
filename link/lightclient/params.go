// SPDX-License-Identifier: Apache-2.0

package lightclient

import (
	"github.com/goccy/go-yaml"
	"github.com/pkg/errors"
)

// RawParams is a client-type-specific config block captured verbatim at decode
// time and interpreted by the ProverFactory registered for that type, not by the
// config package.
//
// It is used as a pointer in config structs so that adding it does not make
// those structs non-comparable.
type RawParams struct {
	raw []byte
}

// NewRawParams builds params from an already-encoded YAML document. Mainly
// useful in tests.
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

// IsEmpty reports whether no params were configured. A ProverFactory that requires
// params should check this and return a helpful error rather than decoding an
// empty document into a zero value.
func (p *RawParams) IsEmpty() bool {
	return p == nil || len(p.raw) == 0
}

// Decode unmarshals the params into v, rejecting fields v does not declare.
//
// Rejecting unknown fields matters: the top-level config decode cannot see
// inside a captured block, so without this a misspelled key would silently
// become a zero value. Factories should always decode through this method
// rather than unmarshalling p.Bytes() themselves.
//
// Decoding empty params is a no-op, leaving v at its defaults.
func (p *RawParams) Decode(v any) error {
	if p.IsEmpty() {
		return nil
	}

	if err := yaml.UnmarshalWithOptions(p.raw, v, yaml.DisallowUnknownField()); err != nil {
		return errors.Wrap(err, "decoding params")
	}

	return nil
}

// Bytes returns the captured document. Prefer Decode, which applies strict
// field checking.
func (p *RawParams) Bytes() []byte {
	if p == nil {
		return nil
	}

	return p.raw
}
