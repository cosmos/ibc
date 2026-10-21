<!-- SPDX-License-Identifier: Apache-2.0 -->

# CLI commands

- Use the single `init()` in `main.go` to wire `cmd*` subcommands; do not add per-file `init()`
  functions.
- Put global flags such as `--home` and `--config` in `config.FlagSet`, registered by
  `config.DeclarePersistentFlags`.
