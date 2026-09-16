#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0

"""Tests for tools/refgen.py.

The generator writes into pages that mix hand-written prose with generated
tables. Two properties matter more than any formatting detail:

  1. Text outside a generated region is never modified.
  2. Running it twice changes nothing the second time.

Both have precedent: regex-based tooling over these pages destroyed 108
headings once and a page's citations another time. Everything else here is
ordinary coverage.

    python3 tools/test-refgen.py
"""
import os
import re
import sys
import tempfile
import traceback

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import refgen  # noqa: E402

PASS, FAIL = [], []


def case(name):
    def deco(fn):
        try:
            fn()
            PASS.append(name)
        except Exception as e:
            FAIL.append((name, e, traceback.format_exc()))
        return fn
    return deco


PAGE = """---
title: "API"
---

Hand-written intro that must survive untouched.

## Relayer service

{/* GEN:a START */}
stale content
{/* GEN:a END */}

Prose between two regions, also untouched. It has a `{/* citation */}` in it.

{/* GEN:b START */}
{/* GEN:b END */}

Trailing prose.
"""


@case("replaces only the region body")
def _():
    out = refgen.render(PAGE, {"a": "NEW A", "b": "NEW B"})
    assert "NEW A" in out and "NEW B" in out
    assert "stale content" not in out
    for keep in ("Hand-written intro that must survive untouched.",
                 "Prose between two regions, also untouched.",
                 "Trailing prose.", "`{/* citation */}`"):
        assert keep in out, keep


@case("text outside regions is byte-identical")
def _():
    out = refgen.render(PAGE, {"a": "X", "b": "Y"})
    assert refgen.outside(out, refgen.find_regions(out)) == \
           refgen.outside(PAGE, refgen.find_regions(PAGE))


@case("idempotent: second run is a no-op")
def _():
    once = refgen.render(PAGE, {"a": "X", "b": "Y"})
    twice = refgen.render(once, {"a": "X", "b": "Y"})
    assert once == twice


@case("markers themselves survive, so the page stays regenerable")
def _():
    out = refgen.render(PAGE, {"a": "X", "b": "Y"})
    ids = [i for i, *_ in refgen.find_regions(out)]
    assert ids == ["a", "b"], ids


@case("unclosed START is an error")
def _():
    try:
        refgen.find_regions("{/* GEN:a START */}\nbody\n")
    except refgen.MarkerError:
        return
    raise AssertionError("expected MarkerError")


@case("END without START is an error")
def _():
    try:
        refgen.find_regions("body\n{/* GEN:a END */}\n")
    except refgen.MarkerError:
        return
    raise AssertionError("expected MarkerError")


@case("nested regions are an error")
def _():
    try:
        refgen.find_regions("{/* GEN:a START */}{/* GEN:b START */}{/* GEN:b END */}{/* GEN:a END */}")
    except refgen.MarkerError:
        return
    raise AssertionError("expected MarkerError")


@case("mismatched START/END ids are an error")
def _():
    try:
        refgen.find_regions("{/* GEN:a START */}x{/* GEN:b END */}")
    except refgen.MarkerError:
        return
    raise AssertionError("expected MarkerError")


@case("duplicate region id is an error")
def _():
    try:
        refgen.find_regions("{/* GEN:a START */}x{/* GEN:a END */}{/* GEN:a START */}y{/* GEN:a END */}")
    except refgen.MarkerError:
        return
    raise AssertionError("expected MarkerError")


@case("a generated block with no marker on the page is an error")
def _():
    try:
        refgen.render(PAGE, {"a": "X", "b": "Y", "c": "orphan"})
    except refgen.MarkerError:
        return
    raise AssertionError("expected MarkerError")


@case("a marker the generator did not fill is an error, not a silent skip")
def _():
    try:
        refgen.render(PAGE, {"a": "X"})
    except refgen.MarkerError:
        return
    raise AssertionError("expected MarkerError")


