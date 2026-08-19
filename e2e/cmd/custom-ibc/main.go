// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"

	"github.com/cosmos/ibc/e2e/internal/customlightclient"
	"github.com/cosmos/ibc/link/cli"
	"github.com/cosmos/ibc/link/lightclient"
	"github.com/cosmos/ibc/link/lightclient/remotepoc"
)

func main() {
	registry := lightclient.NewRegistry()
	if err := registry.Register(customlightclient.Factory{}); err != nil {
		panic(err)
	}
	if err := registry.Register(remotepoc.Factory{}); err != nil {
		panic(err)
	}

	root := cli.NewRootCmd(cli.Options{
		Relayer: cli.RelayerOptions{LightClients: registry},
	})
	os.Exit(cli.Execute(root))
}
