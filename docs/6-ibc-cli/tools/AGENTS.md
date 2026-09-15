<!-- SPDX-License-Identifier: Apache-2.0 -->

# Updating the reference pages

You are here because something under `cli/`, `proto/` or `gen/` changed. Three
pages carry tables derived from that surface:

| Page | Covers | Derived from |
|------|--------|--------------|
| [5-configuration.md](../5-configuration.md) | every key in `ibc.yml` | the config structs in `cli/internal/config/` |
| [6-cli-commands.md](../6-cli-commands.md) | every command and flag | the built binary's command tree, plus the wiring in `cli/cmd/ibc/` |
| [7-api.md](../7-api.md) | both gRPC services | `proto/cli/*.proto` |

Other pages in `docs/6-ibc-cli/` -- the overview, the tutorial, the two
standalone guides -- mention the same commands and keys in hand-written prose
and examples. Nothing checks those. When you rename or remove something, grep
the whole directory for it; the generator will not tell you.

Do this **before the pull request is opened**, and put the documentation
changes in the same pull request as the code. You need `python3` and a Go
toolchain; generating the CLI page builds the binary and asks it, because flag
defaults are only honest once the binary is assembled. Run everything from the
repository root.

**The one rule: never edit between a `<!-- GEN:... START -->` and its `END`.**
Those blocks are rewritten from source, so an edit there disappears the next
time anyone regenerates — a change that looks like a fix and is not. Everything
else on those pages is yours: prose, headings, section order, examples.

## 1. Find out what moved

```sh
python3 docs/6-ibc-cli/tools/refgen.py all --check   # a diff per stale table
python3 docs/6-ibc-cli/tools/refgen.py all --plan    # the work order, as JSON
```

Neither one modifies a page. Both do build the CLI and run every one of its
commands to read what they require -- in a throwaway home directory, with a
timeout -- so they are not free, and they are not inert.

If `--check` is quiet and `--plan` reports no work, the tables are current. That
is not the same as the pages being right: see step 4.

`--plan` is the list to work from. It classifies every gap, and each entry
carries what you need to resolve it — including the file and line to go to:

| Kind | Meaning | What you do |
|------|---------|-------------|
| `stale` | a table no longer matches the source | nothing; step 3 heals it |
| `missing_marker` | the source has something the page has nowhere to put. Carries the rendered table, a suggested heading, and where to insert it | step 2 |
| `orphaned_marker` | a marker for something the source no longer has | step 2 |
| `curation` | a choice the source cannot express | step 2 |

Exit codes: `0` nothing to do, `1` something is stale or missing, `2` the tool
refused. A refusal outranks staleness, so `--plan` and `--report` still exit 2
when a page could not be read at all, and name it.

The `curation` list is where the specific kinds live -- `missing_description`,
`fingerprint_mismatch`, `unreadable_member` and a dozen more. Step 2 works
through the common ones. **A kind not named there is not an exception**: every
entry carries a `message` saying what is wrong and what would resolve it, and
the same rules apply — fix it at the source where you can, ask where the code
does not say, hand it back where it needs a decision that is not yours.

`--list-regions` prints just the stale region ids, one per line, which is
useful when you only want to know which prose to re-read.

## 2. Resolve everything that needs a decision

The generator will not write a page while any of these remain, and it names
each one.

**A page can be blocked by one item and still have other work you can do.** If
something on a page cannot be resolved without changing `refgen.py`, resolve
everything else on that page anyway: the descriptions, the markers, the
sections, the prose. Leaving them because the page will not regenerate yet is
how a whole page gets skipped over one line of protobuf.

**A key or field nobody has described** (`missing_description`). Fix it at the
source: add a doc comment to the declaration, which the plan names by file and
line. The page then documents itself and nothing has to be maintained in this
directory. Only when the declaration genuinely cannot be edited, add an entry
to `FALLBACK_DOCS` in `refgen.py` with the fingerprint the plan gives you.

**If the code does not say what something means, ask the person you are working
with.** They wrote it. A guess reads exactly like knowledge on the page, and
nobody downstream can tell the difference.

**A description whose source changed underneath it** (`fingerprint_mismatch`).
Re-read the sentence against the code at the named line, correct it if it is now
wrong, and record the new fingerprint.

**A section the page has nowhere to put** (`missing_marker`). Add the heading at
the suggested insertion point, then its `START`/`END` pair, then write the prose
that introduces it. The table itself comes from step 3.

**A marker whose subject is gone** (`orphaned_marker`). Delete the marker pair
and the prose that existed only to introduce it.

**Something the parser could not read** (`unreadable_declaration`,
`unreadable_member`, `unreadable_default`). The source grew a construct this
tool does not understand, and it refused rather than drop rows from a table —
because a short table reads exactly like a complete one. You may teach it the
construct. The bar is: after your change, the refusal is gone, both suites pass,
and the pages come out unchanged except where you meant them to change. Those
suites fail while the refusal stands -- that is what you are fixing -- so run
them *after*, not before, and judge the result then.

