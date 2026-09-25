<!-- SPDX-License-Identifier: Apache-2.0 -->

# Auditing the reference pages

Read this when you are checking whether the three pages are *true*, not whether
they are *current*. The two are different questions and only one of them is
automated.

`refgen.py all --check` compares each page against what the generator read. It
passes whenever the page matches that reading — including when the reading was
wrong. Every defect found in the September audit was in the reading, and
`--check` exited 0 over all of them. So an audit that consults the generator
learns nothing: it only hears the generator's own answer a second time.

## The brief

Four rules. They are the whole method.

1. **Ignore the generator.** Do not read `refgen.py`, do not run `--check`, do
   not reason about what a plan entry would say. You are auditing its output as
   a stranger would.
2. **Ignore the marked regions' provenance.** A generated table is not exempt.
   It is the most likely place for a wrong answer to be, because nobody reads
   it by hand.
3. **Read the source.** Not the doc comment — the code the comment describes.
   A comment is an author's claim; the surrounding statements are the program.
4. **Ask the binary.** Where a claim is behavioural — required, defaulted,
   accepted, enabled — build it and make it answer. Remove the key and run
   `config validate`. Pass the value and see whether it is refused. An English
   reading of a doc comment is not evidence about behaviour.

## The question that finds defects

For each claim on the page, ask:

> **Is the question the page answers the question the reader is asking?**

Every defect the audit found was the same shape — the generator answering a
narrower question, correctly, and the page reading as if it had answered the
reader's:

| The page said | Which was true of | But the reader asks |
|---|---|---|
| `ics26Router` is optional | a config file that parses and validates | whether the relayer will *run* without it |
| `clearOnStart` defaults to `true` | the named constant in the source | what happens when you leave the key out |
| `params` is a `json.RawMessage` | the Go field's type | what you write in the yaml |

None of the three was a false statement. All three misled. A claim can be
literally correct and still be a defect, and this is the only check that catches
that class — no test and no `--check` rule can, because the generator is
answering its own question correctly in each case.

## Citations

Every `<!-- [path/to/file.go: Symbol] -->` is a claim that *this symbol is where
that sentence comes from*. Check each one by opening the symbol and reading it.

`--check` verifies the symbol **is declared at that path**. It cannot verify it
is the right symbol for the sentence, and in the September audit 5 of 18 pointed somewhere unrelated while
the prose above them was true — a uniqueness rule cited to `DBConfig.Validate`,
an RPC's behaviour cited to a timeout constant, a fallback list cited to the
wrong file. Existence is cheap; relevance is the thing.

When the sentence is true because of a constant rather than the field that
carries it, cite the constant. "Server reflection is always enabled" is true
because `rpcEnableReflection` is `true`, not because a field exists.

## Per-page traps

**[5-configuration.md](../5-configuration.md)** — The required column and the
Type column are both probed, so a wrong entry there means the probe asked the
wrong thing, not that a word was misread. Check what "required" was scoped to:
required to parse, to validate, or to run. Two claims on this page are
*conventional*, not enforced — the signer split — and the program will not
confirm them; confirm them against the operator docs instead, and expect them
to stay unconfirmable.

**[6-cli-commands.md](../6-cli-commands.md)** — The command tree and flags come
from the binary, so they are as true as the build. The prose around them does
not. When grepping the page for a flag, match both forms: a pattern requiring
`` | `-- `` silently skips every short-form flag, which is how a "missing flag"
gets reported that was never missing.

**[7-api.md](../7-api.md)** — Generated from the descriptor set, so the field
lists and types are the compiler's. What is *not* the compiler's is any prose
about what a service does with a request. Check those sentences against the
handler.

## What an audit hands back

Per finding: the claim, the source that decides it, and whether it is *wrong*
or *narrow*. Narrow findings are the valuable ones and the easy ones to talk
yourself out of — record them.

A finding that the generator cannot express belongs in the same list, marked as
such, with the cost of fixing it. `params.url` reaching no row is one of these:
known, recorded, not worth a day.

Nothing about this procedure is a numbered step in
[AGENTS.md](AGENTS.md). It is not per-pull-request work — it is what you do
when you want to know whether the pages are true, which is a different and
rarer occasion.
