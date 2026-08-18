// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"

	"github.com/cosmos/ibc/link/cli"
)

func main() {
	os.Exit(cli.Execute(cli.NewRootCmd(cli.Options{})))
}
