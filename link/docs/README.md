<!-- SPDX-License-Identifier: Apache-2.0 -->

# link/docs

Reference documentation for the `ibc` binary. The tables on these pages are
generated from this repository; the prose around them is written by hand.

| Page | Covers | Generated from |
|------|--------|----------------|
| [configuration-generated.md](configuration-generated.md) | every key in `ibc.yml` | the config structs in `link/internal/config/` |
| [cli-commands.md](cli-commands.md) | every command and flag | the built binary's `--help` tree, plus `link/cmd/ibc/main.go` |
| [api.md](api.md) | both gRPC services | `proto/link/relayer.proto`, `proto/link/attestor.proto` |

## Editing a page

Edit it. Prose, headings, section order, examples: all normal markdown.

**One rule: never edit between a `<!-- GEN:... START -->` and its `END`.**
Those blocks are regenerated from source, so a hand edit there is reverted the
next time anyone regenerates. It is a change that looks like a fix and is not.

Then check that the markers still line up:

```sh
python3 link/docs/tools/refgen.py all --check
```

## When the code changes

A pull request touching `link/`, `proto/`, or `gen/` runs
[`.github/workflows/reference-currency.yml`](../../.github/workflows/reference-currency.yml),
which fails if a generated table no longer matches the code. It reports and
never rewrites anything. To fix a failure:

```sh
python3 link/docs/tools/refgen.py all --list-regions   # what moved
python3 link/docs/tools/refgen.py all                  # heal the tables
```

**Then read the prose around whatever moved.** Regenerating repairs the tables
and touches no sentence next to them, so a page can be green and still wrong:
a removed flag leaves the example that used it, a new default leaves the
paragraph that called the key required. The region ids from `--list-regions`
are the list of sections to re-read.

## Changing what the tables contain

The generator holds the choices a schema cannot make, and it fails loudly
rather than guessing. Edit `link/docs/tools/refgen.py`:

| To change | Edit |
|-----------|------|
| which task group a command appears under | `CLI_TASKS` |
| which commands get their own flag section | `CLI_FLAG_SECTIONS`, and add the page section with its markers |
| which config blocks get a table | `CONFIG_BLOCKS`, and add the page section with its markers |
| the description of a config key the Go source does not document | `FALLBACK_DOCS` |
| where a pointer field's default comes from | `DEFAULT_CONSTS` |

Any of these left half-done stops the generator, with a message naming what
is missing. A command in no task group, a config key with no description, a
region with no marker, a flag documented nowhere: each refuses to write.

## Tests

```sh
python3 link/docs/tools/test-refgen.py       # the marker engine and the extraction
python3 link/docs/tools/test-refgen-e2e.py   # mutates a copy of the source 15 ways
```

The second is the one that matters when changing the generator. It breaks the
source on purpose, fifteen ways, and asserts that a stale page goes red and
that each loud failure still fires. Removals are covered as well as additions.

## Why generated

A reference page's content already exists in the code. Anything retyped by
hand drifts from it silently, and the drift is invisible until a reader trusts
a wrong default. Generating the tables also repairs their line-numbered
citations, which rot on every refactor and which no path checker can catch.
