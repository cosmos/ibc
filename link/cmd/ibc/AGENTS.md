# CLI development (located in cmd/ibc/)

1. Use a single `init()` function in main.go to wire all subcommands. Never create init() per CLI file.
2. Commands implemented in this package should be variables and start with a `cmd*` prefix. The config,
   relayer, and test-app families are constructed by importable command packages because their handlers
   are selected at the composition root. Examples for commands that remain here:
 - `ibc attestor run` corresponds to `var cmdAttestorRun = ...`;
 - `ibc migrate up` corresponds to `var cmdMigrateUp = ...`;
3. Command implementations in this package should use `func commandName(cmd *cobra.Command, args []string) error`.
   Importable command packages may pass command-owned option structs to injected handlers so flags stay
   local to the command constructor. Be consistent within each command family.
4. Global CLI flags such as `--home` or `--config` belong in `config.FlagSet`. Add new global flags there, and don't use a separate variable if it can be global.
