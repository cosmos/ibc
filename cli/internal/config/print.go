package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/goccy/go-yaml"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// PrintJSON prints anything as JSON to stdout.
func PrintJSON(v any) error {
	return printJSON(os.Stdout, v)
}

// PrintYAML prints anything as YAML to stdout.
func PrintYAML(v any) error {
	bz, err := yaml.Marshal(v)
	if err != nil {
		return err
	}

	fmt.Println(string(bz))

	return nil
}

// PrintYAMLWithComments prints v as YAML to stdout, attaching a line comment
// to every field addressed by a YAML path in comments, keyed as
// "$.relayer.connections[0].clientA.signer".
func PrintYAMLWithComments(v any, comments map[string]string) error {
	bz, err := yaml.MarshalWithOptions(v, yaml.WithComment(toCommentMap(comments)))
	if err != nil {
		return err
	}

	fmt.Println(string(bz))

	return nil
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