@case("pipes in cell content are escaped")
def _():
    t = refgen.table(["A"], [["sqlite | postgres"]])
    assert r"sqlite \| postgres" in t


@case("newlines in cell content do not break the row")
def _():
    t = refgen.table(["A", "B"], [["one\ntwo", "x"]])
    assert len([l for l in t.split("\n") if l.startswith("|")]) == 3


@case("empty row set renders a placeholder, not a headerless table")
def _():
    assert refgen.table(["A"], []) == "_None._"


@case("proto parse: oneof becomes one field with its options")
def _():
    b = refgen.gen_api()
    t = b["api:msg:RelayRequest"]
    assert "oneof: `all_packets` or `selected_packets`" in t
    assert "`tx_hash`" in t and "`source_chain_id`" in t


@case("proto parse: an empty message closed on its own line does not swallow the next one")
def _():
    b = refgen.gen_api()
    # SelectedPackets follows `message AllPackets {}` in relayer.proto
    assert "`packets`" in b["api:msg:SelectedPackets"]
    assert "api:msg:AllPackets" not in b, "empty message should produce no table"


@case("proto parse: enum drops the UNSPECIFIED zero value")
def _():
    t = refgen.gen_api()["api:enum:PacketState"]
    assert "UNSPECIFIED" not in t
    assert "`PACKET_STATE_SUCCEEDED`" in t


@case("every generated block cites a real file, at a line that exists")
def _():
    # asserted against the source tree rather than a path prefix, so this
    # holds in the docs repo and in the copy that lives beside the code
    for gen in refgen.GENERATORS.values():
        for region, body in gen().items():
            if region == "notice":
                continue          # says what the page is, not what the code says
            cites = re.findall(r"\]\(([^)#]+)#L(\d+)", body)
            assert cites, f"{region} carries no citation"
            for path, line in cites:
                rel = path.split("repos/ibc/")[-1]
                full = os.path.join(refgen.IBC, rel)
                assert os.path.exists(full), f"{region}: {path} does not exist"
                assert int(line) <= len(open(full).read().split("\n")), \
                    f"{region}: {path}#L{line} is past the end of the file"


@case("proto parse: a one-line message with a field does not swallow the next")
def _():
    b = refgen.gen_api()
    # `message StateAttestationResponse { Attestation attestation = 1; }` is
    # followed by PacketAttestationRequest, whose fields the line-scanning
    # parser used to hand to the message above it
    assert "`attestation`" in b["api:msg:StateAttestationResponse"]
    assert "`packets`" not in b["api:msg:StateAttestationResponse"]
    assert "`packets`" in b["api:msg:PacketAttestationRequest"]


@case("proto parse: every message with fields gets a table")
def _():
    b = refgen.gen_api()
    for name in ("InfoRequest", "InfoResponse", "LatestHeightRequest",
                 "LatestHeightResponse", "PacketAttestationRequest"):
        assert f"api:msg:{name}" in b, name


@case("proto parse: an optional field is kept, with its own comment")
def _():
    t = refgen.gen_api()["api:msg:Attestation"]
    assert "`timestamp`" in t
    assert "The timestamp of the block |" in t
    assert "The timestamp of the block The attested data" not in t


@case("proto parse: fields keep their declaration order")
def _():
    rows = [r for r in refgen.gen_api()["api:msg:RelayRequest"].split("\n")
            if r.startswith("| `")]
    assert rows[0].startswith("| `tx_hash`"), rows
    assert "oneof" in rows[-1], rows


@case("a two-valued type escapes its pipe exactly once")
def _():
    row = [r for r in refgen.gen_config()["config:db"].split("\n") if r.startswith("| `type`")][0]
    assert r"`sqlite` \| `postgres`" in row, row
    assert r"\\|" not in row, row


@case("api: every RPC has its own region, so a new one cannot arrive unnoticed")
def _():
    b = refgen.gen_api()
    for rpc in ("Relay", "Packets", "StateAttestation", "PacketAttestation",
                "LatestHeight", "Info"):
        assert f"api:rpc:{rpc}" in b, rpc
    assert not [k for k in b if k.endswith(":rpcs")], "the summary tables are gone"


