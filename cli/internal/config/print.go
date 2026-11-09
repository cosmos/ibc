// SPDX-License-Identifier: Apache-2.0

package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// PrintJSON prints anything as JSON to stdout.
func PrintJSON(v any) error {
	return printJSON(os.Stdout, v)
}

func printJSON(out io.Writer, v any) error {
	if msg, ok := v.(proto.Message); ok {
		return printProtoJSON(out, msg)
	}

	bz, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(out, string(bz))

	return err
}

func printProtoJSON(out io.Writer, msg proto.Message) error {
	opts := protojson.MarshalOptions{
		Indent:          "  ",
		UseProtoNames:   false,
		EmitUnpopulated: true,
	}

	bz, err := opts.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(out, string(bz))

	return err
}
