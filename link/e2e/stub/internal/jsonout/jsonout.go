// Package jsonout writes the stub's line-oriented machine-readable output: a single compact JSON value
// with HTML escaping disabled (so hex payloads and addresses survive verbatim) and a trailing newline.
// It is the writer for the streaming surfaces that must stay strictly line-oriented — the `relayer run`
// readiness line and the status API. The one-shot commands (`config validate`, `deploy`, the app
// actions) emit their result via the root module's config.PrintJSON instead; both are valid JSON the
// harness decodes, but they are distinct formatters and the two can differ on framing/escaping.
package jsonout

import (
	"encoding/json"
	"io"
)

// Write encodes v to w as one compact, newline-terminated JSON line with HTML escaping disabled.
func Write(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
