#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0

"""Generate reference-page tables from source, in place, between markers.

A reference page is part prose and part table. The prose is written by a human
and the tables are derived from code, so the tables can be regenerated whenever
the code moves and the page cannot drift from it.

    python3 tools/refgen.py api    ibc-docs/ibc-cli/api.mdx
    python3 tools/refgen.py config ibc-docs/ibc-cli/configuration.mdx
    python3 tools/refgen.py cli    ibc-docs/ibc-cli/cli-commands.mdx
    python3 tools/refgen.py all --check      # exit 1 if any page is stale

Generated regions are delimited by comments the review-copy stripper removes:

    {/* GEN:api:relayer:rpcs START */}
    ...generated...
    {/* GEN:api:relayer:rpcs END */}

Only bytes strictly between a START and its END are ever rewritten. Text
outside every marker is asserted byte-identical after a write, because a tool
that edits these pages has destroyed content here twice before.

Currency has two halves. This tool is the first one:

    python3 tools/refgen.py all --check           # fails if a table moved
    python3 tools/refgen.py all --list-regions    # names which ones
    python3 tools/refgen.py all                   # heals them

The second half is the prose around the tables, which regenerating does not
touch, and which only reading can check. See the `reference-drift` skill, run
after regenerating and triggered by check mode going red.

--------------------------------------------------------------------------
THE PAGES LIVE IN cosmos/ibc (Evan, 2026-08-21)
--------------------------------------------------------------------------
The reference pages are upstream's, at cli/docs/, and are edited there. This
copy of the tool runs against the pinned clone so the docs project can still
generate and check; `tools/upstreamize.py` syncs the tool upstream and
deliberately does not copy the pages, which would clobber them.

This checklist stays for the record, and for the next surface that moves.

  1. Copy `refgen.py`, `test-refgen.py`, and `test-refgen-e2e.py`.
  2. Set IBC to the repo root. Here it points at a pinned clone in `repos/ibc`;
     upstream it is the repo itself, so `--check` runs against the working tree
     and a developer sees their own change.
  3. Copy the reference pages, or point PAGES at wherever they live. PAGES is
     the coverage contract: a region with no marker on its owning page is an
     error, which is what stops a new key from landing in the generator and
     never reaching a reader.
  4. Copy the workflow. Switch its `paths` to `cli/**` and `proto/**`, which
     is the whole point of moving: the pull request that adds a flag is the
     one that goes red. The workflow header carries that block.
  5. Drop the clone step from the workflow. Upstream the source is already
     checked out, and CLI generation needs only the Go toolchain.
  6. Run `test-refgen-e2e.py` first. Ten mutations, and each one proves a
     failure this design depends on. If they pass upstream, the move worked.
  7. Keep check mode alone until it has been quiet for a while. A scheduled
     regenerate-and-open-a-PR job comes after that, never straight to the
     default branch.

What does NOT travel: the hand-written prose passes and the `reference-drift`
skill, which are docs-side. Upstream sees a red check and a diff; deciding
what the prose should now say stays with whoever owns the pages.
"""
import argparse
import difflib
import hashlib
import json
import os
import re
import subprocess
import sys

# upstream layout: this file is docs/6-ibc-cli/tools/refgen.py, so the repo root
# is three levels up, and the source it reads is the working tree itself
ROOT = os.path.dirname(os.path.dirname(os.path.dirname(
    os.path.dirname(os.path.abspath(__file__)))))
IBC = ROOT

# Markers and citations sit in comments the reader never sees. MDX pages use
# {/* ... */}; a plain markdown page upstream uses <!-- ... -->. Both are
# recognised, and COMMENT emits the style this checkout writes.
START = re.compile(r"(?:\{/\*|<!--)\s*GEN:(?P<id>[A-Za-z0-9_:.-]+)\s+START\s*(?:\*/\}|-->)")
END = re.compile(r"(?:\{/\*|<!--)\s*GEN:(?P<id>[A-Za-z0-9_:.-]+)\s+END\s*(?:\*/\}|-->)")

COMMENT = ("<!--", "-->")   # plain markdown, so GitHub hides them


class MarkerError(Exception):
    pass


def find_regions(text):
    """Return [(id, body_start, body_end, outer_start, outer_end)] in order.

    Raises on unbalanced, nested, mismatched, or duplicated markers. A page
    whose markers do not make sense is never written to.
    """
    events = []
    for m in START.finditer(text):
        events.append((m.start(), "start", m.group("id"), m))
    for m in END.finditer(text):
        events.append((m.start(), "end", m.group("id"), m))
    events.sort()

    regions, open_at = [], None
    for _pos, kind, ident, m in events:
        if kind == "start":
            if open_at is not None:
                raise MarkerError(f"GEN:{ident} START inside unclosed GEN:{open_at[0]}")
            open_at = (ident, m)
        else:
            if open_at is None:
                raise MarkerError(f"GEN:{ident} END with no START")
            if open_at[0] != ident:
                raise MarkerError(f"GEN:{open_at[0]} START closed by GEN:{ident} END")
            s = open_at[1]
            regions.append((ident, s.end(), m.start(), s.start(), m.end()))
            open_at = None
    if open_at is not None:
        raise MarkerError(f"GEN:{open_at[0]} START never closed")

    seen = set()
    for ident, *_ in regions:
        if ident in seen:
            raise MarkerError(f"GEN:{ident} appears more than once")
        seen.add(ident)
    return regions


def outside(text, regions):
    """The page with every generated body blanked, for equality checks."""
    out, last = [], 0
    for _ident, bs, be, _os_, _oe in regions:
        out.append(text[last:bs])
        last = be
    out.append(text[last:])
    return "".join(out)


def render(text, blocks):
    """Replace region bodies from {id: body}. Unknown ids on the page are an
    error; a generator that produces nothing for a marker is a bug, not a
    no-op."""
    regions = find_regions(text)
    have = {i for i, *_ in regions}
    missing = set(blocks) - have
    if missing:
        raise MarkerError(f"no marker on page for: {', '.join(sorted(missing))}")
    unfilled = have - set(blocks)
    if unfilled:
        raise MarkerError(
            "marker present but generator produced nothing: "
            + ", ".join(sorted(unfilled))
            + ". The source no longer has what that region described, so delete "
              "the page section along with its markers, or restore the source. "
              "Nothing is written until one of those happens.")

    new = text
    for ident, bs, be, _os_, _oe in reversed(regions):
        new = new[:bs] + "\n\n" + blocks[ident].strip() + "\n\n" + new[be:]

    # the safety property: nothing outside a generated body may change
    if outside(new, find_regions(new)) != outside(text, regions):
        raise MarkerError("refusing to write: text outside generated regions would change")
    return new


def table(headers, rows):
    if not rows:
        return "_None._"
    out = ["| " + " | ".join(headers) + " |", "|" + "|".join("---" for _ in headers) + "|"]
    for r in rows:
        cells = [str(c).replace("|", r"\|").replace("\n", " ") for c in r]
        out.append("| " + " | ".join(cells) + " |")
    return "\n".join(out)


def cite(path, start, end=None):
    rng = f"L{start}" if end is None or end == start else f"L{start}-L{end}"
    open_, close = COMMENT
    return f"{open_} [{os.path.basename(path)}:{rng}]({path}#{rng}) {close}"


# ---------------------------------------------------------------- proto -> api


def _decl_blocks(src):
    """Yield (kind, name, line_no, doc, body) for every top-level proto decl.

    Brace-aware, so a message written on one line and a message with a body
    are handled by the same code. The earlier line-scanning version read
    forward past a one-line message and stole the next message's fields.
    """
    lines = src.split("\n")
    doc, i = [], 0
    decl = re.compile(r"^(service|message|enum)\s+(\w+)\s*\{")
    while i < len(lines):
        stripped = lines[i].strip()
        if stripped.startswith("//"):
            doc.append(stripped[2:].strip())
            i += 1
            continue
        m = decl.match(stripped)
        if not m:
            doc, i = [], i + 1
            continue
        # walk characters from the opening brace until it balances
        depth, j, body = 0, i, []
        while j < len(lines):
            line = lines[j]
            start = line.index("{") + 1 if j == i else 0
            piece, cut = [], None
            for pos in range(start, len(line)):
                ch = line[pos]
                if ch == "{":
                    depth += 1
                elif ch == "}":
                    if depth == 0:
                        cut = pos
                        break
                    depth -= 1
                piece.append(ch)
            body.append("".join(piece))
            if cut is not None:
                break
            j += 1
        yield m.group(1), m.group(2), i + 1, " ".join(doc), "\n".join(body)
        doc, i = [], j + 1


FIELD = re.compile(r"^(optional\s+|repeated\s+|required\s+)?([\w.]+)\s+(\w+)\s*=\s*\d+")
RPC = re.compile(r"rpc\s+(\w+)\s*\(\s*([\w.]+)\s*\)\s*returns\s*\(\s*([\w.]+)\s*\)")