@case("api: no field table has an empty description")
def _():
    for region, body in refgen.gen_api().items():
        for row in (l for l in body.split("\n") if l.startswith("| `")):
            cells = [c.strip() for c in row.split("|")[1:-1]]
            if len(cells) == 3:
                assert cells[-1], f"{region}: {cells[0]} has no description"


@case("probe: a flag group is not read as a flag, and does not claim required")
def _():
    # cobra says `at least one of the flags in the group [alpha beta] is
    # required`. Reading that as a flag name produced a flag called
    # "alpha beta", which matches nothing -- so both real flags rendered
    # optional while the binary refused to run without one.
    for line in ("Error: at least one of the flags in the group [alpha beta] is required",
                 "Error: if any flags in the group [alpha beta] are set they must all be set"):
        hits = [rx.search(line) for rx in refgen._REQUIRED_ALSO]
        assert not any(hits), (line, hits)
        assert any(rx.search(line) for rx in refgen._FLAG_GROUP), line
    # a genuine single-flag message is still read
    assert refgen._REQUIRED_ALSO[0].search("Error: --chain is required")


@case("probe: only the error line is read, never the usage block under it")
def _():
    out = ("Error: accepts 1 arg(s), received 0\n"
           "Usage:\n  ibc keys show [name] [flags]\n"
           "Flags:\n      --private   required for remote signers\n")
    assert refgen._error_line(out) == "Error: accepts 1 arg(s), received 0"
    # the usage text below must not be mined for requirements
    assert not refgen._REQUIRED_ALSO[0].search(refgen._error_line(out))


@case("probe: a command whose answer is a floor is recorded as one")
def _():
    binary = refgen.build_cli()
    tree = refgen.walk_cli(binary)
    refgen.required_flags(binary, tree)
    # every command that reports required flags stops at the first value it
    # rejects, so each of them is a floor and the report has to say so
    assert refgen.INCOMPLETE_PROBES, "nothing recorded as incomplete"
    for path in refgen.INCOMPLETE_PROBES:
        assert path in tree, path


@case("plan: a description gap names the declaration to go and fix")
def _():
    key = ("ServerConfig", "listenAddr")
    saved = dict(refgen.FALLBACK_DOCS)
    try:
        del refgen.FALLBACK_DOCS[key]
        refgen.PLAN = []
        refgen.gen_config()
        gaps = [c for c in refgen.PLAN if c["kind"] == "missing_description"]
    finally:
        refgen.PLAN = None
        refgen.FALLBACK_DOCS.clear()
        refgen.FALLBACK_DOCS.update(saved)
    assert gaps, "no missing_description recorded"
    g = gaps[0]
    assert g["file"].endswith(".go"), g
    assert isinstance(g["line"], int) and g["line"] > 0, g
    line = refgen._read(g["file"]).split("\n")[g["line"] - 1]
    assert "ListenAddress" in line, (g, line)


@case("plan: a key that stops being required is named in the work order")
def _():
    # The worst diff this tool can produce and the least obvious: a validation
    # message reworded past the words `_requirement` knows turns a mandatory
    # key optional, and it reads as ordinary drift.
    page = ("<!-- GEN:config:db START -->\n"
            "| Key | Type | Default or required | Description |\n"
            "|---|---|---|---|\n"
            "| `url` | `string` | **required** | x. |\n"
            "| `type` | `string` | **required** | y. |\n"
            "<!-- GEN:config:db END -->\n")
    blocks = {"config:db": ("| Key | Type | Default or required | Description |\n"
                            "|---|---|---|---|\n"
                            "| `url` | `string` | optional | x. |\n"
                            "| `type` | `string` | **required** | y. |\n")}
    got = refgen._dropped_requirements(page, blocks)
    assert len(got) == 1, got
    assert got[0]["key"] == "`url`" and got[0]["region"] == "config:db", got
    # a key that is still required must not be reported
    assert all(g["key"] != "`type`" for g in got), got
    # and neither must a region the regeneration did not produce
    assert refgen._dropped_requirements(page, {}) == []


