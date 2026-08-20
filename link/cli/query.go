// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"github.com/spf13/cobra"
)

var cmdQuery = &cobra.Command{
	Use:     "query",
	Short:   "Query commands",
	Aliases: []string{"q"},
}