def _statements(body):
    """Yield (doc, statement, offset) for each `;`-terminated statement in a
    body, carrying the comment lines that precede it and the line it sits on,
    counted from the body's first line. Statements may share a line."""
    doc = []
    for offset, raw in enumerate(body.split("\n")):
        line = raw.strip()
        if line.startswith("//"):
            doc.append(line[2:].strip())
            continue
        for part in line.split(";"):
            part = part.strip()
            if not part:
                continue
            yield " ".join(doc), part, offset
            doc = []


def _fields(body):
    """Fields of a message body, in declaration order, with a oneof folded
    into one entry."""
    oneofs = {name: {"name": name, "type": "oneof", "doc": doc,
                     "opts": [f.group(3) for _d, s, _o in _statements(inner)
                              for f in [FIELD.match(s)] if f]}
              for name, doc, inner in _decl_oneofs(body)}
    # blank the oneof bodies, keeping line positions, so one ordered pass works
    masked = body
    for _n, _d, inner in _decl_oneofs(body):
        masked = masked.replace(inner, "\n" * inner.count("\n"))

    out, doc = [], []
    for raw in masked.split("\n"):
        line = raw.strip()
        if line.startswith("//"):
            doc.append(line[2:].strip())
            continue
        m = re.match(r"oneof\s+(\w+)\s*\{", line)
        if m:
            out.append(oneofs[m.group(1)])
            doc = []
            continue
        for part in line.split(";"):
            stmt = part.strip()
            if not stmt or stmt in ("}", "{"):
                continue
            f = FIELD.match(stmt)
            if not f:
                continue
            prefix = (f.group(1) or "").strip()
            t = f.group(2)
            if prefix == "repeated":
                t = "repeated " + t
            elif prefix == "optional":
                t = t + ", optional"
            out.append({"name": f.group(3), "type": t, "doc": " ".join(doc)})
            doc = []
    return out


def _decl_oneofs(body):
    """Yield (name, doc, inner_body) for each oneof in a message."""
    lines = body.split("\n")
    doc = []
    for idx, raw in enumerate(lines):
        line = raw.strip()
        if line.startswith("//"):
            doc.append(line[2:].strip())
            continue
        m = re.match(r"oneof\s+(\w+)\s*\{", line)
        if not m:
            doc = []
            continue
        rest = "\n".join(lines[idx:])
        open_at = rest.index("{")
        depth, end = 0, None
        for pos in range(open_at + 1, len(rest)):
            if rest[pos] == "{":
                depth += 1
            elif rest[pos] == "}":
                if depth == 0:
                    end = pos
                    break
                depth -= 1
        yield m.group(1), " ".join(doc), rest[open_at + 1:end]
        doc = []


def parse_proto(path):
    """Services, rpcs, messages, enums with their leading // comments."""
    src = open(os.path.join(IBC, path)).read()
    out = {"services": [], "messages": [], "enums": []}
    for kind, name, line, doc, body in _decl_blocks(src):
        if kind == "service":
            rpcs = []
            for rdoc, stmt, offset in _statements(body):
                m = RPC.match(stmt)
                if m:
                    # the rpc's own line, so its citation points at itself
                    # rather than at the service declaration above it
                    rpcs.append({"name": m.group(1), "req": m.group(2),
                                 "resp": m.group(3), "doc": rdoc,
                                 "line": line + offset})
            out["services"].append({"name": name, "doc": doc, "line": line, "rpcs": rpcs})
        elif kind == "message":
            out["messages"].append({"name": name, "doc": doc, "line": line,
                                    "fields": _fields(body)})
        else:
            values = []
            for vdoc, stmt, _offset in _statements(body):
                m = re.match(r"(\w+)\s*=\s*\d+", stmt)
                if m:
                    values.append({"name": m.group(1), "doc": vdoc})
            out["enums"].append({"name": name, "doc": doc, "line": line, "values": values})
    return out

def _proto_type(t):
    """A field's type as a reader of JSON meets it.

    `repeated` is protobuf's word for a list and means nothing to someone
    writing a request body, so it renders as an array instead.
    """
    if t.startswith("repeated "):
        return f"`{t[len('repeated '):]}[]`"
    if t.endswith(", optional"):
        return f"`{t[:-len(', optional')]}` (optional)"
    return f"`{t}`"


def _lead_strip(name, doc):
    """Drop the identifier a Go or proto comment opens with.

    `// Relay tracks the packets` documents the RPC named Relay, and a table
    cell that repeats the name in its own row reads as a stutter.
    """
    if not doc:
        return ""
    camel = "".join(part.capitalize() for part in name.split("_"))
    for lead in (name, camel):
        if doc.startswith(lead + " "):
            doc = doc[len(lead) + 1:]
            break
    doc = re.sub(r"^is\s+", "", doc)
    return doc[0].upper() + doc[1:] if doc else ""


def _fence(doc, names):
    """Fence a field name used inside another field's description."""
    for n in sorted(names, key=len, reverse=True):
        doc = re.sub(r"(?<![`\w])" + re.escape(n) + r"(?![`\w])", f"`{n}`", doc)
    return doc


PROTO_DIR = "proto"


def _proto_files():
    """Every .proto under PROTO_DIR. Naming them would mean a new service is
    absent from the page with nothing to notice it."""
    out = []
    for root, _dirs, files in os.walk(os.path.join(IBC, PROTO_DIR)):
        for f in sorted(files):
            if f.endswith(".proto"):
                out.append(os.path.relpath(os.path.join(root, f), IBC))
    return sorted(out)


def _proto_short(path):
    """The last segment of the file's proto package, which names its regions."""
    m = re.search(r"^package\s+([\w.]+);", _read(path), re.M)
    if not m:
        raise SourceError(f"{path} declares no proto package")
    return m.group(1).split(".")[-1]


# The services, in the order a reader meets them, with the names the page uses.
# The proto files sort alphabetically and the service type is RelayerApiService,
# so both the order and the display name are a human's call. A service missing
# from here raises, and an entry naming a service that is gone raises too.
SERVICES = [("relayer", "Relayer service"), ("attestor", "Attestation service"),
            ("prover", "Prover service")]

# Descriptions for fields the protos leave undocumented. Nearly all of these
# were prose on the page already, moved into the cell they belong in. Values
# never come from here, only wording, and the four checks that keep
# FALLBACK_DOCS honest apply here too: an entry for a field that is gone
# raises, a field that gains a proto comment raises, and a fingerprint over the
# field's type raises when the shape changes under a stable name.
FIELD_DOCS = {
    ("RelayRequest", "tx_hash"): ("The transaction that sent the packets, on the source chain.", "e86e1cfa"),
    ("RelayRequest", "source_chain_id"): ("The chain that transaction was sent on.", "47d6b1f4"),
    ("SelectedPackets", "packets"): ("The packets to relay. At least one.", "d196665f"),
    ("PacketSelector", "source_client_id"): ("The client the packet was sent on.", "947d8614"),
    ("PacketSelector", "sequence_number"): ("The packet's number on that client.", "71c4fc7e"),
    ("ObservedPacket", "source_client_id"): ("The client the packet was sent on.", "947d8614"),
    ("ObservedPacket", "sequence_number"): ("The packet's number on that client.", "71c4fc7e"),
    ("ObservedPacket", "selection"): ("Whether this relayer took the packet. See the values below.", "8fb64d4f"),
    ("PacketsRequest", "filter"): ("Narrows the results. Every field is optional.", "a4b73c55"),
    ("PacketFilter", "source_chain_id"): ("Only packets sent from this chain.", "c649a782"),
    ("PacketFilter", "destination_chain_id"): ("Only packets bound for this chain.", "8fed4c25"),
    ("PacketFilter", "source_client_id"): ("Only packets sent on this client.", "e2f36fe9"),
    ("PacketFilter", "destination_client_id"): ("Only packets received on this client.", "63078f21"),
    ("PacketFilter", "state"): ("Only packets in this state.", "feb79267"),
    ("PacketFilter", "source_tx_hash"): ("Only packets sent by this transaction.", "224d8872"),
    ("PacketFilter", "sequence_number"): ("Only packets with this sequence number.", "25882ec9"),
    ("PacketsResponse", "packets"): ("One entry per matching packet on this page, newest first.", "039ea081"),
    ("PacketStatus", "state"): ("Where the packet got to. See the states below.", "5c7b7988"),
    ("TransactionInfo", "tx_hash"): ("The transaction's hash.", "e86e1cfa"),
    ("TransactionInfo", "chain_id"): ("The chain it was submitted to.", "8140443d"),
    ("StateAttestationRequest", "attestor"): ("Which attestor to ask, by its `name` in the `attestors` block.", "5d870b5f"),
    ("StateAttestationRequest", "height"): ("The height to attest to.", "f4439355"),
    ("StateAttestationResponse", "attestation"): ("The signed attestation. See below.", "b9fffb17"),
    ("PacketAttestationRequest", "attestor"): ("Which attestor to ask, by its `name` in the `attestors` block.", "5d870b5f"),
    ("PacketAttestationResponse", "attestation"): ("The signed attestation. See below.", "b9fffb17"),
    ("LatestHeightRequest", "attestor"): ("Which attestor to ask, by its `name` in the `attestors` block.", "5d870b5f"),
    ("LatestHeightResponse", "height"): ("The highest height this attestor will attest to.", "f4439355"),
    ("InfoRequest", "attestor"): ("Which attestor to ask, by its `name` in the `attestors` block.", "5d870b5f"),
}