@case("report: a page the tool could not read is named, and the exit code says so")
def _():
    # This is the failure the report exists to prevent, and the report had it:
    # a refused page was dropped from `plans` and rc=2 was overwritten by rc=1,
    # so it printed "every table matches the source" and exited 0.
    refused = [{"page": "x.md", "kind": "api", "regions": 0,
                "refused": "go build failed", "stale": [], "missing_marker": [],
                "orphaned_marker": [], "curation": []}]
    out = refgen.report(refused)
    assert "could not be read" in out.lower(), out
    assert "go build failed" in out, out
    assert "every table matches the source" not in out, out


@case("report: counts only the pages it could actually read")
def _():
    plans = [{"page": "a.md", "kind": "api", "regions": 7, "stale": [],
              "missing_marker": [], "orphaned_marker": [], "curation": []},
             {"page": "b.md", "kind": "cli", "regions": 0, "refused": "nope",
              "stale": [], "missing_marker": [], "orphaned_marker": [],
              "curation": []}]
    out = refgen.report(plans)
    assert "7 regions across 1 page" in out, out


@case("report: a message naming a file is not cut at the file's dot")
def _():
    plans = [{"page": "a.md", "kind": "config", "regions": 1, "stale": [],
              "missing_marker": [], "orphaned_marker": [],
              "curation": [{"kind": "missing_description",
                            "message": "DBConfig.URL has no doc comment in "
                                       "cli/internal/config/config.go anywhere. Fix it.",
                            "file": "cli/internal/config/config.go", "line": 3}]}]
    out = refgen.report(plans)
    assert "config.go anywhere" in out, out


@case("notice: every page carries one, and it says the prose is still yours")
def _():
    for kind, gen in refgen.GENERATORS.items():
        blocks = gen()
        assert "notice" in blocks, kind
        body = blocks["notice"]
        assert body.startswith(refgen.COMMENT[0]), (kind, body[:40])
        assert body.rstrip().endswith(refgen.COMMENT[1]), kind
        assert "AGENTS.md" in body, kind
        # it must not tell anyone to stop editing the prose, which is theirs
        assert "yours to change" in body, kind


@case("notice: it is not a section, so it gets no heading and goes first")
def _():
    assert refgen._suggest_heading("notice") == ""
    assert refgen._suggest_heading("cli:cmd:keys-export") == "### `ibc keys export`"


@case("notice: a marker above the frontmatter is refused")
def _():
    page = ("<!-- GEN:notice START -->\n<!-- GEN:notice END -->\n"
            "---\ntitle: \"X\"\n---\n\nbody\n")
    try:
        refgen._check_notice_placement(page, "x.md")
    except refgen.MarkerError as e:
        assert "frontmatter" in str(e), e
    else:
        raise AssertionError("a notice above the frontmatter must be refused")
    ok = ("---\ntitle: \"X\"\n---\n\n<!-- GEN:notice START -->\n"
          "<!-- GEN:notice END -->\n\nbody\n")
    refgen._check_notice_placement(ok, "x.md")      # must not raise


@case("api: a proto field with no comment and no entry raises, not a blank cell")
def _():
    # This used to return an empty string and raise nothing, so a new field on
    # a public API shipped with a blank Description and a clean check.
    key = ("PacketSelector", "sequence_number")
    saved = refgen.FIELD_DOCS[key]
    try:
        del refgen.FIELD_DOCS[key]
        try:
            refgen.gen_api()
        except refgen.SourceError as e:
            assert "sequence_number" in str(e), e
        else:
            raise AssertionError("a field nobody described must not render blank")
    finally:
        refgen.FIELD_DOCS[key] = saved


