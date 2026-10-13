// SPDX-License-Identifier: Apache-2.0

package network

import "crypto/tls"

// Endpoint is a remote service address paired with its TLS settings. A nil
// TLS leaves the transport's defaults: an https URL still uses TLS, with
// system roots, while a gRPC target dials plaintext.
type Endpoint struct {
	URL string
	TLS *tls.Config
}
