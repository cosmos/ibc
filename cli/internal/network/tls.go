// SPDX-License-Identifier: Apache-2.0

package network

import (
	"crypto/tls"
	"net/http"
	"net/url"
)

// Endpoint is a remote service address paired with its TLS settings. A nil
// TLS leaves the transport's defaults: an https URL still uses TLS, with
// system roots, while a gRPC target dials plaintext.
type Endpoint struct {
	URL string
	TLS *tls.Config
}

// NewGRPCHTTPClient returns a client for connect's gRPC protocol to endpoint.
// gRPC needs HTTP/2. Over TLS it is negotiated, with HTTP/1.1 also offered for
// servers that serve gRPC over it, such as connect-go. Plaintext has no
// negotiation and Go would always pick HTTP/1.1 there, so it is h2c only.
func NewGRPCHTTPClient(endpoint Endpoint) *http.Client {
	protocols := new(http.Protocols)

	if u, err := url.Parse(endpoint.URL); err == nil && u.Scheme == "https" {
		protocols.SetHTTP1(true)
		protocols.SetHTTP2(true)
	} else {
		protocols.SetUnencryptedHTTP2(true)
	}

	return &http.Client{Transport: &http.Transport{Protocols: protocols, TLSClientConfig: endpoint.TLS}}
}
