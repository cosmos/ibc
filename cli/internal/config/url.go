// SPDX-License-Identifier: Apache-2.0

package config

import (
	"net/url"
	"slices"
	"strings"
)

// Schemes an endpoint may be reached over.
const (
	schemeHTTP  = "http"
	schemeHTTPS = "https"
	schemeWS    = "ws"
	schemeWSS   = "wss"
)

const schemeSeparator = "://"

// parseEndpoint parses raw as an absolute endpoint URL restricted to schemes,
// returning an error tagged with segment.
//
// It parses rather than prefix-matches so that a scheme in any case is
// accepted, since schemes are case-insensitive, and so that a URL with no host
// is rejected -- "https://" satisfies a prefix check but cannot be dialed.
// Path, query and fragment are left to the caller: a chain endpoint
// legitimately carries a provider API key in either, while an RPC base URL
// cannot.
func parseEndpoint(segment, raw string, schemes ...string) (*url.URL, error) {
	// A bare host:port parses inconsistently -- "host:9090" yields a nonsense
	// scheme while "127.0.0.1:9090" fails outright -- so name the missing
	// scheme up front and give both the same answer.
	if !strings.Contains(raw, schemeSeparator) {
		return nil, errPathf(segment, "must be a URL with scheme %s, got %q", schemeList(schemes), raw)
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, errPathf(segment, "invalid URL %q: %s", raw, err)
	}

	switch {
	case !slices.Contains(schemes, parsed.Scheme):
		return nil, errPathf(segment, "must be a URL with scheme %s, got %q", schemeList(schemes), raw)
	case parsed.Host == "":
		return nil, errPathf(segment, "must include a host, got %q", raw)
	}

	return parsed, nil
}

// schemeList renders schemes for an error message, as "ws:// or wss://".
func schemeList(schemes []string) string {
	rendered := make([]string, len(schemes))
	for i, scheme := range schemes {
		rendered[i] = scheme + schemeSeparator
	}

	return strings.Join(rendered, " or ")
}
