<!-- SPDX-License-Identifier: Apache-2.0 -->

# docs/6-ibc-cli tooling

Three reference pages carry tables generated from this repository. The prose
around those tables is written by hand. `refgen.py` regenerates the tables and
never writes prose.

| Page | Covers | Generated from |
|------|--------|----------------|
| [5-configuration.md](../5-configuration.md) | every key in `ibc.yml` | the config structs in `link/internal/config/` |
| [6-cli-commands.md](../6-cli-commands.md) | every command and flag | the built binary's `--help` tree, plus `link/cmd/ibc/main.go` |
| [7-api.md](../7-api.md) | both gRPC services | `proto/link/relayer.proto`, `proto/link/attestor.proto` |

## Before you run anything

- Run every command from the repository root.
- A **Go toolchain is required**. Generating the CLI page runs
  `go build -o bin/ibc ./cmd/ibc/...` and reads the binary's `--help` tree,
  because flag defaults are only honest once the binary is assembled.
- `python3`, no packages to install.

## The one rule

**Never edit between a `<!-- GEN:... START -->` and its `END`.** Those blocks
are rewritten from source, so a hand edit there disappears the next time
anyone regenerates. It is a change that looks like a fix and is not.

Everything else on those pages is yours: prose, headings, section order,
examples.

## When the CLI, config, or protos change

Work through this in order. Steps 3 and 4 are where the generator will refuse
to proceed until a person decides something.

**1. See whether anything drifted.**

```sh
python3 docs/6-ibc-cli/tools/refgen.py all --check
```

Exits non-zero and prints a diff per stale region. If it is quiet, stop; there
is nothing to do.

**2. Get the work list before touching anything.**

```sh
python3 docs/6-ibc-cli/tools/refgen.py all --plan
```

JSON, side-effect free, classifying every gap into four kinds:

| Kind | Meaning | Who fixes it |
|------|---------|--------------|
| `stale` | a table no longer matches the source | step 5 does it |
| `missing_marker` | the source has a region the page has nowhere to put, with a suggested heading and insertion point | you, step 4 |
| `orphaned_marker` | a marker for a region the source no longer has | you, step 4 |
| `curation` | a choice the code cannot express: an undocumented key or field, a command group missing from the order list, a stale fingerprint | you, step 3 |

**3. Resolve every `curation` item first.** The generator refuses to write
while any remain, and it names each one. See the table below for which
constant to edit.

**4. Add or remove markers for `missing_marker` and `orphaned_marker`.** For a
missing one, add the heading and its `START`/`END` pair at the suggested point.
For an orphaned one, delete the marker pair and the prose that only existed to
introduce it.

**5. Heal the tables.**

```sh
python3 docs/6-ibc-cli/tools/refgen.py all
```

**6. Re-read the prose beside everything that moved.** This is the step that
gets skipped and the one that matters.

```sh
python3 docs/6-ibc-cli/tools/refgen.py all --list-regions
```

Regenerating repairs tables and touches no sentence next to them, so a page
can be green and still wrong. A removed flag leaves the example that used it.
A changed default leaves the paragraph calling the key required. A new key
arrives with no sentence saying when to reach for it. The region ids are your
list of sections to re-read.

**7. Run the tests**, below.

**8. Check the page renders.** Confirm no marker pair got orphaned and no
table landed under the wrong heading.

To work on one page instead of all three, pass the kind and the page:

```sh
python3 docs/6-ibc-cli/tools/refgen.py config docs/6-ibc-cli/5-configuration.md --check
```

The page argument is required unless the kind is `all`.

## Changing what the tables contain

The generator holds the choices a schema cannot make, and it fails loudly
rather than guessing. Edit `docs/6-ibc-cli/tools/refgen.py`:

| To change | Edit |
|-----------|------|
| the order command groups appear in | `CLI_SECTION_ORDER`. Membership is discovered; a new group missing from the list raises |
| commands left off the page entirely | `CLI_EXCLUDED`, currently `completion` and `help` |
| the description of a config key the Go source does not document | `FALLBACK_DOCS`. Each entry carries a fingerprint, so a key that gains a real doc comment raises rather than keeping the copy here |
| the description of a proto field the `.proto` does not document | `FIELD_DOCS`, same fingerprint rule |
| where a pointer field's default comes from | `DEFAULT_CONSTS`, which names the constant in the code rather than repeating its value |
| a pointer field that has no named default | `NO_NAMED_DEFAULT`, so a new one cannot quietly read as optional |
| a config field to leave out | `SKIP_FIELDS`, currently empty |
| which services the API page covers, and their display names | `SERVICES` |
| where a generated page lives | `PAGES` |

Any of these left half-done stops the generator, with a message naming what is
missing. A command group missing from the order list, a config key with no
description, a region with no marker: each refuses to write.

## Tests

```sh
python3 docs/6-ibc-cli/tools/test-refgen.py       # the marker engine and the extraction
python3 docs/6-ibc-cli/tools/test-refgen-e2e.py   # mutates a copy of the source 24 ways
```

The second is the one that matters when changing the generator. It copies the
source, breaks it twenty-four ways, and asserts one of three outcomes each
time: check goes red and regenerating heals it, the generator raises and names
the decision a person has to make, or the page is missing a marker for a region
that now exists. It is slower, because four cases rebuild the binary.

## What is not automated

**Nothing runs `--check` for you.** A currency workflow exists on the
`docs-reference-generation` branch and has not been merged, so today these
pages go stale silently and only this procedure catches it. Run step 1 as part
of any change under `link/`, `proto/`, or `gen/`.

## Why generated

A reference page's content already exists in the code. Anything retyped by
hand drifts from it silently, and the drift is invisible until a reader trusts
a wrong default. Generating the tables also repairs their line-numbered
citations, which rot on every refactor and which no path checker can catch.