def _field_fingerprint(msg, field):
    """Hash what a hand-written field description depends on: the field's type
    and its name. Blind to formatting and to the rest of the message."""
    return hashlib.sha1(f"{field['type']}|{field['name']}".encode()).hexdigest()[:8]


def _field_doc(msg, field, seen):
    """The Description cell, with the same four checks the config page uses."""
    doc = _lead_strip(field["name"], field["doc"])
    entry = FIELD_DOCS.get((msg, field["name"]))
    where = f"{msg}.{field['name']}"
    if doc and entry:
        _problem("stale_field_doc",
                 f"{where} now has a proto comment; drop its FIELD_DOCS entry", field=where)
        return doc
    if not entry:
        return doc
    text, recorded = entry
    seen.add((msg, field["name"]))
    current = _field_fingerprint(msg, field)
    if recorded and current != recorded:
        _problem("field_fingerprint_mismatch",
                 f"{where}: the field changed shape (fingerprint {recorded} -> {current}). "
                 f"Re-read \"{text}\" against the proto, then record the new fingerprint.",
                 field=where, description=text, was=recorded, now=current)
    return text


def gen_api():
    blocks, seen_docs, by_short = {}, set(), {}
    for fname in _proto_files():
        by_short[_proto_short(fname)] = (fname, parse_proto(fname))

    missing = [s for s, _n in SERVICES if s not in by_short]
    if missing:
        _problem("missing_service",
                 f"SERVICES names proto packages that are gone: {missing}", services=missing)
    unlisted = sorted(set(by_short) - {s for s, _n in SERVICES})
    if unlisted:
        _problem("unlisted_service",
                 "these proto packages have no section on the page: " + ", ".join(unlisted),
                 services=unlisted)

    for short, _display in SERVICES:
        if short not in by_short:
            continue
        fname, p = by_short[short]
        for svc in p["services"]:
            for r in svc["rpcs"]:
                # one region per RPC, so an RPC that reuses existing messages
                # still needs a section and cannot arrive unnoticed
                blocks[f"api:rpc:{r['name']}"] = (
                    _lead_strip(r["name"], r["doc"]) + "\n\n" + cite(fname, r["line"]))
        for msg in p["messages"]:
            if not msg["fields"]:
                continue
            rows, names = [], {f["name"] for f in msg["fields"]}
            for f in msg["fields"]:
                t = (f"oneof: {' or '.join('`'+o+'`' for o in f['opts'])}"
                     if f["type"] == "oneof" else _proto_type(f["type"]))
                doc = _fence(_field_doc(msg["name"], f, seen_docs), names - {f["name"]})
                rows.append((f"`{f['name']}`", t, doc))
            blocks[f"api:msg:{msg['name']}"] = (
                table(["Field", "Type", "Description"], rows)
                + "\n\n" + cite(fname, msg["line"]))
        for en in p["enums"]:
            rows = [(f"`{v['name']}`", v["doc"]) for v in en["values"]
                    if not v["name"].endswith("UNSPECIFIED")]
            blocks[f"api:enum:{en['name']}"] = (
                table(["Value", "Meaning"], rows) + "\n\n" + cite(fname, en["line"]))

    orphans = sorted(f"{m}.{f}" for m, f in set(FIELD_DOCS) - seen_docs)
    if orphans:
        _problem("dead_field_doc",
                 "FIELD_DOCS describes fields that are gone: " + ", ".join(orphans),
                 fields=orphans)
    return blocks


# ------------------------------------------------------------- go -> config

# The config package, read whole. Naming files here would mean a new file with
# a new block is silently absent from the page, which is the failure this whole
# tool exists to prevent.
CONFIG_PKG = "cli/internal/config"

# The one anchor. Every block on the page is a struct reachable from this type,
# so the page's shape follows the code's rather than a list kept by hand.
CONFIG_ROOT = "Config"

# Where a pointer field's default lives when the struct itself carries no
# value: a named constant in the code that consumes the field. The label in
# each entry is prose; the value is always read from source, and a missing
# constant is an error rather than a stale number.
DEFAULT_CONSTS = {
    ("RelayerConfig", "DispatchPollInterval"): [
        ("", "cli/internal/relay/dispatch/dispatcher.go", "DefaultPollInterval")],
    ("RelayerChainOverride", "TxSubmissionDelay"): [
        ("", "cli/internal/txsubmitter/evm/evm.go", "DefaultTxSubmissionDelay")],
    ("RelayerChainOverride", "PacketBatchSize"): [
        ("", "cli/internal/relay/pipeline/opts.go", "DefaultBatchSize")],
    ("RelayerChainOverride", "PacketBatchTimeout"): [
        ("receive and acknowledge", "cli/internal/relay/pipeline/opts.go", "DefaultBatchTimeout"),
        ("timeout", "cli/internal/relay/pipeline/opts.go", "DefaultTimeoutBatchTimeout")],
}

# Keys the Go source does not document. Values never come from here, only
# wording: every default, type, and required-ness is read from source on every
# run. Each entry is (description, fingerprint), where the fingerprint covers
# the field's type, its yaml key, and every validation rule naming it.
#
# Four ways this map is stopped from going stale, each with a mutation test:
#   * a key with no comment and no entry here raises
#   * a key that gains a doc comment upstream raises, so an entry cannot
#     outlive the gap it fills
#   * an entry matching no field raises, so a removed key cannot leave a
#     dead description behind
#   * a fingerprint mismatch raises, so a key whose type or validation rules
#     changed under a stable name forces someone to re-read the sentence
#
# The real fix is upstream doc comments. Every one added shrinks this map, and
# the second rule above turns that into a guided migration rather than a sweep.
FALLBACK_DOCS = {
    ("ServerConfig", "ListenAddress"): ("Address the gRPC server binds. It serves the relayer and attestor APIs together.", "a05468ed"),
    ("DBConfig", "Type"): ("Database backend.", "70e2ad2c"),
    ("DBConfig", "URL"): ("File path for sqlite, connection string for postgres. `:memory:` is rejected.", "d084b0d4"),
    ("ChainConfig", "ChainID"): ("The chain's id, as the chain reports it.", "69a3e543"),
    ("EVMChainConfig", "RPC"): ("JSON-RPC endpoint for the chain.", "690d071f"),
    ("EVMChainConfig", "ICS26Router"): ("Address of the ICS26 router on the chain.", "1daaecba"),
    ("AttestorConfig", "Type"): ("Whether this process runs the attestor or queries it.", "a58f9a4e"),
    ("SignerConfig", "Type"): ("Whether the key is a file on disk or a key held by a remote signer.", "febf1ab4"),
    ("RelayerConfig", "DispatchPollInterval"): ("How often the dispatcher polls the store for unfinished packets.", "893f79b1"),
    ("RelayerChainOverride", "ChainID"): ("The chain these settings apply to.", "69a3e543"),
    ("RelayerChainOverride", "TxSubmissionDelay"): ("Minimum delay between two transaction submissions on the chain.", "5691fa23"),
    ("RelayerChainOverride", "PacketBatchSize"): ("How many packets the relayer puts in one transaction.", "b4f4f14c"),
    ("RelayerChainOverride", "PacketBatchTimeout"): ("How long the relayer waits to fill a batch before submitting it.", "84d8816e"),
    ("RelayerEVMConfig", "GasFeeCapMultiplier"): ("Multiplies the fee cap the node suggests.", "b9de0a8d"),
    ("RelayerEVMConfig", "GasTipCapMultiplier"): ("Multiplies the tip cap the node suggests.", "634e0708"),
    ("ConnectionConfig", "Alias"): ("Name for the connection, unique in the file.", "7e352d14"),
    ("AutoRelayConfig", "Enabled"): ("Whether the relayer carries packets leaving this end without being asked.", "d693129e"),
    ("ClientEnd", "ChainID"): ("The chain this end's client lives on.", "69a3e543"),
    ("ClientEnd", "Signer"): ("`signers` alias that submits relay transactions on this chain.", "00fd3d36"),
    ("ClientEnd", "ClientID"): ("The light client's id on this chain.", "bb596da7"),
    ("ClientEnd", "Type"): ("Light client type.", "85b1564f"),
}

