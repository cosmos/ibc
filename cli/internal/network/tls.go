// SPDX-License-Identifier: Apache-2.0

package network

import (
	"crypto/tls"
	"net"
	"net/url"
	"strings"
)

// Endpoint is a remote service address paired with its TLS settings. TLS is
// set for an https:// URL and nil for anything else, which dials plaintext.
type Endpoint struct {
	URL string
	TLS *tls.Config
}

// GRPCTarget returns URL as a gRPC target. An http(s):// URL becomes its
// host:port, with 443 or 80 when the port is omitted; anything else, such as
// host:port or dns:///host:port, is returned as written.
func (e Endpoint) GRPCTarget() string {
	scheme, rest, ok := strings.Cut(e.URL, "://")
	if !ok {
		return e.URL
	}

	scheme = strings.ToLower(scheme)
	if scheme != "http" && scheme != "https" {
		return e.URL
	}

	u, err := url.Parse(scheme + "://" + rest)
	if err != nil || u.Host == "" {
		return e.URL
	}

	if u.Port() != "" {
		return u.Host
	}

	port := "80"
	if scheme == "https" {
		port = "443"
	}

	return net.JoinHostPort(u.Hostname(), port)
}
