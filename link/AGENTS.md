# IBC Link Development Guide for AI Agents

1. Use `make lint-fix` to auto-format and lint code before finishing work.
2. YAML configuration should only contain `camelCase` keywords; never use `snake_case`.
3. Global CLI flags such as `--home` or `--config` belong in `config.FlagSet`. Add new global flags there, and don't use a separate variable if it can be global.

## CLI development (located in cmd/ibc/)

1. Use a single `init()` function in main.go to wire all subcommands. Never create init() per CLI file.
2. Every command should be a variable, just like other commands, and start with a `cmd*` prefix. Examples:
 - `ibc relayer run` corresponds to `var cmdRelayerRun = ...`;
 - `ibc config validate` corresponds to `var cmdConfigValidate = ...`;
3. Every command implementation should be a func of the form `commandName(cmd *cobra.Command, args []string) error`. Be consistent and rely on existing conventions. All CLI logic should look consistent.