# Explicit skips only. Unexported struct fields and yaml:"-" tags are dropped
# in parse_go_config and never reach here.
SKIP_FIELDS = set()

# Pointer fields whose default is not a named constant anywhere: unset means
# unset, and the prose says what that implies. Listed so that a new pointer
# field cannot quietly read as "optional" when a default exists for it.
NO_NAMED_DEFAULT = {
    # nil and false are the same input: a connection end without it is not
    # auto-relayed (relayer.go:L127)
    ("AutoRelayConfig", "Enabled"),
    ("RelayerEVMConfig", "GasFeeCapMultiplier"),
    ("RelayerEVMConfig", "GasTipCapMultiplier"),
}

GO_TYPES = {"string": "string", "uint": "uint", "uint64": "uint64", "int": "int",
            "bool": "bool", "float64": "float64", "time.Duration": "duration"}


class SourceError(Exception):
    pass


# When PLAN is a list, a problem is recorded and generation continues with a
# placeholder, so one run reports every gap rather than the first. When it is
# None, the same problem raises and nothing is written. Check mode and a normal
# regeneration always run with PLAN None: refusing to write stays the default.
PLAN = None


def _problem(kind, message, **fields):
    """Raise, or record and continue in plan mode."""
    if PLAN is None:
        raise SourceError(message)
    PLAN.append(dict(kind=kind, message=message, **fields))


def _read(path):
    return open(os.path.join(IBC, path)).read()


def _is_config_field(go_name, yaml_key):
    """True when a struct field belongs in generated config docs."""
    if yaml_key == "-":
        return False
    return go_name[0].isupper()


def _config_files():
    """Every non-test Go file in the config package, sorted for stable output."""
    d = os.path.join(IBC, CONFIG_PKG)
    return [f"{CONFIG_PKG}/{f}" for f in sorted(os.listdir(d))
            if f.endswith(".go") and not f.endswith("_test.go")]


def parse_go_config():
    """Structs, string constants, literal defaults, and validation rules from
    the config package."""
    structs, aliases, consts, const_type, defaults, validations = {}, {}, {}, {}, {}, {}
    files = _config_files()
    src = "\n".join(_read(f) for f in files)

    for m in re.finditer(r"type\s+(\w+)\s+\[\](\w+)", src):
        aliases[m.group(1)] = m.group(2)

    for m in re.finditer(r'(\w+)\s+(\w+)?\s*=\s*"([^"]*)"', src):
        consts[m.group(1)] = m.group(3)
        if m.group(2):
            const_type[m.group(1)] = m.group(2)

    for path in files:
        lines = _read(path).split("\n")
        i = 0
        while i < len(lines):
            m = re.match(r"type\s+(\w+)\s+struct\s*\{", lines[i])
            if not m:
                i += 1
                continue
            name, fields, doc = m.group(1), [], []
            j = i + 1
            while j < len(lines) and not lines[j].startswith("}"):
                ln = lines[j].strip()
                if ln.startswith("//"):
                    doc.append(ln[2:].strip())
                elif ln:
                    f = re.match(r"(\w+)\s+([^\s`]+)(?:\s+`([^`]*)`)?", ln)
                    if f:
                        go_name = f.group(1)
                        tag = re.search(r'yaml:"([^",]+)', f.group(3) or "")
                        yaml_key = tag.group(1) if tag else go_name
                        if _is_config_field(go_name, yaml_key):
                            fields.append({"go": go_name, "type": f.group(2),
                                           "yaml": yaml_key,
                                           "doc": " ".join(doc), "line": j + 1})
                    doc = []
                else:
                    doc = []
                j += 1
            structs[name] = {"fields": fields, "line": i + 1, "file": path}
            i = j + 1

    # literal defaults from DefaultConfig()
    body = src[src.index("func DefaultConfig()"):]
    body = body[:body.index("\n}\n")]
    current = None
    for ln in body.split("\n"):
        s = ln.strip()
        m = re.match(r"\w+:\s*(\w+)\{$", s)
        if m:
            current = m.group(1)
            continue
        if s.startswith("}"):
            current = None
            continue
        m = re.match(r'(\w+):\s*(?:"([^"]*)"|(\w+)),$', s)
        if m and current:
            value = m.group(2) if m.group(2) is not None else consts.get(m.group(3), m.group(3))
            defaults[(current, m.group(1))] = value

    # validation rules: every error string a struct's Validate can return
    for m in re.finditer(r"func \((?:\w+ )?\*?(\w+)\) Validate\([^)]*\) error \{", src):
        recv = m.group(1)
        tail = src[m.end():]
        tail = tail[:tail.index("\n}\n")]
        msgs = []
        for e in re.finditer(r'errors\.(?:New|Errorf)\("(\.[^"]+)"((?:,\s*\w+)*)', tail):
            msgs.append((e.group(1), [a.strip() for a in e.group(2).split(",") if a.strip()]))
        for e in re.finditer(r'errPathf\("([^"]+)",\s*"([^"]+)"((?:,\s*[\w.]+)*)', tail):
            msgs.append((f".{e.group(1)} {e.group(2)}",
                         [a.strip() for a in e.group(3).split(",") if a.strip()]))
        for e in re.finditer(r'errPathIndexf\(\w+,\s*"([^"]+)"((?:,\s*[\w.]+)*)', tail):
            msgs.append((f".[] {e.group(1)}",
                         [a.strip() for a in e.group(2).split(",") if a.strip()]))
        unknown = {c for c in re.findall(r'\b((?:errors|fmt)\.\w+|errPath\w*)\(', tail)
                   if c not in KNOWN_ERR_CTORS}
        if unknown:
            raise SourceError(
                f"{recv}.Validate() builds errors with {', '.join(sorted(unknown))}, "
                "which this scraper does not read. Validation rules drive the "
                "required column, enum values, and the discriminators, so an unread "
                "constructor silently empties all three. Teach parse_go_config() to "
                "read it, or add it to KNOWN_ERR_CTORS if it carries no field rule.")

        helpers = {h for h in re.findall(r'\.(validate\w+)', tail)
                   if h not in KNOWN_VALIDATE_HELPERS}
        if helpers:
            raise SourceError(
                f"{recv}.Validate() delegates to {', '.join(sorted(helpers))}, whose "
                "body this scraper does not follow: the path segment is at the call "
                "site and the message is inside the helper, so the rule cannot be "
                "reassembled. Its checks would go unread while the page still "
                "renders. Add it to KNOWN_VALIDATE_HELPERS once you have confirmed "
                "what the page should say about the fields it guards.")
        validations[recv] = msgs

    return {"structs": structs, "aliases": aliases, "consts": consts,
            "const_type": const_type, "defaults": defaults, "validations": validations}


# Ways a Validate() body is allowed to build an error. The scraper reads some of
# these as field rules and knows the rest carry none. Anything outside this set is
# a refusal, because an unread constructor harvests nothing and nothing is exactly
# what a green check looks like: the config package moved to errPathf helpers and
# every rule in it went invisible at once, while the pages still rendered.
KNOWN_ERR_CTORS = {
    "errors.New", "errors.Errorf",          # read as field rules
    "errors.Wrap", "errors.Wrapf",          # wrap a nested Validate(), no rule
    "errPath", "errPathIndex",              # wrap a nested error, no rule
    "errPathf", "errPathIndexf",            # both read above
    "fmt.Errorf", "fmt.Sprintf",            # message construction, no path segment
}

# Structs whose Validate() legitimately yields no field rule: they check
# cross-references and delegate to nested Validate() calls rather than rejecting a
# field's own value. Asserted in test-refgen.py, so a struct cannot go silent.
# Validate() bodies delegate some checks to helpers on the same receiver, and the
# scraper does not follow them: the path segment lives at the call site and the
# message inside the helper, so the rule cannot be reassembled reliably. These are
# the helpers that exist today. A new one is a refusal, because its rules would go
# unread and required-ness, constraints, and fingerprints would silently drift.
#
# evm.ics26Router is the live example: validateICS26Router requires it, but only
# when the caller passes the flag, so the page's "optional" is deliberate rather
# than a miss. Do not "fix" it by asserting required.
KNOWN_VALIDATE_HELPERS = {
    "validateAutoRelay", "validateChainOverrides", "validateChainReferences",
    "validateConnectionSigners", "validateConnections", "validateICS26Router",
    "validateRunnable",
}

RULELESS_VALIDATORS = {
    "Attestors", "Config", "ServerConfig",   # check cross-references, delegate the rest
    "AttestationParams",                     # `return nil`, satisfies an interface
}


