package ibcrelay

import (
	"encoding/json"
	"fmt"
	"io"
)

func printIndentedJSON(w io.Writer, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	if _, err := fmt.Fprintln(w, string(data)); err != nil {
		return fmt.Errorf("write JSON: %w", err)
	}
	return nil
}
