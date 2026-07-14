// Package jsonout emits one compact JSON line per write (HTML escaping off) for readiness and status API.
package jsonout

import (
	"encoding/json"
	"io"
)

func Write(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