def _const_value(path, name):
    """Read a named Go constant's value out of the file that declares it."""
    src = _read(path)
    m = re.search(r"\b" + name + r"\s*=\s*([^\n]+)", src)
    if not m:
        raise SourceError(f"constant {name} not found in {path}")
    raw = m.group(1).strip().rstrip(",")
    line = src[:m.start()].count("\n") + 1
    d = re.match(r"(\d+)\s*\*\s*time\.(Second|Minute|Hour)", raw)
    if d:
        return f"{d.group(1)}{d.group(2)[0].lower()}", line
    d = re.match(r"time\.(Second|Minute|Hour)$", raw)
    if d:
        return f"1{d.group(1)[0].lower()}", line
    return raw, line


def _element_type(go_type, model):
    """The struct a field leads to, or None. Strips pointers, slices, and the
    named list aliases the config package uses (`Attestors` is []AttestorConfig)."""
    t = go_type.lstrip("*").removeprefix("[]").lstrip("*")
    t = model["aliases"].get(t, t)
    return t if t in model["structs"] else None


def _is_list(go_type, model):
    t = go_type.lstrip("*")
    return t.startswith("[]") or model["aliases"].get(t, "").startswith("") and t in model["aliases"]


def _discriminator(struct, model):
    """The field whose value decides which other keys apply, and its values.

    A block like `attestors` holds two shapes behind one struct, and a reader
    of the local shape should never meet a remote-only key. Detected rather
    than declared: a field with two or more known values is one.
    """
    for field in model["structs"][struct]["fields"]:
        values = sorted(v for c, v in model["consts"].items()
                        if model["const_type"].get(c) == field["type"].lstrip("*"))
        if len(values) < 2:
            for msg, args in model["validations"].get(struct, []):
                if msg.lstrip(".").split()[0] == field["yaml"] and "must be one of" in msg:
                    values = sorted(model["consts"][a] for a in args if a in model["consts"])
        if len(values) >= 2:
            return field, values
    return None, []


def _applies(struct, field, model, value):
    """Whether a key belongs in the table for one discriminator value."""
    key = field["yaml"]
    for msg, _a in model["validations"].get(struct, []):
        body = msg.lstrip(".")
        if not body.startswith(key + " "):
            continue
        rest = body[len(key):].strip()
        m = re.match(r"required for (?:type: )?(\w+)", rest)
        if m and m.group(1) != value:
            return False
        if rest.startswith("must not be set for") and rest.split()[-2] == value:
            return False
    return True


def discover_config_sections(model):
    """Every table the page needs, by three rules and no list.

    One table per top-level block. A nested struct flattens into its parent
    with a dotted key; a list of structs gets its own table. A block with a
    discriminator splits into one table per value of it.
    """
    if CONFIG_ROOT not in model["structs"]:
        raise SourceError(f"the config package has no {CONFIG_ROOT} struct to start from")
    out = []

    def walk(region, struct, parent, prefix=""):
        rows, nested, siblings = [], [], {}
        for field in model["structs"][struct]["fields"]:
            if (struct, field["go"]) in SKIP_FIELDS:
                continue
            child = _element_type(field["type"], model)
            if child and field["type"].lstrip("*").startswith(("[]",)) or (
                    child and model["aliases"].get(field["type"].lstrip("*"))):
                nested.append((f"{region}:{field['yaml']}", child,
                               (struct, field["yaml"]), f"{field['yaml']}[]."))
            elif child:
                siblings.setdefault(child, []).append(field["yaml"])
            else:
                rows.append((struct, field, f"{prefix}{field['yaml']}", parent))
        # clientA and clientB are the same shape, so they are one set of rows
        for child, names in siblings.items():
            rows.extend(walk_rows(child, [f"{prefix}{n}." for n in names],
                                  (struct, names[0])))

        field, values = _discriminator(struct, model)
        if values:
            per_value = {v: [r for r in rows if _applies(r[0], r[1], model, v)]
                         for v in values}
            # a two-valued key that gates nothing is not a discriminator: db.type
            # picks a backend, it does not change which keys exist
            if len({tuple(k for _s, _f, k, _p in rs) for rs in per_value.values()}) == 1:
                values = []
        if values:
            for value in values:
                out.append({"region": f"{region}:{value}", "struct": struct,
                            "parent": parent, "rows": per_value[value],
                            "discriminator": (field, value)})
        else:
            out.append({"region": region, "struct": struct, "parent": parent,
                        "rows": rows, "discriminator": None})
        for args in nested:
            walk(*args)

    def walk_rows(struct, prefixes, parent=None):
        """Rows for a flattened struct. Several prefixes mean several fields
        share this shape, and one row names them all."""
        if isinstance(prefixes, str):
            prefixes = [prefixes]
        rows = []
        for field in model["structs"][struct]["fields"]:
            if (struct, field["go"]) in SKIP_FIELDS:
                continue
            child = _element_type(field["type"], model)
            if child and not model["aliases"].get(field["type"].lstrip("*")) \
                    and not field["type"].lstrip("*").startswith("[]"):
                rows.extend(walk_rows(child, [f"{p}{field['yaml']}." for p in prefixes],
                                      (struct, field["yaml"])))
            else:
                rows.append((struct, field,
                             ", ".join(f"{p}{field['yaml']}" for p in prefixes), parent))
        return rows

    for field in model["structs"][CONFIG_ROOT]["fields"]:
        child = _element_type(field["type"], model)
        if not child:
            raise SourceError(f"top-level key {field['yaml']} is not a block; "
                              "the page has no shape for a scalar there")
        walk(f"config:{field['yaml']}", child, (CONFIG_ROOT, field["yaml"]))
    return out


def _fingerprint(struct, field, model):
    """Hash the source a hand-written description depends on.

    A description is only as good as the code it describes, and a key whose
    meaning changes under a stable name moves no table cell, so nothing else
    here would notice. This covers the field's type, its yaml key, and every
    validation rule naming it: enough to catch a real change, and blind to
    whitespace and to code elsewhere in the struct.
    """
    rules = sorted(msg for msg, _a in model["validations"].get(struct, [])
                   if msg.lstrip(".").split()[0].split("[")[0] == field["yaml"])
    basis = "|".join([field["type"], field["yaml"], *rules])
    return hashlib.sha1(basis.encode()).hexdigest()[:8]


def _clean_doc(field):
    """A Go field comment, read as a sentence about the key.

    Go comments open with the field's own name and often restate the
    required-ness that already has its own column, so both come off.
    """
    doc = field["doc"]
    if not doc:
        return ""
    if doc.startswith(field["go"]):
        doc = doc[len(field["go"]):].strip()
    doc = re.sub(r"^is\s+", "", doc)
    doc = re.sub(r"^(required|optional)[^.]*?(?:--|—)\s*", "", doc)
    doc = re.sub(r"^required for [^.]*\.\s*", "", doc)
    doc = re.sub(r"^(local|remote) only\.\s*", "", doc)
    doc = re.sub(r'"([^"]+)"', r"`\1`", doc)
    if re.fullmatch(r"\[.*\]", doc):
        return ""
    if not doc:
        return ""
    doc = doc[0].upper() + doc[1:]
    if not doc.endswith("."):
        doc += "."
    return doc


def _type_cell(go, field, model):
    """The Type column: a Go type, or the values a constrained key accepts."""
    t = field["type"].lstrip("*")
    named = model["const_type"]
    values = [v for c, v in model["consts"].items() if named.get(c) == t]
    if values:
        return " | ".join(f"`{v}`" for v in sorted(values))
    for msg, args in model["validations"].get(go, []):
        if msg.lstrip(".").split()[0] == field["yaml"] and "must be one of" in msg:
            resolved = [model["consts"][a] for a in args if a in model["consts"]]
            if resolved:
                return " | ".join(f"`{v}`" for v in resolved)
    if t in GO_TYPES:
        return f"`{GO_TYPES[t]}`"
    if t.startswith("[]"):
        return "list"
    if t in model["structs"] or t in model["aliases"]:
        return "block"
    return f"`{t}`"


