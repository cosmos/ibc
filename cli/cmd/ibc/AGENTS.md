<!-- SPDX-License-Identifier: Apache-2.0 -->

# CLI commands

- Use the single `init()` in `main.go` to wire subcommands; do not add per-file `init()` functions.
- Define commands implemented in this package as variables with a `cmd*` prefix.
- Package-local handlers use `func commandName(cmd *cobra.Command, args []string) error`.
- Put global flags such as `--home` and `--config` in `config.FlagSet`.
