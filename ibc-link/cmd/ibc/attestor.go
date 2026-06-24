package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	cmdAttestor = &cobra.Command{
		Use:   "attestor",
		Short: "Attestor commands",
	}

	cmdAttestorRun = &cobra.Command{
		Use:   "run",
		Short: "Run the attestor",
		RunE:  attestorRun,
	}
)

func attestorRun(cmd *cobra.Command, args []string) error {
	fmt.Println("Running attestor...")

	return nil
}