def _requirement(go, field, model, parent=None):
    """The Default-or-required column.

    A key with a default is never the reader's to supply, so a default wins
    over a validation rule. Required-ness itself is read out of the Validate
    methods, which is where this codebase keeps it.
    """
    key = field["yaml"]
    if (go, field["go"]) in model["defaults"]:
        return f"`{model['defaults'][(go, field['go'])]}`", None
    if (go, field["go"]) in DEFAULT_CONSTS:
        parts, cites = [], []
        for label, path, const in DEFAULT_CONSTS[(go, field["go"])]:
            value, line = _const_value(path, const)
            parts.append(f"`{value}` ({label})" if label else f"`{value}`")
            cites.append((path, line))
        return ", ".join(parts), cites

    sources = [(go, key)]
    if parent:
        sources.append((parent[0], f"{parent[1]}.{key}"))
    rules = []
    for owner, path in sources:
        for msg, _args in model["validations"].get(owner, []):
            body = msg.lstrip(".")
            if body.startswith(path + " ") or body == path:
                rules.append(body[len(path):].strip())

    # a `required for X` rule outranks a `must not be set for Y` rule: both say
    # the key belongs to one kind, and only the first says it is mandatory
    for rest in rules:
        m = re.match(r"required(?: for (?:type: )?(\w+))?", rest)
        if m:
            return ("**required**" if not m.group(1) else f"**required** for `{m.group(1)}`"), None
    for rest in rules:
        if rest.startswith("must not be set for"):
            return f"`{_other_kind(go, rest.split()[-2], model)}` only", None
        if ("unknown" in rest and "type" in rest) or "must be one of" in rest:
            return "**required**", None

    # a nested struct whose own Validate requires something is itself required
    nested = field["type"].lstrip("*")
    for msg, _a in model["validations"].get(nested, []):
        if " required" in msg or msg.endswith("required"):
            return "**required**", None
    return "optional", None


def _other_kind(go, kind, model):
    """The other value of the struct's discriminator field.

    A `must not be set for local` rule means the key belongs to a remote
    entry, so the column has to name the opposite of what the rule says.
    """
    for field in model["structs"][go]["fields"]:
        values = sorted(v for c, v in model["consts"].items()
                        if model["const_type"].get(c) == field["type"].lstrip("*"))
        if kind in values and len(values) == 2:
            return [v for v in values if v != kind][0]
    raise SourceError(f"{go}: cannot tell what the opposite of {kind!r} is")


def _description(struct, field, model, seen):
    """The Description cell, and the four checks that keep it honest.

    A key documented in the source uses that; a key the source leaves
    undocumented uses FALLBACK_DOCS, whose entry carries a fingerprint of the
    code it describes. Both, neither, or a fingerprint that no longer matches
    all raise, because each of those is a description nobody has re-read.
    """
    doc = _clean_doc(field)
    fallback = FALLBACK_DOCS.get((struct, field["go"]))
    where = f"{struct}.{field['go']}"
    if doc and fallback:
        _problem("stale_fallback",
                 f"{where} now has a doc comment; drop its FALLBACK_DOCS entry",
                 field=where)
        return doc
    if not doc and not fallback:
        _problem("missing_description",
                 f"{where} has no doc comment and no FALLBACK_DOCS entry",
                 field=where, yaml_key=field["yaml"],
                 fingerprint=_fingerprint(struct, field, model))
        return "TODO: describe this key"
    if not fallback:
        return doc
    text, recorded = fallback
    seen.add((struct, field["go"]))
    current = _fingerprint(struct, field, model)
    if current != recorded:
        _problem("fingerprint_mismatch",
                 f"{where}: the source behind its hand-written description changed "
                 f"(fingerprint {recorded} -> {current}). Re-read \"{text}\" against the "
                 "code, then record the new fingerprint. Nothing is written until you do.",
                 field=where, description=text, was=recorded, now=current)
    return text


def _root_table(model, seen):
    """The index of top-level blocks: what each is and which processes read it.

    Type and default mean nothing for a key whose value is a whole block, so
    this table carries the two things a reader wants instead.
    """
    fields = model["structs"][CONFIG_ROOT]["fields"]
    keys = [f["yaml"] for f in fields]
    missing = [k for k in keys if k not in READ_BY]
    read_by = dict(READ_BY)   # never mutate the module's copy: plan mode runs
    if missing:                # in the same process as everything else
        _problem("missing_read_by",
                 "READ_BY has no entry for the top-level block(s): " + ", ".join(missing),
                 blocks=missing)
        for k in missing:
            read_by[k] = "TODO: which processes read this"
    gone = [k for k in READ_BY if k not in keys]
    if gone:
        _problem("dead_read_by",
                 "READ_BY names top-level blocks that are gone: " + ", ".join(gone),
                 blocks=gone)
    rows = [(f"`{f['yaml']}`", read_by[f["yaml"]], _description(CONFIG_ROOT, f, model, seen))
            for f in fields]
    info = model["structs"][CONFIG_ROOT]
    return (table(["Block", "Read by", "Purpose"], rows)
            + "\n\n" + cite(info["file"], info["line"]))


def gen_config():
    model = parse_go_config()
    blocks, seen_fallbacks = {}, set()
    for sec in discover_config_sections(model):
        rows, cites = [], []
        for struct, field, key, parent in sec["rows"]:
            description = _description(struct, field, model, seen_fallbacks)
            if sec["discriminator"] and field is sec["discriminator"][0]:
                # the key that names this table: its value is the heading
                rows.append((f"`{key}`", f"`{sec['discriminator'][1]}`",
                             "**required**", description))
                continue
            req, extra = _requirement(struct, field, model, parent)
            if extra:
                cites.extend(extra)
            if sec["discriminator"]:
                # this table is already about one kind, so the condition that
                # named that kind says nothing a reader here needs
                value = sec["discriminator"][1]
                req = {f"**required** for `{value}`": "**required**",
                       f"`{value}` only": "optional"}.get(req, req)
            rows.append((f"`{key}`", _type_cell(struct, field, model), req, description))
        body = table(["Key", "Type", "Default or required", "Description"], rows)
        info = model["structs"][sec["struct"]]
        body += "\n\n" + cite(info["file"], info["line"])
        for path, line in dict.fromkeys(cites):
            body += " " + cite(path, line)
        blocks[sec["region"]] = body

    reachable = {(st, f["go"]) for sec in discover_config_sections(model)
                 for st, f, _k, _p in sec["rows"]}
    orphans = sorted(f"{s}.{f}" for s, f in set(FALLBACK_DOCS) - seen_fallbacks)
    if orphans:
        _problem("dead_description",
                 "FALLBACK_DOCS describes fields that are gone: " + ", ".join(orphans),
                 fields=orphans)
    dead_defaults = sorted(f"{s}.{f}" for s, f in set(DEFAULT_CONSTS) - reachable)
    if dead_defaults:
        _problem("dead_default",
                 "DEFAULT_CONSTS names fields that are gone: " + ", ".join(dead_defaults),
                 fields=dead_defaults)
    unclaimed = sorted(
        f"{st}.{f['go']}" for sec in discover_config_sections(model)
        for st, f, _k, _p in sec["rows"]
        if f["type"].startswith("*") and not _element_type(f["type"], model)
        and (st, f["go"]) not in DEFAULT_CONSTS
        and (st, f["go"]) not in NO_NAMED_DEFAULT
        and (st, f["go"]) not in SKIP_FIELDS)
    if unclaimed:
        _problem("unclaimed_default",
                 "these pointer fields have no default mapped and are not listed in "
                 "NO_NAMED_DEFAULT, so the page would call them optional without "
                 "saying what unset means: " + ", ".join(unclaimed),
                 fields=unclaimed)
    return blocks


# ----------------------------------------------------------------- cli -> page

CLI_DIR = "cli"
CLI_SRC = "cli/cmd/ibc"

# The root command's own flags are declared here rather than in main.go.
GLOBAL_FLAGS_FILE = "cli/internal/config/flags.go"
CLI_BIN = "cli/bin/ibc"

# Cobra generates these and nobody reads a page about them. Excluded here so
# the coverage assertion below still accounts for every other command.
CLI_EXCLUDED = {"completion", "help"}

# Sections follow the command tree, so nothing is scattered and a reader's
# muscle memory transfers to `ibc <group> --help`. Membership is discovered;
# only the order is a human's call, and it is the order a reader meets them in.
# A group missing from this list raises, so a new one cannot go undocumented.
CLI_SECTION_ORDER = ["config", "keys", "deploy", "relayer", "attestor",
                     "tx", "query", "migrate"]

def build_cli():
    """Build the binary, because Cobra computes a flag's default when the flag
    is registered, so the honest source for defaults is `--help` itself.

    `--help` prints and exits, so this needs a Go toolchain and nothing else:
    no chains, no config, no network.
    """
    out = os.path.join(IBC, CLI_BIN)
    if os.environ.get("REFGEN_NO_BUILD") and os.path.exists(out):
        return out
    r = subprocess.run(["go", "build", "-o", "bin/ibc", "./cmd/ibc/..."],
                       cwd=os.path.join(IBC, CLI_DIR), capture_output=True, text=True)
    if r.returncode != 0:
        raise SourceError(f"go build failed:\n{r.stderr}")
    return out


def _cli_help(binary, path):
    r = subprocess.run([binary] + path + ["--help"], capture_output=True, text=True)
    if r.returncode != 0:
        raise SourceError(f"ibc {' '.join(path)} --help failed:\n{r.stderr}")
    return r.stdout


FLAG_LINE = re.compile(r"^\s{2,}(?:-(\w), )?--([\w-]+)(?: (\w+))?\s{2,}(.*)$")