@case("api: an entry for a field that is gone raises")
def _():
    refgen.FIELD_DOCS[("PacketSelector", "gone_away")] = ("x", "y")
    try:
        refgen.gen_api()
    except refgen.SourceError as e:
        assert "gone_away" in str(e), e
    else:
        raise AssertionError("expected SourceError for a dead FIELD_DOCS entry")
    finally:
        refgen.FIELD_DOCS.pop(("PacketSelector", "gone_away"), None)


@case("api: a list renders as an array, not as protobuf's `repeated`")
def _():
    b = refgen.gen_api()
    assert "`PacketSelector[]`" in b["api:msg:SelectedPackets"]
    assert "repeated" not in b["api:msg:SelectedPackets"]
    assert "`uint64` (optional)" in b["api:msg:Attestation"]


@case("api: a description does not repeat the name of its own row")
def _():
    b = refgen.gen_api()
    assert b["api:rpc:Relay"].startswith("Tracks the packets")
    assert "Relay tracks" not in b["api:rpc:Relay"]


@case("api: a sibling field named in a description is fenced")
def _():
    t = refgen.gen_api()["api:msg:PacketStatus"]
    assert "Together with `source_client_id`" in t


@case("config: required-ness comes out of the Validate methods")
def _():
    b = refgen.gen_config()
    assert "| `chainId` | `string` | **required** |" in b["config:attestors:local"]
    assert "| `grpc` | `string` | **required** |" in b["config:attestors:remote"]


@case("config: a key of one kind never appears in the other kind's table")
def _():
    b = refgen.gen_config()
    # `.finalityOffset must not be set for remote attestors` keeps it out
    assert "`finalityOffset`" in b["config:attestors:local"]
    assert "`finalityOffset`" not in b["config:attestors:remote"]
    assert "`grpc`" not in b["config:attestors:local"]


@case("config: defaults come from DefaultConfig and from named constants")
def _():
    b = refgen.gen_config()
    assert "`0.0.0.0:3000`" in b["config:server"]
    assert "`1s`" in b["config:relayer"]            # dispatch.DefaultPollInterval
    assert "`50`" in b["config:relayer:chainOverrides"]   # pipeline.DefaultBatchSize


@case("config: a key with no description anywhere is an error, not a blank cell")
def _():
    saved = dict(refgen.FALLBACK_DOCS)
    try:
        del refgen.FALLBACK_DOCS[("ServerConfig", "listenAddr")]
        try:
            refgen.gen_config()
        except refgen.SourceError as e:
            assert e.kind == "missing_description", f"raised {e.kind!r}: {e}"
            return
        raise AssertionError("expected SourceError")
    finally:
        refgen.FALLBACK_DOCS.clear()
        refgen.FALLBACK_DOCS.update(saved)


@case("config: the canary fires when no validation message is recognised any more")
def _():
    # The guard against the worst thing this tool can do: render a whole page
    # of `optional` because the config package reworded its errors. It had no
    # test at all -- it could be deleted outright and both suites stayed green.
    saved = refgen.REQUIREMENT_VOCABULARY
    refgen.REQUIREMENT_VOCABULARY = ("no-message-says-this",)
    try:
        refgen.gen_config()
    except refgen.SourceError as e:
        assert e.kind == "all_keys_optional", f"raised {e.kind!r}: {e}"
        return
    finally:
        refgen.REQUIREMENT_VOCABULARY = saved
    raise AssertionError("expected the canary to refuse")


@case("config: a fallback description that the source now provides is an error")
def _():
    # Keyed on the yaml key, not the Go field name. Keyed on "Name" this entry
    # matched no field at all, so the refusal under test never ran and the case
    # passed on `dead_description` instead -- which is why the kind is asserted
    # rather than the mere fact of a raise.
    refgen.FALLBACK_DOCS[("AttestorConfig", "name")] = "shadows a real doc comment"
    try:
        refgen.gen_config()
    except refgen.SourceError as e:
        assert e.kind == "stale_fallback", f"raised {e.kind!r}: {e}"
        return
    finally:
        del refgen.FALLBACK_DOCS[("AttestorConfig", "name")]
    raise AssertionError("expected SourceError")


