// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/cosmos/ibc/link/config"
	"github.com/cosmos/ibc/link/internal/fsutil"
)

func printJSON(v any) error {
	bz, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(bz))
	return nil
}

func printProtoJSON(msg proto.Message) error {
	bz, err := protojson.MarshalOptions{Indent: "  ", UseProtoNames: true, EmitUnpopulated: true}.Marshal(msg)
	if err != nil {
		return err
	}
	fmt.Println(string(bz))
	return nil
}

func printYAMLWithComments(v any, comments map[string]string) error {
	bz, err := config.MarshalYAMLWithComments(v, comments)
	if err != nil {
		return err
	}
	fmt.Println(string(bz))
	return nil
}

func writeConfigFile(path string, cfg config.Config, comments map[string]string) error {
	if err := fsutil.EnsureDirectory(path); err != nil {
		return err
	}

	bz, err := config.MarshalYAMLWithComments(cfg, comments)
	if err != nil {
		return err
	}

	return os.WriteFile(path, bz, 0o644)
}