def _parse_flags(help_text, section):
    """Flags from one section of a --help page.

    A flag with no type word is a bool, which is how Cobra prints one. A
    trailing `(default ...)` is Cobra's; a trailing `(default: ...)` is the
    flag author's own words. Both are the flag's default, so both move out of
    the description and into their own column.
    """
    lines, keep, out = help_text.split("\n"), False, []
    for line in lines:
        if line.rstrip().endswith("Flags:"):
            keep = line.strip() == section
            continue
        if not line.strip():
            continue
        m = FLAG_LINE.match(line)
        if not keep:
            continue
        if m:
            out.append({"short": m.group(1), "name": m.group(2),
                        "type": m.group(3) or "bool", "doc": m.group(4).strip()})
        elif out and line.startswith("  "):
            out[-1]["doc"] += " " + line.strip()
    for f in out:
        d = re.search(r"\s*\(default:?\s+(.+)\)$", f["doc"])
        f["default"] = d.group(1).strip('"') if d else ""
        if d:
            f["doc"] = f["doc"][:d.start()].rstrip()
    return [f for f in out if f["name"] != "help"]


def walk_cli(binary):
    """The command tree, as the binary reports it: {path: {short, subs}}."""
    tree = {}

    def visit(path):
        text = _cli_help(binary, path)
        short = text.split("\n", 1)[0].strip()
        subs = []
        m = re.search(r"Available Commands:\n((?:  \S.*\n)+)", text)
        if m:
            for line in m.group(1).strip("\n").split("\n"):
                name = line.strip().split()[0]
                if name in CLI_EXCLUDED:
                    continue
                subs.append(name)
        tree[" ".join(path)] = {"short": short, "subs": subs, "help": text,
                                "flags": _parse_flags(text, "Flags:")}
        for name in subs:
            visit(path + [name])

    visit([])
    return tree


def parse_cli_source():
    """Required flags, which `--help` does not print.

    Cobra keeps required-ness in an annotation set by MarkFlagRequired, so it
    is read out of the command wiring the same way config required-ness is
    read out of the Validate methods.
    """
    # declarations are spread across the package; every AddCommand, flag
    # registration, and MarkFlagRequired lives in main.go, and citations point
    # there, so the wiring is parsed on its own to keep line numbers true
    src = "\n".join(open(os.path.join(IBC, CLI_SRC, f)).read()
                    for f in sorted(os.listdir(os.path.join(IBC, CLI_SRC)))
                    if f.endswith(".go") and not f.endswith("_test.go"))
    wiring = open(os.path.join(IBC, CLI_SRC, "main.go")).read()

    consts = {m.group(1): m.group(2)
              for m in re.finditer(r'(\w+)\s*=\s*"([\w-]+)"', src)}

    use = {}
    for m in re.finditer(r"(cmd\w+)\s*=\s*&cobra\.Command\{", src):
        tail = src[m.end():m.end() + 400]
        u = re.search(r'Use:\s*(?:"([\w-]+)|(\w+))', tail)
        if u:
            # Use: is sometimes a const shared by two commands, as with
            # useStatus, so resolve identifiers through the const table
            use[m.group(1)] = u.group(1) or consts.get(u.group(2), u.group(2))

    parent = {}
    for m in re.finditer(r"(cmd\w+|rootCmd)\.AddCommand\(", wiring):
        depth, i = 1, m.end()
        while depth:
            depth += {"(": 1, ")": -1}.get(wiring[i], 0)
            i += 1
        for child in re.findall(r"cmd\w+", wiring[m.end():i - 1]):
            parent[child] = m.group(1)

    def path_of(var):
        parts = []
        while var in use:
            parts.insert(0, use[var])
            var = parent.get(var, "")
        return " ".join(parts)

    lines = {}
    root = re.search(r"rootCmd\.AddCommand\(", wiring)
    if not root:
        raise SourceError("main.go no longer assembles the tree with rootCmd.AddCommand")
    # the root's own flags are declared in the config package, so citing
    # main.go's AddCommand for them points a reader at the wrong file
    lines[""] = wiring[:root.start()].count("\n") + 1
    flags_src = _read(GLOBAL_FLAGS_FILE)
    decl = re.search(r"func DeclarePersistentFlags\(", flags_src)
    if not decl:
        raise SourceError(f"{GLOBAL_FLAGS_FILE} no longer declares the persistent flags")
    lines["__global__"] = flags_src[:decl.start()].count("\n") + 1
    for var, path in ((v, path_of(v)) for v in use):
        m = re.search(r"\b" + var + r"\.(?:Persistent)?Flags\(\)", wiring)
        if m:
            lines[path] = wiring[:m.start()].count("\n") + 1

    required = {}
    for m in re.finditer(r"(cmd\w+)\.Mark(?:Persistent)?FlagRequired\((\"[\w-]+\"|\w+)\)", wiring):
        raw = m.group(2)
        if not raw.startswith('"'):
            if raw not in consts:
                continue          # a loop variable; the loop forms handle those
            flag = consts[raw]
        else:
            flag = raw.strip('"')
        required.setdefault(path_of(m.group(1)), set()).add(flag)

    # the loop form: for _, req := range []string{...} { cmdX.Mark...(req) }
    for m in re.finditer(r"for _, req := range \[\]string\{([^}]*)\}\s*\{([^}]*)\}", wiring):
        names = [consts.get(n.strip().strip('"'), n.strip().strip('"'))
                 for n in m.group(1).split(",") if n.strip()]
        c = re.search(r"(cmd\w+)\.Mark(?:Persistent)?FlagRequired", m.group(2))
        if c:
            required.setdefault(path_of(c.group(1)), set()).update(names)

    for m in re.finditer(r"for _, c := range \[\]\*cobra\.Command\{([^}]*)\}\s*\{", wiring):
        cmds = re.findall(r"cmd\w+", m.group(1))
        depth, i = 1, m.end()
        while depth and i < len(wiring):
            depth += {"{": 1, "}": -1}.get(wiring[i], 0)
            i += 1
        body = wiring[m.end():i]
        flags = [f.strip('"') for f in
                 re.findall(r"Mark(?:Persistent)?FlagRequired\(\"([\w-]+)\"\)", body)]
        loop_line = wiring[:m.start()].count("\n") + 1
        for var in cmds:
            required.setdefault(path_of(var), set()).update(flags)
            # flags registered inside the loop, so cite the loop
            lines.setdefault(path_of(var), loop_line)

    return {"paths": {v: path_of(v) for v in use}, "required": required,
            "lines": lines}


def _flag_name(f):
    """The flag as a reader types it, with its type in the signature.

    A separate Type column spends a column on five characters. Cobra already
    writes the type after the flag, and a bool takes no value at all.
    """
    lead = f"-{f['short']}, --{f['name']}" if f["short"] else f"--{f['name']}"
    return f"`{lead}`" if f["type"] == "bool" else f"`{lead} <{f['type']}>`"


def _flag_rows(flags, path, source):
    """Flag, Default, Description. Required-ness shows in the Default column,
    because a required flag is precisely one with no default."""
    required = source["required"].get(path, set())
    rows = []
    for f in flags:
        if f["name"] in required:
            default = "required"
        elif f["default"] in ("", "false", "[]", "0"):
            default = ""
        elif " " in f["default"]:
            default = _prose(f["default"])   # a phrase, so only its tokens fence
        else:
            default = f"`{f['default']}`"
        doc = _prose(f["doc"]).rstrip(".")
        rows.append((_flag_name(f), default,
                     (doc[0].upper() + doc[1:] + "." if doc else "")))
    return rows


FLAG_COLUMNS = ["Flag", "Default", "Description"]


def _prose(text):
    """A Cobra description, safe and readable in a table cell.

    A bare `<ibc-home>` is a tag to MDX, and a bare `--amount` reads as prose
    when it is a flag, so both become inline code.
    """
    return re.sub(r"(?<![`\w])((?:--[\w-]+)|(?:[\w<>/-]*<[\w-]+>[\w<>/-]*))",
                  r"`\1`", text)


def _slug(path):
    return path.replace(" ", "-")


