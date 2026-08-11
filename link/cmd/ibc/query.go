// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var cmdQuery = &cobra.Command{
	Use:     "query",
	Short:   "Query commands",
	Aliases: []string{"q"},
	RunE:    queryRun,
}

func queryRun(_ *cobra.Command, _ []string) error {
	fmt.Println("Querying something...")

	return nil
}
