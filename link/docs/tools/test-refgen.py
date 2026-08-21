#!/usr/bin/env python3
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
    # StatusRequest follows `message RelayResponse {}` in relayer.proto
    assert "`tx_hash`" in b["api:msg:StatusRequest"]
    assert "api:msg:RelayResponse" not in b, "empty message should produce no table"


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


@case("api: a description does not repeat the name of its own row")
def _():
    b = refgen.gen_api()
    assert "| `Relay` | `RelayRequest` | `RelayResponse` | Tracks the packets" in b["api:relayer:rpcs"]
    assert "Relay tracks" not in b["api:relayer:rpcs"]


@case("api: a sibling field named in a description is fenced")
def _():
    t = refgen.gen_api()["api:msg:PacketStatus"]
    assert "Together with `source_client_id`" in t


@case("config: required-ness comes out of the Validate methods")
def _():
    t = refgen.gen_config()["config:attestors"]
    assert "| `chainId` | `string` | **required** for `local` |" in t
    assert "| `grpc` | `string` | **required** for `remote` |" in t


@case("config: a must-not-be-set rule names the kind the key belongs to")
def _():
    # `.finalityOffset must not be set for remote attestors` makes it local only
    assert "| `finalityOffset` | `uint` | `local` only |" in refgen.gen_config()["config:attestors"]


@case("config: defaults come from DefaultConfig and from named constants")
def _():
    b = refgen.gen_config()
    assert "`0.0.0.0:3000`" in b["config:server"]
    assert "`5s`" in b["config:relayer"]            # dispatch.DefaultPollInterval
    assert "`50`" in b["config:relayer:chainOverrides"]   # pipeline.DefaultBatchSize


@case("config: a key with no description anywhere is an error, not a blank cell")
def _():
    saved = dict(refgen.FALLBACK_DOCS)
    try:
        del refgen.FALLBACK_DOCS[("ServerConfig", "ListenAddress")]
        try:
            refgen.gen_config()
        except refgen.SourceError:
            return
        raise AssertionError("expected SourceError")
    finally:
        refgen.FALLBACK_DOCS.clear()
        refgen.FALLBACK_DOCS.update(saved)


@case("config: a fallback description that the source now provides is an error")
def _():
    refgen.FALLBACK_DOCS[("AttestorConfig", "Name")] = "shadows a real doc comment"
    try:
        refgen.gen_config()
    except refgen.SourceError:
        return
    finally:
        del refgen.FALLBACK_DOCS[("AttestorConfig", "Name")]
    raise AssertionError("expected SourceError")


@case("config: a missing default constant is an error, not a stale number")
def _():
    key = ("RelayerConfig", "DispatchPollInterval")
    saved = refgen.DEFAULT_CONSTS[key]
    refgen.DEFAULT_CONSTS[key] = [("", "link/internal/relay/dispatch/dispatcher.go", "GoneAway")]
    try:
        refgen.gen_config()
    except refgen.SourceError:
        return
    finally:
        refgen.DEFAULT_CONSTS[key] = saved
    raise AssertionError("expected SourceError")


@case("cli: every command lands in exactly one task group")
def _():
    saved = list(refgen.CLI_TASKS)
    region, commands = saved[2]
    refgen.CLI_TASKS[2] = (region, [c for c in commands if c != "attestor run"])
    try:
        refgen.gen_cli()
    except refgen.SourceError as e:
        assert "attestor run" in str(e), e
        return
    finally:
        refgen.CLI_TASKS[:] = saved
    raise AssertionError("expected SourceError")


@case("cli: required flags come from the wiring, which --help never prints")
def _():
    b = refgen.gen_cli()
    assert "| `--tx-hash` | `string` | **required** |" in b["cli:flags:relayer-relay"]
    assert "| `--counterparty-chain` | `string` | **required** |" in b["cli:flags:deploy-client"]


@case("cli: a command crossing the flag threshold is an error, not a silent drop")
def _():
    saved = list(refgen.CLI_FLAG_SECTIONS)
    refgen.CLI_FLAG_SECTIONS.remove("deploy client")
    try:
        refgen.gen_cli()
    except refgen.SourceError as e:
        assert "deploy client" in str(e), e
        return
    finally:
        refgen.CLI_FLAG_SECTIONS[:] = saved
    raise AssertionError("expected SourceError")


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
    assert "| `--home` | `string` | `~/.ibc` |" in b["cli:global-flags"]
    assert "`deployments`" in b["cli:group-flags:deploy"]
    assert "`link-<a>-<b>`" in b["cli:flags:deploy-client"]


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


for name in PASS:
    print(f"  ok    {name}")
for name, e, tb in FAIL:
    print(f"  FAIL  {name}: {e}")
print(f"\n{len(PASS)} passed, {len(FAIL)} failed")
if FAIL:
    print()
    print(FAIL[0][2])
sys.exit(1 if FAIL else 0)