def gen_cli():
    binary = build_cli()
    tree = walk_cli(binary)
    source = parse_cli_source()
    blocks = {}

    leaves = sorted(p for p, c in tree.items() if p and not c["subs"])
    # the tree the wiring describes and the tree the binary reports must agree
    wired = {p for p in source["paths"].values() if p}
    absent = sorted(set(leaves) - wired)
    if absent:
        _problem("unwired_command",
                 f"commands the binary has and the wiring does not: {absent}",
                 commands=absent)

    main_go = os.path.join(CLI_SRC, "main.go")

    def where(path):
        """Cite the line in main.go that registers this command's flags, or
        the line that assembles the tree for a table that spans commands."""
        return cite(main_go, source["lines"].get(path, source["lines"][""]))

    tree_citation = where("")

    blocks["cli:global-flags"] = (
        table(FLAG_COLUMNS,
              _flag_rows(_parse_flags(tree[""]["help"], "Flags:"), "", source))
        + "\n\n" + cite(GLOBAL_FLAGS_FILE, source["lines"]["__global__"]))

    groups = {}
    for path in leaves:
        groups.setdefault(path.split()[0], []).append(path)

    missing_order = sorted(set(groups) - set(CLI_SECTION_ORDER))
    if missing_order:
        _problem("ungrouped_command",
                 "these command groups are not in CLI_SECTION_ORDER, so they would "
                 f"have no section: {missing_order}",
                 groups=missing_order,
                 commands={g: groups[g] for g in missing_order})
    gone = [g for g in CLI_SECTION_ORDER if g not in groups]
    if gone:
        _problem("dead_section",
                 f"CLI_SECTION_ORDER names groups the binary does not have: {gone}",
                 groups=gone)

    # A command's section lists every flag it accepts, its own first and the
    # ones inherited from its parents after, so a reader who arrives at one
    # command sees the whole invocation. Group flags are repeated rather than
    # tabled once: five rows across eight commands costs about twenty lines and
    # removes the need for a threshold rule, a fold rule, and a cross-reference.
    # Root flags are the exception, stated once at the top: they apply to all
    # 28 commands, and inlining them would add 140 rows.
    def inherited_by(path):
        return [(node, f) for node, node_tree in tree.items()
                if node and node_tree["subs"] and path.startswith(node + " ")
                for f in node_tree["flags"]]

    for path in leaves:
        rows = sorted(_flag_rows(tree[path]["flags"], path, source))
        for node, f in sorted(inherited_by(path), key=lambda e: e[1]["name"]):
            rows += _flag_rows([f], node, source)
        body = _prose(tree[path]["short"]).rstrip(".") + "."
        if rows:
            body += "\n\n" + table(FLAG_COLUMNS, rows)
        blocks[f"cli:cmd:{_slug(path)}"] = body + "\n\n" + where(path)

    # Every flag a command accepts appears in its own table or in an inherited
    # one. Nothing may be documented nowhere.
    # Every flag of every node reaches a table: a leaf's own flags in its own
    # section, a group's flags in each of its commands, the root's at the top.
    for node, node_tree in tree.items():
        if not node_tree["flags"]:
            continue
        if not node:
            continue                                   # the global table
        under = [p for p in leaves if p == node or p.startswith(node + " ")]
        if not under:
            _problem("undocumented_flag",
                     f"`ibc {node}` has flags and no command under it, so they "
                     "appear in no table",
                     node=node, flags=[f["name"] for f in node_tree["flags"]])

    for region, body in blocks.items():
        if "IBC Link" in body:
            raise SourceError(f"{region} carries the retired product name; the page cannot")
    return blocks


GENERATORS = {"api": gen_api, "cli": gen_cli, "config": gen_config}


# The page each generator owns. A page named here must carry a marker for
# every region its generator produces, so a new region cannot land in the
# generator and go missing from the page.
PAGES = {"config": "docs/6-ibc-cli/5-configuration.md",
         "cli": "docs/6-ibc-cli/6-cli-commands.md",
         "api": "docs/6-ibc-cli/7-api.md"}


def stale_regions(kind, path):
    """Region ids whose generated body differs from the page, and nothing else.

    The prose pass needs to know which tables moved, so it can re-read the
    sentences next to those and leave the rest of the page alone.
    """
    blocks = GENERATORS[kind]()
    text = open(path).read()
    regions = find_regions(text)
    stale = []
    for ident, bs, be, _os_, _oe in regions:
        if ident in blocks and text[bs:be].strip() != blocks[ident].strip():
            stale.append(ident)
    return stale


def plan(kind, path):
    """Everything a human or an agent needs to bring one page back in line.

    Deterministic and side-effect free. It reports four kinds of work:

      stale            a table on the page no longer matches the source
      missing_marker   a region the source now has and the page has nowhere to
                       put, with the table already rendered and a suggested
                       heading and insertion point
      orphaned_marker  a marker for a region the source no longer has
      curation         a choice only a person can make: a description the code
                       does not carry, a command in no task group, a
                       fingerprint that no longer matches

    The tool stops at proposing. It never writes a heading or a sentence into a
    page, because every word a reader sees should have been through a review.
    """
    global PLAN
    PLAN = []
    try:
        blocks = GENERATORS[kind]()
    finally:
        curation, PLAN = PLAN, None

    text = open(path).read()
    regions = find_regions(text)
    present = [i for i, *_ in regions]
    order = list(blocks)

    def after(region):
        """The last region already on the page that precedes this one in source
        order, which is where a new section belongs."""
        i = order.index(region)
        earlier = [r for r in order[:i] if r in present]
        return earlier[-1] if earlier else None

    return {
        "page": os.path.relpath(path, ROOT) if path.startswith(ROOT) else path,
        "kind": kind,
        "stale": [i for i in present
                  if i in blocks and _body(text, regions, i) != blocks[i].strip()],
        "missing_marker": [{
            "region": r,
            "suggested_heading": _suggest_heading(r),
            "insert_after": after(r),
            "table": blocks[r],
        } for r in order if r not in present],
        "orphaned_marker": [{
            "region": r,
            "reason": "the source no longer has what this region described",
        } for r in present if r not in blocks],
        "curation": curation,
    }


def _body(text, regions, ident):
    for i, bs, be, _os_, _oe in regions:
        if i == ident:
            return text[bs:be].strip()
    return None


def _suggest_heading(region):
    """A heading a writer will probably keep, derived from the region id. The
    words are a suggestion; the writer owns them."""
    parts = region.split(":")
    if parts[0] == "config":
        name = re.sub(r"Config$", "", parts[-1])
        return "### `" + name[0].lower() + name[1:] + "`"
    if parts[0] == "api" and parts[1] == "msg":
        return f"#### `{parts[-1]}`"
    if parts[0] == "api" and parts[1] == "enum":
        return f"#### `{parts[-1]}`"
    if parts[:2] == ["cli", "flags"]:
        return "### `ibc " + parts[-1].replace("-", " ") + "`"
    if parts[:2] == ["cli", "group-flags"]:
        return "### Flags every `" + parts[-1].replace("-", " ") + "` command accepts"
    return f"## {parts[-1]}"


def run(kind, path, check):
    blocks = GENERATORS[kind]()
    text = open(path).read()
    present = {i for i, *_ in find_regions(text)}
    if os.path.normpath(path) == os.path.normpath(os.path.join(ROOT, PAGES[kind])) or \
            os.path.normpath(path) == PAGES[kind]:
        absent = sorted(set(blocks) - present)
        if absent:
            raise MarkerError("the page is missing a marker for: " + ", ".join(absent))
    wanted = {k: v for k, v in blocks.items() if k in present}
    new = render(text, wanted)
    if new == text:
        print(f"{path}: up to date ({len(wanted)} regions)")
        return 0
    if check:
        print(f"{path}: STALE")
        sys.stdout.writelines(difflib.unified_diff(
            text.splitlines(True), new.splitlines(True), "on disk", "generated"))
        return 1
    open(path, "w").write(new)
    print(f"{path}: regenerated {len(wanted)} regions")
    return 0


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("kind", choices=sorted(GENERATORS) + ["all"])
    ap.add_argument("page", nargs="?", help="omit for kind `all`")
    ap.add_argument("--check", action="store_true")
    ap.add_argument("--list-regions", action="store_true",
                    help="print the stale region ids, one per line, and nothing else")
    ap.add_argument("--plan", action="store_true",
                    help="print, as JSON, every gap and the work each one needs")
    a = ap.parse_args()
    jobs = [(k, os.path.join(ROOT, p)) for k, p in sorted(PAGES.items())] \
        if a.kind == "all" else [(a.kind, a.page)]
    if a.kind != "all" and not a.page:
        ap.error("a page is required unless kind is `all`")
    rc, plans = 0, []
    for kind, page in jobs:
        try:
            if a.plan:
                plans.append(plan(kind, page))
                continue
            if a.list_regions:
                stale = stale_regions(kind, page)
                for ident in stale:
                    print(ident)
                rc = max(rc, 1 if stale else 0)
                continue
            rc = max(rc, run(kind, page, a.check))
        except (MarkerError, SourceError) as e:
            print(f"{page}: {e}", file=sys.stderr)
            rc = max(rc, 2)
    if a.plan:
        print(json.dumps(plans, indent=2))
        work = sum(len(p["stale"]) + len(p["missing_marker"])
                   + len(p["orphaned_marker"]) + len(p["curation"]) for p in plans)
        rc = 1 if work else 0
    return rc


if __name__ == "__main__":
    sys.exit(main())