@case("config: error builders are found by shape, however they are spelled")
def _():
    # The four spellings Go allows for the same signature. Reading only one of
    # them means a constructor written another way carries its rules off the
    # page while the page still renders, which is the failure this replaced a
    # hardcoded list of names to avoid.
    for sig in ("segment string, format string, args ...any",
                "segment, format string, args ...any",
                "string, string"):
        got = refgen._path_error_ctors("func e(%s) error { return nil }" % sig)
        assert got == {"e": "seg_fmt"}, (sig, got)
    for sig, kind in (("segment string, err error", "seg_err"),
                      ("idx int, format string, args ...any", "idx_fmt"),
                      ("idx int, err error", "idx_err")):
        got = refgen._path_error_ctors("func e(%s) error { return nil }" % sig)
        assert got == {"e": kind}, (sig, got)
    # a function of another shape is not one of these
    assert refgen._path_error_ctors(
        "func store(c Config, path string, m map[string]string) error { return nil }") == {}

    # and the package's own builders are still all found
    src = "\n".join(refgen._read(f) for f in refgen._config_files())
    found = refgen._path_error_ctors(src)
    assert set(found.values()) == {"seg_err", "seg_fmt", "idx_err", "idx_fmt"}, found


@case("config: every method in the package is parsed, so no helper goes unread")
def _():
    src = "\n".join(refgen._read(f) for f in refgen._config_files())
    declared = {(m.group(2), m.group(3)) for m in
                re.finditer(r"^func \((\w+ )?\*?(\w+)\) (\w+)\(", src, re.M)}
    parsed = set(refgen._method_bodies(src))
    assert not declared - parsed, sorted(declared - parsed)


@case("config: a brace inside a string does not cut a Validate body short")
def _():
    # A rule below a raw string containing a lone `}` used to be unreadable,
    # and unreadable means absent from the page rather than reported.
    body = ('func (c T) Validate() error {\n'
            '\t_ = `\n}\n`\n'
            '\treturn errPathf("key", "required")\n}\n')
    rules = refgen._rules_in("".join(refgen._method_bodies(body).values()),
                             {"errPathf": "seg_fmt"})
    assert rules == [(".key required", [])], rules
    # and a parameter list with its own parentheses parses too
    body = ('func (c T) Validate(check func(string) error) error {\n'
            '\treturn errPathf("key", "required")\n}\n')
    rules = refgen._rules_in("".join(refgen._method_bodies(body).values()),
                             {"errPathf": "seg_fmt"})
    assert rules == [(".key required", [])], rules


@case("config: a default constant is read from its declaration, not a comment")
def _():
    import tempfile
    saved, saved_anchors = refgen.IBC, dict(refgen._ANCHORS)
    with tempfile.TemporaryDirectory() as d:
        os.makedirs(os.path.join(d, "cli", "pkg"))
        with open(os.path.join(d, "cli", "pkg", "opts.go"), "w") as fh:
            fh.write("package pkg\n\n"
                     "// DefaultThing = 99 * time.Second  (before the change)\n"
                     "/*\nDefaultThing = 98 * time.Second\n*/\n"
                     "const (\n\tDefaultThing = 2 * time.Second\n)\n")
        try:
            refgen.IBC = d
            refgen._ANCHORS.clear()
            refgen._ANCHORS.update({"__root__": d, "cli_module": "cli"})
            value, path, _line = refgen._const_value("DefaultThing")
        finally:
            refgen.IBC = saved
            refgen._ANCHORS.clear()
            refgen._ANCHORS.update(saved_anchors)
    assert value == "2s", value
    assert path.endswith("opts.go"), path


