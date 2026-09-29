// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

func parseEndpoint(raw string, schemes ...string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(schemes, parsed.Scheme) || parsed.Hostname() == "" {
		return nil, fmt.Errorf("must be a %s:// URL with a host", strings.Join(schemes, ":// or "))
	}
	if strings.HasSuffix(parsed.Host, ":") {
		return nil, fmt.Errorf("port must not be empty")
	}
	if port := parsed.Port(); port != "" {
		n, parseErr := strconv.Atoi(port)
		if parseErr != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("port must be between 1 and 65535")
		}
	}
	if strings.Contains(raw, "#") {
		return nil, fmt.Errorf("must not contain a fragment")
	}
	return parsed, nil
}
