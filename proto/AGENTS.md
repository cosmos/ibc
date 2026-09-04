
## Documentation generated from this code

Three reference pages under `docs/6-ibc-cli/` are generated from this repository:
the configuration reference from the config structs, the CLI reference from the
command tree and the built binary's `--help`, and the API reference from the
protos. The prose around the tables is hand-written; the tables are not.

If you change a config struct, a command, a flag, or a proto, run:

```sh
python3 docs/6-ibc-cli/tools/refgen.py all --check   # is anything stale?
python3 docs/6-ibc-cli/tools/refgen.py all           # bring the tables up to date
```

Exit 1 means a page is stale and regenerating fixes it. Exit 2 means the generator
refused to build a page, and the message says what it could not read. Never
hand-edit between a `<!-- GEN:... START -->` marker and its `END`: those regions are
rewritten on the next run.

`docs/6-ibc-cli/tools/README.md` is the full guide, including what to do when a key
has no doc comment and when a hand-written description's fingerprint moves.
