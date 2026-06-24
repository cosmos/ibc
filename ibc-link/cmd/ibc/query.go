package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	cmdQuery = &cobra.Command{
		Use:   "query",
		Short: "Query commands",
		Aliases: []string{"q"},
		RunE:  queryRun,
	}
)

func queryRun(cmd *cobra.Command, args []string) error {
	fmt.Println("Querying something...")

	return nil
}
