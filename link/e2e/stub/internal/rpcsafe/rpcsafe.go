// Package rpcsafe redacts RPC URLs from errors; resolved ${NAME} secrets must not reach logs/stdout.
package rpcsafe

import (
	"net/url"
	"regexp"
)

func Endpoint(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "<redacted>"
	}
	return u.Scheme + "://" + u.Host
}

var urlRE = regexp.MustCompile(`[a-z][a-z0-9+.-]*://[^\s"'` + "`" + `]+`)

func RedactURLs(s string) string {
	return urlRE.ReplaceAllStringFunc(s, Endpoint)
}
