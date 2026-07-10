// Package rpcsafe keeps resolved RPC URLs out of the stub's output. A chain RPC URL can carry a
// resolved ${NAME} secret (basic-auth userinfo or an API-key query parameter), and conventions §7
// forbids any secret from reaching a log/artifact. Lower layers (go-ethereum's http transport) embed
// the full URL in their error strings, so the stub sanitizes at its output boundary.
package rpcsafe

import (
	"net/url"
	"regexp"
)

// Endpoint reduces a URL to scheme://host, dropping userinfo, path and query — enough to identify the
// endpoint for debugging without leaking a credential. Unparseable input yields "<redacted>".
func Endpoint(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "<redacted>"
	}
	return u.Scheme + "://" + u.Host
}

// urlRE matches an embedded endpoint up to the first whitespace or quote, as endpoints appear inside
// go-ethereum / net-http error strings. It covers any `scheme://…` form.
var urlRE = regexp.MustCompile(`[a-z][a-z0-9+.-]*://[^\s"'` + "`" + `]+`)

// RedactURLs rewrites every endpoint embedded in s to its safe Endpoint form, so an error string bubbled
// up from a lower layer can be logged without leaking a secret carried in the URL.
func RedactURLs(s string) string {
	return urlRE.ReplaceAllStringFunc(s, Endpoint)
}