@case("config: a missing default constant is an error, not a stale number")
def _():
    key = ("RelayerConfig", "dispatchPollInterval")
    saved = refgen.DEFAULT_CONSTS[key]
    refgen.DEFAULT_CONSTS[key] = [("", "GoneAway")]
    try:
        refgen.gen_config()
    except refgen.SourceError:
        return
    finally:
        refgen.DEFAULT_CONSTS[key] = saved
    raise AssertionError("expected SourceError")


@case("cli: a command group with no section raises rather than going missing")
def _():
    saved = list(refgen.CLI_SECTION_ORDER)
    refgen.CLI_SECTION_ORDER.remove("migrate")
    try:
        refgen.gen_cli()
    except refgen.SourceError as e:
        assert "migrate" in str(e), e
        return
    finally:
        refgen.CLI_SECTION_ORDER[:] = saved
    raise AssertionError("expected SourceError")


@case("cli: one region per command, so a flagless command still needs a section")
def _():
    b = refgen.gen_cli()
    for path in ("migrate-up", "deploy-core", "keys-list"):     # no flags of their own
        assert f"cli:cmd:{path}" in b, path
        if path == "keys-list":            # no own flags and nothing inherited
            assert "| Flag |" not in b[f"cli:cmd:{path}"]
    assert len([k for k in b if k.startswith("cli:cmd:")]) == 28


@case("cli: a command lists every flag it accepts, its own first")
def _():
    b = refgen.gen_cli()
    assert not [k for k in b if "inherited" in k], "group flags inline, no separate table"
    rows = [l for l in b["cli:cmd:deploy-client"].split("\n") if l.startswith("| `--")]
    own = [i for i, l in enumerate(rows) if "--threshold" in l][0]
    got = [i for i, l in enumerate(rows) if "--manifest-dir" in l][0]
    assert own < got, "a command's own flags come before the ones it inherits"
    assert len(rows) == 13, rows                      # eight own, five inherited
    # a command with no flags of its own still shows what it inherits
    core = b["cli:cmd:deploy-core"]
    assert "`--manifest-dir <string>`" in core and "`--chain <string>`" in core


@case("cli: the type rides in the flag signature, and a bool has none")
def _():
    b = refgen.gen_cli()
    assert "`--threshold <uint8>`" in b["cli:cmd:deploy-client"]
    assert "`--dry-run`" in b["cli:cmd:deploy-core"]
    assert "| Flag | Default | Description |" in b["cli:global-flags"]
    assert "`bool`" not in b["cli:cmd:deploy-core"]


@case("cli: required flags come from the wiring, which --help never prints")
def _():
    b = refgen.gen_cli()
    assert "| `--tx-hash <string>` | required |" in b["cli:cmd:relayer-relay"]
    assert "| `--counterparty-chain <string>` | required |" in b["cli:cmd:deploy-client"]


@case("cli: angle brackets are fenced, so MDX cannot read one as a tag")
def _():
    for body in refgen.gen_cli().values():
        for row in (l for l in body.split("\n") if l.startswith("| ")):
            for cell in row.split("|"):
                if "<" in cell:
                    assert "`" in cell, cell


@case("cli: defaults are Cobra's own, including a flag author's parenthetical")
def _():
    b = refgen.gen_cli()
    assert "| `--home <string>` | `~/.ibc` |" in b["cli:global-flags"]
    assert "`deployments`" in b["cli:cmd:deploy-core"]
    assert "`cli-<a>-<b>`" in b["cli:cmd:deploy-client"]


@case("no generated table carries the retired product name")
def _():
    for gen in refgen.GENERATORS.values():
        for region, body in gen().items():
            assert "IBC Link" not in body, region


@case("--check reports stale and writes nothing")
def _():
    with tempfile.TemporaryDirectory() as d:
        p = os.path.join(d, "page.mdx")
        open(p, "w").write("{/* GEN:api:enum:PacketState START */}\nstale\n{/* GEN:api:enum:PacketState END */}\n")
        before = open(p).read()
        rc = refgen.run("api", p, check=True)
        assert rc == 1, rc
        assert open(p).read() == before, "check mode must not write"


