<!-- SPDX-License-Identifier: Apache-2.0 -->

# CLI commands

- Use the single `init()` in `main.go` to wire subcommands; do not add per-file `init()` functions.
- Define commands implemented in this package as variables with a `cmd*` prefix. Importable command
  packages may construct command families whose transport types are shared outside this package.
- Package-local handlers use `func commandName(cmd *cobra.Command, args []string) error`. Importable
  command packages may pass command-owned option structs to injected handlers.
- Put global flags such as `--home` and `--config` in `config.FlagSet`.
