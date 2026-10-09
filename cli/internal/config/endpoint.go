// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// errInvalidURL deliberately omits the raw value: providers commonly embed an
// API key in the path, and url.Parse's own error echoes its whole input back.
var errInvalidURL = errors.New("invalid URL")

// endpointURL returns addr with a scheme, adding https:// to a bare host:port
// when a tls block is present and http:// otherwise.
func endpointURL(addr string, tls *TLSClientConfig) string {
	switch {
	case strings.Contains(addr, "://"):
		return addr
	case tls != nil:
		return "https://" + addr
	default:
		return "http://" + addr
	}
}

func parseEndpoint(raw string, schemes ...string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, errInvalidURL
	}
	if !slices.Contains(schemes, parsed.Scheme) || parsed.Hostname() == "" {
		prefixes := make([]string, len(schemes))
		for i, s := range schemes {
			prefixes[i] = s + "://"
		}
		return nil, fmt.Errorf("must start with %s and include a host", strings.Join(prefixes, " or "))
	}
	if port := parsed.Port(); port != "" {
		n, parseErr := strconv.Atoi(port)
		if parseErr != nil || n < 1 || n > 65535 {
			return nil, errors.New("port must be between 1 and 65535")
		}
	}
	return parsed, nil
}