@case("write then --check reports fresh")
def _():
    with tempfile.TemporaryDirectory() as d:
        p = os.path.join(d, "page.mdx")
        open(p, "w").write("{/* GEN:api:enum:PacketState START */}\nstale\n{/* GEN:api:enum:PacketState END */}\n")
        assert refgen.run("api", p, check=False) == 0
        assert refgen.run("api", p, check=True) == 0


@case("a region the owning page has no marker for is an error")
def _():
    saved = dict(refgen.PAGES)
    with tempfile.TemporaryDirectory() as d:
        page = os.path.join(d, "api.mdx")
        open(page, "w").write("{/* GEN:api:enum:PacketState START */}\n{/* GEN:api:enum:PacketState END */}\n")
        refgen.PAGES["api"] = page
        try:
            refgen.run("api", page, check=True)
        except refgen.MarkerError as e:
            assert "missing a marker" in str(e), e
            return
        finally:
            refgen.PAGES.clear()
            refgen.PAGES.update(saved)
    raise AssertionError("expected MarkerError")


@case("an html-comment marker works too, for a plain markdown page upstream")
def _():
    page = ("<!-- GEN:api:enum:PacketState START -->\nstale\n"
            "<!-- GEN:api:enum:PacketState END -->\n")
    out = refgen.render(page, {"api:enum:PacketState": "NEW"})
    assert "NEW" in out and "stale" not in out
    assert [i for i, *_ in refgen.find_regions(out)] == ["api:enum:PacketState"]


@case("a page with no markers at all is left alone")
def _():
    with tempfile.TemporaryDirectory() as d:
        p = os.path.join(d, "page.mdx")
        open(p, "w").write("just prose\n")
        assert refgen.run("api", p, check=False) == 0
        assert open(p).read() == "just prose\n"


@case("every struct that validates yields a rule, or is listed as ruleless")
def _():
    """A rule count that can silently reach zero is the failure this guards.

    Validation rules drive required-ness, enum values, and the discriminators.
    When the config package moved to errPathf helpers, the scraper harvested
    nothing from any struct, the pages still rendered, and check mode still
    reported them up to date. So any struct declaring Validate() must yield at
    least one rule unless it is named in RULELESS_VALIDATORS.
    """
    validations = refgen.parse_go_config()["validations"]
    assert validations, "no struct with a Validate() was found at all"
    empty = {s for s, rules in validations.items() if not rules}
    unexpected = empty - refgen.RULELESS_VALIDATORS
    assert not unexpected, (
        f"{sorted(unexpected)} declare Validate() but yield no field rule. Either "
        "the scraper cannot read how they build errors, or they belong in "
        "RULELESS_VALIDATORS.")
    # Absence, not just emptiness: a receiver shape the regex cannot match drops
    # the struct from validations entirely, and an emptiness check cannot see that.
    import re as _re, os as _os
    declared = set()
    for rel in refgen._config_files():
        src = open(_os.path.join(refgen.IBC, rel)).read()
        declared |= set(_re.findall(
            r"func \((?:\w+ )?\*?(\w+)\) Validate\([^)]*\) error \{", src))
    missing = declared - set(validations)
    assert not missing, (
        f"{sorted(missing)} declare Validate() in the source but never reached the "
        "parser. The receiver pattern in parse_go_config() cannot match their shape.")

    stale = refgen.RULELESS_VALIDATORS - set(validations)
    assert not stale, (
        f"{sorted(stale)} are in RULELESS_VALIDATORS but have no Validate() any "
        "more. Drop them from the list.")


for name in PASS:
    print(f"  ok    {name}")
for name, e, tb in FAIL:
    print(f"  FAIL  {name}: {e}")
print(f"\n{len(PASS)} passed, {len(FAIL)} failed")
if FAIL:
    print()
    print(FAIL[0][2])
sys.exit(1 if FAIL else 0)
