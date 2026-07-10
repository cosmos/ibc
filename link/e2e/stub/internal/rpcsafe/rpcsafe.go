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
// endpoint for debugging without leaking a credential. Unparseable input yields "<redacted>". A bare
// user:pass@host:port target (no scheme, as a gRPC dial string) parses with an empty scheme, so the
// scheme:// prefix is emitted only when a scheme is present.
func Endpoint(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "<redacted>"
	}
	if u.Scheme == "" {
		return u.Host
	}
	return u.Scheme + "://" + u.Host
}

// urlRE matches an embedded endpoint up to the first whitespace or quote, as endpoints appear inside
// go-ethereum / net-http / gRPC error strings. It covers any `scheme://…` form (not just http(s): the
// config also carries CometBFT `tcp://` endpoints) and a bare `user:pass@host…` userinfo form (a gRPC
// dial target with no scheme), so a credential in either shape is redacted.
var urlRE = regexp.MustCompile(
	`(?:[a-z][a-z0-9+.-]*://|[A-Za-z0-9._~%-]+:[^\s"'` + "`" + `@/]+@)[^\s"'` + "`" + `]+`,
)

// RedactURLs rewrites every endpoint embedded in s to its safe Endpoint form, so an error string bubbled
// up from a lower layer can be logged without leaking a secret carried in the URL.
func RedactURLs(s string) string {
	return urlRE.ReplaceAllStringFunc(s, func(match string) string {
		if schemeRE.MatchString(match) {
			return Endpoint(match)
		}
		// A bare userinfo form (user:pass@host:port) has no scheme; url.Parse needs one to populate Host,
		// so parse with a throwaway scheme and return just the host (dropping the userinfo credential).
		u, err := url.Parse("redact://" + match)
		if err != nil || u.Host == "" {
			return "<redacted>"
		}
		return u.Host
	})
}

// schemeRE reports whether a matched token already carries a `scheme://` prefix.
var schemeRE = regexp.MustCompile(`^[a-z][a-z0-9+.-]*://`)