If you cannot reach that bar, stop and hand back the refusal with a proposed
patch. A parser change that reads the source wrongly fails silently, and nobody
reviewing a Go pull request will catch it.

Other choices the source cannot make live in `refgen.py`:

| To change | Edit |
|-----------|------|
| the order command groups appear in | `CLI_SECTION_ORDER`. Membership is discovered; a new group missing from the list refuses |
| commands left off the page entirely | `CLI_EXCLUDED`, currently `completion` and `help` |
| where a pointer field's default comes from | `DEFAULT_CONSTS`, which names the constant rather than repeating its value |
| a pointer field with no named default | `NO_NAMED_DEFAULT`, so a new one cannot quietly read as optional |
| a config field to leave out | `SKIP_FIELDS` |
| which services the API page covers | `SERVICES` |
| which program the pages document, if this repo ever holds two | `CLI_PACKAGE` |

## 3. Heal the tables

```sh
python3 docs/6-ibc-cli/tools/refgen.py all
```

Deterministic. Run it as often as you like.

## 4. Correct the prose the code has overtaken

Regenerating repairs a table and leaves the paragraph above it saying the old
thing. For every region that moved, read the prose beside it and fix what is now
wrong — a key that gained a default the text still calls required, an example
that no longer shows a useful invocation, a sentence describing a flag that is
gone. **The code is the authority.**

Writing prose here: short. Explain clearly and stop. Match the page you are
writing into — its sentence length, its person, its heading style. Say what the
thing is and when a reader reaches for it. Do not restate a table in words; the
table is already there, and two statements of one fact drift apart later.

## 5. Check your work before handing it back

```sh
python3 docs/6-ibc-cli/tools/refgen.py all --check   # must be clean
```

Then read each region you touched against the code it came from, once, by eye.
Running twice and changing nothing proves the tool is stable, not that it is
right.

That is the whole check **unless you edited `refgen.py`**. The two suites below
test the generator, not the documentation, and one of them takes many minutes
because it rebuilds the binary repeatedly. Running them after an ordinary
documentation change is waste.

```sh
# only if you changed refgen.py
python3 docs/6-ibc-cli/tools/test-refgen.py
python3 docs/6-ibc-cli/tools/test-refgen-e2e.py
```

Run each once. A suite that passes has told you what it knows; do not run it
again in another copy of the tree.

**The suites read the working tree, not a clean one.** So if the source holds
something the generator refuses to read -- anything you are handing back -- the
suites fail on that same refusal, and go on failing until it is resolved. That
is expected, and it is not yours to fix. Check that the failures name the
refusals you already know about, say so under *Handed back*, and move on. Do
not go looking for a version of the tree where they pass.

## 6. Report what you wrote

```sh
python3 docs/6-ibc-cli/tools/refgen.py all --report
```

Put it in the pull request description, and **add one line under *Written by
hand, not derived* for everything you wrote yourself** — prose, a doc comment, a
description — naming where it went.

That section is the point of the report. Everything above it is as true as the
code. Everything below it is only as true as you, and it is where the reviewer
should spend their attention. Keep it to one line per item.

## You are done when

`--check` is clean, or everything still outstanding is listed under *Handed
back* in the report. Nothing further is asked of you.

## If you cannot finish something

Some refusals need a change to `refgen.py`, and you should not make that change
unless you can keep both suites green and the pages unchanged except where you
meant them to change. When you cannot, **say so; do not go quiet.**

Add a *Handed back* section to the report, one line per item, naming what is
blocked, what it blocks, and what you would propose:

```
### Handed back

- `ChainConfig` declares an inline embedded struct the parser does not read,
  so 5-configuration.md cannot regenerate. Proposed: teach the parser to
  follow an inline embed, or tag the field `yaml:"-"` if readers never write
  it.
```

Unfinished and reported is a fine outcome. Unfinished and unmentioned is not:
whoever reads the pull request cannot tell a page you left alone from a page
that needed nothing.

## What you must never do

Write a heading with a placeholder under it. State a value the source does not
state. Delete a marker to make a refusal go away. Edit inside a generated
region.

**Never set `REFGEN_NO_REQUIRED_FLAGS`, `REFGEN_NO_REQUIRED_KEYS` or
`REFGEN_NO_BUILD`.** Two of those switch off the checks that catch a whole page
rendering every flag or key as optional, and the third generates the command
tables from a stale binary. The refusals mention them because a person who has
confirmed the CLI genuinely has no required flags needs a way through. You are
not that person: if you trip one, it is a hand-back.

A refusal is not an obstacle to route around. The tool refuses because it would
otherwise publish something it could not verify, and a wrong reference page is
worse than a missing one: a reader trusts a default, a required flag, a key
name, and acts on it.

## Not automated yet

**Nothing runs `--check` for you.** These pages go stale silently, and this
procedure is the only thing that catches it. Run step 1 whenever a change
touches `cli/`, `proto/` or `gen/`.
