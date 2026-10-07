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

func validateRPCEndpoint(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return errInvalidURL
	}
	// Match ethclient.Dial: scheme-less addresses are IPC paths, not URLs.
	if parsed.Scheme == "" || parsed.Scheme == "stdio" {
		return nil
	}
	_, err = parseEndpoint(raw, "http", "https", "ws", "wss")
	return err
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
