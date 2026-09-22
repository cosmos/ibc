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
import shutil
import subprocess
import sys
import tempfile

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


FRONTMATTER = re.compile(r"\A---\r?\n.*?\r?\n---\r?\n", re.S)


def _check_notice_placement(text, path):
    """The notice must not sit above a page's frontmatter.

    Frontmatter is only frontmatter when it starts at the first byte. A marker
    pair above it turns `title:` into body text and the `---` into a rule, so
    the page loses its title rather than gaining a notice.
    """
    if FRONTMATTER.match(text):
        return                      # frontmatter is where it must be
    without = re.sub(r"\s*<!-- GEN:notice START -->.*?<!-- GEN:notice END -->\s*",
                     "", text, flags=re.S).lstrip()
    if FRONTMATTER.match(without):
        raise MarkerError(
            f"{path}: the GEN:notice marker sits above the page's frontmatter, "
            "which stops the frontmatter being frontmatter -- the title becomes "
            "body text. Move the marker pair below the closing `---`.")


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
    open_, close = COMMENT
    if start is None:
        # the line could not be located; the file still points a reader at the
        # right place, and a citation is never worth failing a run over
        return f"{open_} [{os.path.basename(path)}]({path}) {close}"
    rng = f"L{start}" if end is None or end == start else f"L{start}-L{end}"
    return f"{open_} [{os.path.basename(path)}:{rng}]({path}#{rng}) {close}"


_DESCRIPTOR = {}

# scalar type enum -> the word the .proto author wrote
_PROTO_SCALARS = {
    "TYPE_DOUBLE": "double", "TYPE_FLOAT": "float", "TYPE_INT64": "int64",
    "TYPE_UINT64": "uint64", "TYPE_INT32": "int32", "TYPE_FIXED64": "fixed64",
    "TYPE_FIXED32": "fixed32", "TYPE_BOOL": "bool", "TYPE_STRING": "string",
    "TYPE_BYTES": "bytes", "TYPE_UINT32": "uint32", "TYPE_SFIXED32": "sfixed32",
    "TYPE_SFIXED64": "sfixed64", "TYPE_SINT32": "sint32", "TYPE_SINT64": "sint64",
}


def _buf_descriptor():
    """Every proto in the repo, compiled, with source info. Built once.

    Requires `buf` on PATH. It is not optional and there is no text-parsing
    fallback: a fallback that reads the schema less well than the compiler is
    how a page goes quietly wrong when the schema grows a construct.
    """
    if _DESCRIPTOR.get("__root__") == IBC:
        return _DESCRIPTOR["files"]
    if not shutil.which("buf"):
        raise SourceError(
            "`buf` is not on PATH, and the API page is generated from the "
            "descriptor set it compiles. Install it (https://buf.build/docs/"
            "installation), or run this from an environment that has it. This "
            "repository already builds its protos with buf; see "
            "`proto/buf.gen.yaml`.")
    roots = sorted({os.path.dirname(f) for f in _walk(".proto")})
    if not roots:
        raise SourceError("no .proto files found to compile")
    # buf builds a module, so hand it the directory holding buf.yaml -- the
    # nearest one at or above the protos
    module = os.path.commonpath(roots) if len(roots) > 1 else roots[0]
    while module and not any(
            os.path.exists(os.path.join(IBC, module, n))
            for n in ("buf.yaml", "buf.work.yaml", "buf.gen.yaml")):
        parent = os.path.dirname(module)
        if parent == module:
            break
        module = parent
    out = os.path.join(tempfile.gettempdir(), f"refgen-desc-{os.getpid()}.json")
    try:
        r = subprocess.run(
            ["buf", "build", ".", "-o", out + "#format=json"],
            cwd=os.path.join(IBC, module), capture_output=True, text=True,
            timeout=180)
        if r.returncode != 0:
            raise SourceError(
                f"buf build failed in {module}, so no page derived from the "
                f"schema can be written:\n{r.stderr.strip()}",
                kind="proto_build_failed")
        with open(out) as fh:
            files = json.load(fh).get("file", [])
    finally:
        if os.path.exists(out):
            os.remove(out)
    by_name = {}
    for fd in files:
        if "sourceCodeInfo" not in fd:
            raise SourceError(
                f"{fd.get('name')} came back without source info, so no comment "
                "on it can be read. `buf build` must keep source info.")
        by_name[fd["name"]] = fd
    _DESCRIPTOR.clear()
    _DESCRIPTOR.update({"__root__": IBC, "files": by_name})
    return by_name


def _descriptor_for(path):
    """The compiled file whose name matches this repo-relative .proto path."""
    files = _buf_descriptor()
    want = path.replace(os.sep, "/")
    for name, fd in files.items():
        if want.endswith(name):
            return fd
    raise SourceError(
        f"{path} is not in the descriptor set buf produced "
        f"({', '.join(sorted(files)) or 'nothing'}), so it is not part of the "
        "proto module and nothing here can read it.")


def _comments(fd):
    """{path tuple: leading comment} from the compiled file's source info."""
    out = {}
    for loc in fd["sourceCodeInfo"].get("location", []):
        lead = loc.get("leadingComments")
        if lead:
            out[tuple(loc.get("path", []))] = " ".join(lead.split())
    return out


def _line(fd, path):
    """The 1-based line the element at this descriptor path is declared on."""
    for loc in fd["sourceCodeInfo"].get("location", []):
        if tuple(loc.get("path", [])) == path and loc.get("span"):
            return loc["span"][0] + 1
    return 1


def _field_type(f, owner=None):
    """The type as the .proto author wrote it, and as a reader meets it.

    A `map<k, v>` is a repeated field of a hidden entry message in the
    descriptor. Rendering that literally published `LabelsEntry[]` -- a type
    name that appears nowhere in the schema and that no reader can act on.
    """
    t = _PROTO_SCALARS.get(f.get("type"))
    if t is None:
        t = (f.get("typeName") or "").rsplit(".", 1)[-1]
    if f.get("label") == "LABEL_REPEATED":
        entry = next((n for n in (owner or {}).get("nestedType", [])
                      if n["name"] == t and n.get("options", {}).get("mapEntry")),
                     None)
        if entry:
            by_name = {g["name"]: g for g in entry["field"]}
            return (f"map<{_field_type(by_name['key'])}, "
                    f"{_field_type(by_name['value'])}>")
        return "repeated " + t
    # proto3 `optional` is a synthetic one-field oneof in the descriptor, and
    # `proto3Optional` is what distinguishes it from every other proto3 field,
    # all of which also carry LABEL_OPTIONAL
    if f.get("proto3Optional"):
        return t + ", optional"
    return t


def parse_proto(path):
    """Services, rpcs, messages, enums with their leading comments.

    Read from the compiled descriptor, not from the file's text. Field numbers
     6, 4, 5 are protobuf's own path tags for service, message and enum; 2 is
    the member list of each.
    """
    fd = _descriptor_for(path)
    com = _comments(fd)
    out = {"services": [], "messages": [], "enums": []}

    for si, svc in enumerate(fd.get("service", [])):
        rpcs = []
        for mi, m in enumerate(svc.get("method", [])):
            if m.get("clientStreaming") or m.get("serverStreaming"):
                # the tables describe one request body and one response body.
                # A stream has neither, and rendering it in those columns would
                # read exactly like a unary call -- the descriptor can see the
                # difference even though the page has no shape for it.
                _problem("streaming_rpc",
                         f"{svc['name']}.{m['name']} streams, and the API page "
                         "has a row for a request body and a response body. "
                         "Rendering it there would read as a unary call. Give "
                         "the page a shape for streaming calls, then teach this "
                         "function to use it.",
                         service=svc["name"], rpc=m["name"],
                         file=path, line=_line(fd, (6, si, 2, mi)))
                continue
            rpcs.append({"name": m["name"],
                         "req": m["inputType"].rsplit(".", 1)[-1],
                         "resp": m["outputType"].rsplit(".", 1)[-1],
                         "doc": com.get((6, si, 2, mi), ""),
                         "line": _line(fd, (6, si, 2, mi))})
        out["services"].append({"name": svc["name"], "doc": com.get((6, si), ""),
                                "line": _line(fd, (6, si)), "rpcs": rpcs})

    for mi, msg in enumerate(fd.get("messageType", [])):
        if msg.get("options", {}).get("mapEntry"):
            continue            # the synthetic entry type behind a map<> field
        out["messages"].append({"name": msg["name"], "doc": com.get((4, mi), ""),
                                "line": _line(fd, (4, mi)),
                                "fields": _descriptor_fields(fd, com, mi, msg)})

    for ei, en in enumerate(fd.get("enumType", [])):
        out["enums"].append({
            "name": en["name"], "doc": com.get((5, ei), ""),
            "line": _line(fd, (5, ei)),
            "values": [{"name": v["name"], "doc": com.get((5, ei, 2, vi), "")}
                       for vi, v in enumerate(en.get("value", []))]})
    return out


def _descriptor_fields(fd, com, mi, msg):
    """A message's fields in declaration order, a oneof folded into one entry.

    A real oneof becomes a single row named for the oneof, listing its members,
    the way the page has always shown it. A proto3 `optional` field is also a
    oneof in the descriptor -- a synthetic one -- and is not folded, because to
    a reader it is just an optional field.
    """
    synthetic = {f["oneofIndex"] for f in msg.get("field", [])
                 if f.get("proto3Optional") and "oneofIndex" in f}
    out, seen = [], set()
    for fi, f in enumerate(msg.get("field", [])):
        oi = f.get("oneofIndex")
        if oi is not None and oi not in synthetic:
            if oi in seen:
                continue        # already emitted as part of its oneof
            seen.add(oi)
            decl = msg["oneofDecl"][oi]
            out.append({
                "name": decl["name"], "type": "oneof",
                "doc": com.get((4, mi, 8, oi), ""),
                "opts": [g["name"] for g in msg["field"]
                         if g.get("oneofIndex") == oi]})
            continue
        out.append({"name": f["name"], "type": _field_type(f, msg),
                    "doc": com.get((4, mi, 2, fi), "")})
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
    """The comment as its author wrote it, with a capital at the front.

    This used to drop the identifier a comment opens with, so a row for `Relay`
    did not read "Relay tracks the packets". It worked for exactly that shape
    and mangled every other one: `// Labels are forwarded` published as "Are
    forwarded", `// State of the packet` as "Of the packet", and a comment
    opening `WS` was missed entirely because the casing did not match. Each
    repair was another word added to a list that would have to grow forever --
    the same shape as the vocabularies this tool exists to do without.

    So it strips nothing. If a description stutters on the page, the comment in
    the schema is the thing to reword, which is where every other fact on these
    pages is fixed too.
    """
    return doc[0].upper() + doc[1:] if doc else ""


def _fence(doc, names):
    """Fence a field name used inside another field's description."""
    for n in sorted(names, key=len, reverse=True):
        doc = re.sub(r"(?<![`\w])" + re.escape(n) + r"(?![`\w])", f"`{n}`", doc)
    return doc


def _proto_files():
    """Every .proto in the repo. Naming a directory would mean a schema that
    moved goes missing from the page with nothing to notice it."""
    return _walk(".proto")


def _proto_short(path):
    """The last segment of the file's proto package, which names its regions."""
    m = re.search(r"^package\s+([\w.]+);", _read(path), re.M)
    if not m:
        raise SourceError(f"{path} declares no proto package")
    # the last segment that names something, not a version: `ibc.v2.relayer`
    # is the relayer, and `ibc.core.client.v1` is the client, not `v1`
    parts = [p for p in m.group(1).split(".") if not re.fullmatch(r"v\d+\w*", p)]
    return parts[-1] if parts else m.group(1)


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


def _field_doc(msg, field, seen, where_declared=None, at_line=None):
    """The Description cell, with the same four checks the config page uses.

    Including the one that was missing: a field with neither a comment nor an
    entry used to return an empty string and raise nothing, so a new field on a
    public API shipped with a blank Description cell and a clean check.
    """
    doc = _lead_strip(field["name"], field["doc"])
    entry = FIELD_DOCS.get((msg, field["name"]))
    where = f"{msg}.{field['name']}"
    if doc and entry:
        _problem("stale_field_doc",
                 f"{where} now has a proto comment; drop its FIELD_DOCS entry",
                 field=where, file=where_declared, line=at_line)
        return doc
    if not entry:
        if not doc:
            _problem("missing_field_description",
                     f"{where} has no proto comment and no FIELD_DOCS entry. The "
                     "better fix is a comment on the field in the schema; a "
                     "FIELD_DOCS entry is for a schema you cannot edit.",
                     field=where, declared_in=msg, proto_field=field["name"],
                     file=where_declared, line=at_line)
            return "TODO: describe this field"
        return doc
    text, recorded = entry
    seen.add((msg, field["name"]))
    current = _field_fingerprint(msg, field)
    if recorded and current != recorded:
        _problem("field_fingerprint_mismatch",
                 f"{where}: the field changed shape (fingerprint {recorded} -> {current}). "
                 f"Re-read \"{text}\" against the proto, then record the new fingerprint.",
                 field=where, description=text, was=recorded, now=current,
                 file=where_declared, line=at_line)
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
                doc = _fence(_field_doc(msg["name"], f, seen_docs, fname, msg["line"]),
                             names - {f["name"]})
                rows.append((f"`{f['name']}`", t, doc))
            blocks[f"api:msg:{msg['name']}"] = (
                table(["Field", "Type", "Description"], rows)
                + "\n\n" + cite(fname, msg["line"]))
        for en in p["enums"]:
            rows = [(f"`{v['name']}`", v["doc"]) for v in en["values"]
                    if not v["name"].endswith("UNSPECIFIED")]
            blocks[f"api:enum:{en['name']}"] = (
                table(["Value", "Meaning"], rows) + "\n\n" + cite(fname, en["line"]))

    blocks["notice"] = _notice()
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


# The one anchor. Every block on the page is a struct reachable from this type,
# so the page's shape follows the code's rather than a list kept by hand.
# Which command-line program these pages document, as a package directory,
# when the repo holds more than one. Left unset it is discovered. A repo that
# grows a second CLI cannot be asked which of them a page is about -- that is a
# choice, not a fact -- so the refusal names the candidates and points here.
CLI_PACKAGE = None

CONFIG_ROOT = "Config"

# The function that returns a populated root config, whatever it is called.
# A builder of defaults takes nothing and returns the root config, by value or
# by pointer. Loaders and helpers that merely mention the type take arguments,
# which is what keeps this anchor pointing at one function.
DEFAULTS_FUNC = r"^func [A-Z]\w*\(\s*\)\s*\*?" + CONFIG_ROOT + r"\s*\{"

# Where a pointer field's default lives when the struct itself carries no
# value: a named constant in the code that consumes the field. The label in
# each entry is prose; the value is always read from source, and a missing
# constant is an error rather than a stale number.
# The constant names it: a package that moves costs nothing, and a constant
# that is renamed or deleted is a real change that says so by name.
DEFAULT_CONSTS = {
    ("RelayerConfig", "dispatchPollInterval"): [("", "DefaultPollInterval")],
    ("RelayerChainOverride", "txSubmissionDelay"): [("", "DefaultTxSubmissionDelay")],
    ("RelayerChainOverride", "packetBatchSize"): [("", "DefaultBatchSize")],
    ("RelayerChainOverride", "packetBatchTimeout"): [
        ("receive and acknowledge", "DefaultBatchTimeout"),
        ("timeout", "DefaultTimeoutBatchTimeout")],
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
    ("ServerConfig", "listenAddr"): ("Address the gRPC server binds. It serves the relayer and attestor APIs together.", "a05468ed"),
    ("DBConfig", "type"): ("Database backend.", "70e2ad2c"),
    ("DBConfig", "url"): ("File path for sqlite, connection string for postgres. `:memory:` is rejected.", "d084b0d4"),
    ("ChainConfig", "chainId"): ("The chain's id, as the chain reports it.", "69a3e543"),
    ("EVMChainConfig", "rpc"): ("JSON-RPC endpoint for the chain.", "690d071f"),
    ("EVMChainConfig", "ics26Router"): ("Address of the ICS26 router on the chain.", "1daaecba"),
    ("AttestorConfig", "type"): ("Whether this process runs the attestor or queries it.", "a58f9a4e"),
    ("SignerConfig", "type"): ("Whether the key is a file on disk or a key held by a remote signer.", "febf1ab4"),
    ("RelayerConfig", "dispatchPollInterval"): ("How often the dispatcher polls the store for unfinished packets.", "893f79b1"),
    ("RelayerChainOverride", "chainId"): ("The chain these settings apply to.", "69a3e543"),
    ("RelayerChainOverride", "txSubmissionDelay"): ("Minimum delay between two transaction submissions on the chain.", "5691fa23"),
    ("RelayerChainOverride", "packetBatchSize"): ("How many packets the relayer puts in one transaction.", "b4f4f14c"),
    ("RelayerChainOverride", "packetBatchTimeout"): ("How long the relayer waits to fill a batch before submitting it.", "84d8816e"),
    ("RelayerEVMConfig", "gasFeeCapMultiplier"): ("Multiplies the fee cap the node suggests.", "b9de0a8d"),
    ("RelayerEVMConfig", "gasTipCapMultiplier"): ("Multiplies the tip cap the node suggests.", "634e0708"),
    ("ConnectionConfig", "alias"): ("Name for the connection, unique in the file.", "7e352d14"),
    ("AutoRelayConfig", "enabled"): ("Whether the relayer carries packets leaving this end without being asked.", "d693129e"),
    ("ClientEnd", "chainId"): ("The chain this end's client lives on.", "69a3e543"),
    ("ClientEnd", "signer"): ("`signers` alias that submits relay transactions on this chain.", "00fd3d36"),
    ("ClientEnd", "clientId"): ("The light client's id on this chain.", "bb596da7"),
    ("ClientEnd", "type"): ("Light client type.", "85b1564f"),
    ("Observability", "metrics"): ("Whether the process exports metrics. When false, the rest of this block is ignored.", "0754660b"),
    ("Observability", "type"): ("Which exporter serves the metrics.", "bc67445d"),
    ("Observability", "simpleMetricsListenAddr"): ("Address the `simple` exporter serves metrics on.", "78d41d56"),
    ("Observability", "otelFile"): ("OpenTelemetry configuration file, read when `type` is `otel`. `OTEL_CONFIG_FILE` overrides it, and one of the two is required.", "0e180386"),
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
    ("AutoRelayConfig", "enabled"),
    ("RelayerEVMConfig", "gasFeeCapMultiplier"),
    ("RelayerEVMConfig", "gasTipCapMultiplier"),
}

GO_TYPES = {"string": "string", "uint": "uint", "uint64": "uint64", "int": "int",
            "bool": "bool", "float64": "float64", "time.Duration": "duration"}


class SourceError(Exception):
    """A refusal to publish. `kind` names which one.

    The kind is carried rather than only phrased, because a test that matched
    the message as a substring accepted any refusal that happened to name the
    same identifier -- two cases named for `stale_fallback` were both passing
    on a different refusal entirely.
    """

    def __init__(self, message, kind=None):
        super().__init__(message)
        self.kind = kind


# When PLAN is a list, a problem is recorded and generation continues with a
# placeholder, so one run reports every gap rather than the first. When it is
# None, the same problem raises and nothing is written. Check mode and a normal
# regeneration always run with PLAN None: refusing to write stays the default.
# What a cell holds when the tool could not read it and plan mode is collecting
# rather than refusing. It is never written to a page: outside plan mode the
# problem raises, so nothing renders. It exists so one unreadable value does not
# cost the work order every other item on the page.
UNREADABLE = "?"

PLAN = None


def _problem(kind, message, **fields):
    """Raise, or record and continue in plan mode."""
    if PLAN is None:
        raise SourceError(message, kind)
    PLAN.append(dict(kind=kind, message=message, **fields))


def _read(path):
    return open(os.path.join(IBC, path)).read()

# ------------------------------------------------------- where things are

# Paths were five constants naming `cli/...`. Upstream renamed `link/` to
# `cli/` in #1423 and all five had to be hand-edited, for a change no reader of
# these pages could see. So each one is now found by what it contains: a
# directory move costs nothing, and a thing that genuinely disappears says so
# by name instead of surfacing as a confusing empty table.
_ANCHORS = {}

_SKIP_DIRS = {".git", "vendor", "node_modules", "testdata", "bin"}


def _walk(suffix):
    """Every file under the source repo with this suffix, repo-relative."""
    out = []
    for root, dirs, files in os.walk(IBC):
        dirs[:] = [d for d in dirs if d not in _SKIP_DIRS]
        for f in sorted(files):
            if f.endswith(suffix):
                out.append(os.path.relpath(os.path.join(root, f), IBC))
    return sorted(out)


def _locate(pattern, what, files=None, also=None, unique="match"):
    """The one file whose source matches, or a SourceError naming what is gone.

    The pattern is anchored at the start of a line, so it matches a
    declaration rather than a mention of one: a doc comment that quotes
    `func DefaultConfig()` should not make this ambiguous.

    Ambiguity is an error too. Two matches means the anchor no longer picks out
    one thing, and guessing between them is how a generator starts reading the
    wrong file without saying so.
    """
    rx = re.compile(pattern, re.M)
    hits = []
    for rel in (files if files is not None else _walk(".go")):
        try:
            text = open(os.path.join(IBC, rel), errors="ignore").read()
        except OSError:
            continue
        if also is not None and also not in text:
            continue
        # every match, not every file: two matches in one file is the same
        # ambiguity as two files, and reading the first of them silently is how
        # a generator starts describing the wrong thing.
        # a declaration should occur once, so count matches. A call site is
        # written once per use -- `cmd.PersistentFlags().StringVar(...)` six
        # times over is ordinary Go -- so for those, count files.
        found = rx.findall(text)
        if not found:
            continue
        hits += [rel] if unique == "file" else [rel] * len(found)
    if len(hits) == 1:
        return hits[0]
    raise SourceError(
        f"cannot locate {what}: {len(hits)} matches for {pattern!r}"
        + (f" (in {', '.join(sorted(set(hits)))})" if hits else "")
        + ". It moved, was renamed, was removed, or there are now two of it; "
        "this tool finds it by content rather than by path, so tell it the new "
        "anchor.")


def _anchors():
    """Locate every path this tool reads, once per source tree."""
    if _ANCHORS.get("__root__") == IBC:
        return _ANCHORS
    go = [f for f in _walk(".go") if not f.endswith("_test.go")]

    # The CLI is located first, because everything else is scoped to the module
    # it lives in. `func DefaultConfig() Config` is among the most ordinary
    # things to write in Go, so searching a whole repo for it stops picking out
    # one thing the day a second implementation lands beside this one.
    cobra_dirs = {os.path.dirname(f) for f in go
                  if "cobra.Command{" in open(os.path.join(IBC, f),
                                              errors="ignore").read()}
    entries = [f for f in go if os.path.dirname(f) in cobra_dirs]
    if CLI_PACKAGE:
        entries = [f for f in entries if os.path.dirname(f) == CLI_PACKAGE]
    main_go = _locate(r"^func main\(", "the CLI entry point", entries)

    cli_src = os.path.dirname(main_go)
    # the module the CLI lives in: the nearest go.mod at or above its source
    module = cli_src
    while module and not os.path.exists(os.path.join(IBC, module, "go.mod")):
        module = os.path.dirname(module)
    if not os.path.exists(os.path.join(IBC, module or ".", "go.mod")):
        raise SourceError(f"no go.mod at or above {cli_src}; cannot build the CLI")
    in_module = [f for f in go if f.startswith((module or ".") + os.sep)]

    # the config package: the one in this CLI's module with a function that
    # builds a fully populated root config
    config_go = _locate(DEFAULTS_FUNC,
                        f"the package that builds the {CONFIG_ROOT} this CLI reads",
                        in_module)
    # the root's own flags are declared in the config package rather than beside
    # the command, and are found by the cobra call that declares them, so the
    # function holding them is free to be called anything
    flags_go = _locate(r"PersistentFlags\(\)", "the persistent flag declarations",
                       [f for f in in_module
                        if os.path.dirname(f) == os.path.dirname(config_go)],
                       unique="file")

    _ANCHORS.clear()
    _ANCHORS.update({
        "__root__": IBC,
        "config_pkg": os.path.dirname(config_go),
        "global_flags_file": flags_go,
        "cli_src": cli_src,
        "cli_main": main_go,
        "cli_module": module or ".",
        "cli_pkg": os.path.relpath(cli_src, module or "."),
        "binary": os.path.basename(cli_src),
    })
    return _ANCHORS


def _is_config_field(go_name, yaml_key):
    """True when a struct field belongs in generated config docs."""
    if yaml_key == "-":
        return False
    return go_name[0].isupper()


def _config_files():
    """Every non-test Go file at or below the config package.

    Recursive, because a `Validate` moved into a subpackage is invisible to a
    listing of one directory -- and a struct whose rules go unread renders
    every one of its keys as `optional`.
    """
    pkg = _anchors()["config_pkg"]
    return [f for f in _walk(".go")
            if (f == pkg or f.startswith(pkg + os.sep))
            and not f.endswith("_test.go")]


def _params(sig):
    """A Go parameter list as [(name, type)], with grouped names expanded.

    `segment, format string` declares two strings, and reading it as one
    parameter is how a constructor stops being recognised -- taking every rule
    it carries off the page while the page still renders.
    """
    out, pending = [], []
    for part in sig.split(","):
        bits = part.split()
        if not bits:
            continue
        if len(bits) == 1:
            pending.append(bits[0])
            continue
        typ = " ".join(bits[1:])
        out += [(name, typ) for name in pending + [bits[0]]]
        pending = []
    # a list of bare types and no names: `func f(string, error) error`
    return out or [("", t) for t in pending]


def _path_error_ctors(src):
    """The package's path-prefixing error builders, found by signature.

    Validation errors are wrapped by helpers that take a path segment (or a
    list index) and return an error. Their names belong to whoever wrote them
    and are free to change; their shape is what this reads, so a rename is
    invisible here and a new helper of the same shape is picked up without
    being told about.

        func f(segment string, err error) error            -> seg_err
        func f(segment string, format string, ...) error    -> seg_fmt
        func f(idx int, err error) error                    -> idx_err
        func f(idx int, format string, ...) error           -> idx_fmt
    """
    out = {}
    for m in re.finditer(r"func (\w+)\(([^)]*)\) error \{", src):
        params = _params(m.group(2))
        if len(params) < 2:
            continue
        first, second = params[0][1], params[1][1]
        head = "seg" if first == "string" else "idx" if first == "int" else None
        tail = "err" if second == "error" else "fmt" if second == "string" else None
        if head and tail:
            out[m.group(1)] = f"{head}_{tail}"
    return out


def _blank(src, strings=True):
    """The source with comments -- and optionally string bodies -- blanked.

    Length is preserved rather than the text deleted, so every offset and line
    number still indexes the real source. Searching raw source for a
    declaration finds it inside a comment just as readily: a
    `/* DefaultBatchTimeout = 30 * time.Second */` note above the real one
    published 30s.
    """
    out, j, n = list(src), 0, len(src)
    while j < n:
        c = src[j]
        if c == "/" and j + 1 < n and src[j + 1] == "/":
            while j < n and src[j] != "\n":
                out[j] = " "
                j += 1
            continue
        if c == "/" and j + 1 < n and src[j + 1] == "*":
            close = src.find("*/", j + 2)
            close = n if close == -1 else close + 2
            for k in range(j, close):
                if src[k] != "\n":
                    out[k] = " "
            j = close
            continue
        if c in "\"`'":
            quote, j = c, j + 1
            while j < n:
                if src[j] == "\\" and quote != "`":
                    if strings:
                        out[j] = " "
                        if j + 1 < n:
                            out[j + 1] = " "
                    j += 2
                    continue
                if src[j] == quote:
                    j += 1
                    break
                if strings and src[j] != "\n":
                    out[j] = " "
                j += 1
            continue
        j += 1
    return "".join(out)


def _blank_comments(src):
    """Comments blanked, string literals intact."""
    return _blank(src, strings=False)


def _blank_noncode(src):
    """Comments and string bodies both blanked."""
    return _blank(src, strings=True)


def _go_block(src, i):
    """The braced block starting at or after `i`, and where it ends.

    Braces are counted outside strings and comments. Cutting at the first
    line that is just `}` is close enough almost always, and silently wrong
    when a body holds a raw string containing one -- which takes every rule
    below it off the page with nothing to notice.
    """
    start = src.index("{", i)
    depth, j, n = 0, src.index("{", i), len(src)
    while j < n:
        c = src[j]
        if c == "/" and j + 1 < n and src[j + 1] == "/":
            nl = src.find("\n", j)
            j = n if nl == -1 else nl
            continue
        if c == "/" and j + 1 < n and src[j + 1] == "*":
            close = src.find("*/", j + 2)
            j = n if close == -1 else close + 2
            continue
        if c in "\"`'":
            quote, j = c, j + 1
            while j < n:
                if src[j] == "\\" and quote != "`":
                    j += 2
                    continue
                if src[j] == quote:
                    j += 1
                    break
                j += 1
            continue
        if c == "{":
            depth += 1
        elif c == "}":
            depth -= 1
            if depth == 0:
                return src[start + 1:j], j
        j += 1
    raise SourceError(
        "a declaration's braces never balance; the source did not parse and "
        "reading on would attribute its contents to whatever follows")


def _method_bodies(src):
    """{(receiver type, method name): body} for every method in the package."""
    out = {}
    for m in re.finditer(r"func \((?:\w+ )?\*?(\w+)\) (\w+)\(", src):
        try:
            body, _end = _go_block(src, m.end())
        except (SourceError, ValueError):
            continue
        out[(m.group(1), m.group(2))] = body
    return out


def _validate_source(recv, bodies, ctors):
    """Validate's body, plus the same-receiver helpers whose rules it owns.

    A helper called for its error alone contributes that error's path, so its
    body is read as part of Validate:

        if err := c.validateConnections(); err != nil { ... }

    A helper whose result is wrapped with a path segment at the call site does
    not: the key is at the call site and the condition often is too, so the
    rule inside cannot be restated as `this key is required`.

        case validateICS26Router:
            return errPath("ics26Router", c.validateICS26Router())

    Reading the second kind is how `ics26Router` would come to be called
    required when it is required only when a caller asks for it.
    """
    seg_err = {n for n, k in ctors.items() if k in ("seg_err", "idx_err")}
    seen, out, stack = set(), [], [(recv, "Validate")]
    while stack:
        key = stack.pop()
        if key in seen or key not in bodies:
            continue
        seen.add(key)
        body = bodies[key]
        out.append(body)
        wrapped = set()
        for ctor in seg_err:
            for w in re.finditer(re.escape(ctor) + r"\([^,]+,\s*\w+\.(\w+)\(", body):
                wrapped.add(w.group(1))
        for call in re.findall(r"\b\w+\.(\w+)\(", body):
            if call not in wrapped and (recv, call) in bodies and call != "Validate":
                stack.append((recv, call))
    return "\n".join(out)


def _rules_in(body, ctors):
    """(message, format args) for every field rule a body can return.

    Three shapes, none of which names a function:

      f(".key msg", args...)   a path and a message in one string
      f("key", "msg", args...) a path segment and a message, by a seg_fmt ctor
      f(idx, "msg", args...)   an indexed element, by an idx_fmt ctor
    """
    msgs = []
    for e in re.finditer(r'\b[\w.]+\(\s*"(\.[^"]+)"((?:,\s*\w+)*)', body):
        msgs.append((e.group(1), [a.strip() for a in e.group(2).split(",") if a.strip()]))
    for name, kind in ctors.items():
        if kind == "seg_fmt":
            for e in re.finditer(re.escape(name) + r'\(\s*"([^"]+)",\s*"([^"]+)"((?:,\s*[\w.]+)*)', body):
                msgs.append((f".{e.group(1)} {e.group(2)}",
                             [a.strip() for a in e.group(3).split(",") if a.strip()]))
        elif kind == "idx_fmt":
            for e in re.finditer(re.escape(name) + r'\([^,"]+,\s*"([^"]+)"((?:,\s*[\w.]+)*)', body):
                msgs.append((f".[] {e.group(1)}",
                             [a.strip() for a in e.group(2).split(",") if a.strip()]))
    return msgs


# One field declaration, with Go's grouped names allowed.
FIELD_DECL = re.compile(r"(\w+(?:\s*,\s*\w+)*)\s+([^\s`]+)(?:\s+`([^`]*)`)?$")


def _unparsed_field(struct, path, lines):
    """Refuse on a struct member this parser did not understand.

    The proto side refuses on constructs it cannot read. This is its twin, and
    it was missing: an embedded struct with an inline yaml tag, a field
    whose type this pattern does not match, or an anonymous nested struct all
    matched nothing and were skipped. A skipped field renders a shorter table,
    a table with no rows renders as `_None._`, and regenerating makes the check
    green again -- so a whole block can leave the page and look like currency.
    """
    if not lines:
        return
    _problem(
        "unreadable_member",
        f"{struct} in {path} declares members this parser does not read: "
        + "; ".join(sorted(lines)[:4])
        + f"{' ...' if len(lines) > 4 else ''}. Skipping them would shorten the "
        "table, and a table with nothing left renders as `_None._` on a page "
        "that still passes its check. Teach parse_go_config() the construct, or "
        "give the field a `yaml:\"-\"` tag if a reader never writes it.")


def parse_go_config():
    """Structs, string constants, literal defaults, and validation rules from
    the config package."""
    structs, aliases, consts, const_type, defaults, validations = {}, {}, {}, {}, {}, {}
    duplicates = []
    files = _config_files()
    src = "\n".join(_read(f) for f in files)
    # a copy with comments and string bodies blanked, so a declaration written
    # out inside a comment is not read as one. String *values* are still needed
    # for the constant table, so that scan gets a copy with comments blanked
    # and strings intact.
    blanked = _blank_noncode(src)
    blanked_strings = _blank_comments(src)

    for m in re.finditer(r"type\s+(\w+)\s+\[\](\w+)", src):
        aliases[m.group(1)] = m.group(2)

    # The named string types the package declares. Only these make a key an
    # enum on the page: a constant declared `const X string = "..."` says its
    # type is the builtin, and reading that as an enum rendered every `string`
    # key in every table as a list of unrelated constants.
    named_string_types = {t.group(1) for t in
                          re.finditer(r"^type\s+(\w+)\s+string\b", blanked, re.M)}
    # and the same declarations written in a grouped `type ( ... )` block,
    # which is how this package spells them
    in_group = False
    for ln in blanked.split("\n"):
        if re.match(r"^type\s*\($", ln.strip()):
            in_group = True
            continue
        if in_group:
            if ln.startswith(")"):
                in_group = False
                continue
            d = re.match(r"^\s+(\w+)\s+string\s*$", ln)
            if d:
                named_string_types.add(d.group(1))

    for m in re.finditer(r'(\w+)(?:\s+(\w+))?\s*=\s*"([^"]*)"', blanked_strings):
        consts[m.group(1)] = m.group(3)
        if m.group(2) and m.group(2) in named_string_types:
            const_type[m.group(1)] = m.group(2)

    for path in files:
        text = _read(path)
        # a copy with trailing comments blanked, so `URL string `+"`"+`yaml:"url"`+"`"+` // note`
        # parses as the field it is. Doc comments are read from the raw line,
        # which still has them.
        masked = _blank_comments(text)
        for m in re.finditer(r"^type\s+(\w+)\s+struct\s*\{", text, re.M):
            name = m.group(1)
            # brace-aware, because `type X struct{}` closes on its own line and
            # scanning forward for one swallowed every declaration after it
            body, _end = _go_block(text, m.end() - 1)
            body_masked, _e2 = _go_block(masked, m.end() - 1)
            masked_lines = body_masked.split("\n")
            decl_line = text[:m.start()].count("\n") + 1
            first = text[:m.end()].count("\n") + 1
            fields, doc, leftover = [], [], []
            for k, raw in enumerate(body.split("\n")):
                ln = raw.strip()
                if ln.startswith("//"):
                    doc.append(ln[2:].strip())
                    continue
                if not ln:
                    doc = []
                    continue
                # the declaration without whatever comment trails it
                code = (masked_lines[k] if k < len(masked_lines) else ln).strip()
                if not code:
                    doc = []
                    continue
                f = FIELD_DECL.match(code)
                if not f:
                    leftover.append(ln)
                    doc = []
                    continue
                raw_tag = re.search(r'yaml:"([^"]*)"', f.group(3) or "")
                if raw_tag is None:
                    # the tag's contents were blanked with the comment; read it
                    # back from the real line
                    raw_tag = re.search(r'yaml:"([^"]*)"', ln)
                tag = re.search(r'yaml:"([^",]+)', f.group(3) or "")
                names = [n.strip() for n in f.group(1).split(",")]
                # an embedded struct folded into the parent's keys. Its fields
                # belong in this table and this parser does not reach them, so
                # the row it would render is a lie by omission.
                inline = raw_tag and "inline" in raw_tag.group(1).split(",")
                if inline or (tag and len(names) > 1):
                    leftover.append(ln)
                    doc = []
                    continue
                for go_name in names:
                    # No tag is not an error: goccy lowercases the field name
                    # and the key works. Falling back to the Go name published
                    # a key the CLI rejects outright -- it runs with
                    # DisallowUnknownField, so `MaxRecvBytes` answers
                    # `unknown field`, while the sample config this same tool
                    # documents writes `maxrecvbytes`. Verified against the
                    # binary: ToLower over the whole name, so `TLSCertFile`
                    # becomes `tlscertfile` and not `tlsCertFile`.
                    yaml_key = tag.group(1) if tag else go_name.lower()
                    if _is_config_field(go_name, yaml_key):
                        fields.append({"go": go_name, "type": f.group(2),
                                       "yaml": yaml_key, "doc": " ".join(doc),
                                       "line": first + k})
                doc = []
            _unparsed_field(name, path, leftover)
            if name in structs and structs[name]["file"] != path:
                # only when it would actually reach a page. A repeated type
                # name in a corner of the package that nothing documents is
                # not this tool's business, and refusing over it is the kind
                # of stop that has nothing to do with a reader.
                duplicates.append((name, structs[name]["file"], path, decl_line))
            structs[name] = {"fields": fields, "line": decl_line, "file": path}

    # literal defaults from the function that builds a populated root config
    at = re.findall(DEFAULTS_FUNC, src, re.M)
    if len(at) != 1:
        raise SourceError(
            f"{len(at)} functions return a populated {CONFIG_ROOT}; the defaults "
            "column is read from exactly one of them, and picking the first "
            "silently replaces every default on the page with whatever that "
            "one sets. Name the builder, or fold the others into it.")
    # comments blanked, string bodies kept: the row patterns below are anchored
    # at end-of-line, so a trailing `// note` on a default stopped the row from
    # matching at all and the default silently left the page. Every other Go
    # read in this file already goes through a blanked copy; this one did not.
    # Blanking preserves length, so the offsets still index the real source.
    body, _end = _go_block(blanked_strings,
                           re.search(DEFAULTS_FUNC, blanked_strings, re.M).end())
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

    # validation rules: every error string a struct's Validate can return.
    # The shapes are discovered, not listed: the constructors by their
    # signatures and the helpers by how their result is used. A rename in the
    # config package is invisible here, which is the point -- teaching this
    # file a new name was work that changed no word a reader sees.
    # Two depths, because they answer different questions.
    #
    # `validations` is what Validate itself returns, and it is what the
    # required column reads. A rule a helper returns is usually conditional on
    # the branch that called the helper, so restating it as `this key is
    # required` would be a lie: `otelFile` is required when `type` is `otel`,
    # and `ics26Router` when a caller asks for it.
    #
    # `deep` adds those helper rules, and feeds the fingerprints and the
    # ruleless-struct check. A hand-written description whose helper rule
    # changed then goes stale and says so, which is the guard the two name
    # lists used to provide -- without a list, and without stopping anyone to
    # be taught a name.
    ctors = _path_error_ctors(src)
    bodies = _method_bodies(src)
    deep = {}
    for recv, _name in [k for k in bodies if k[1] == "Validate"]:
        validations[recv] = _rules_in(bodies[(recv, "Validate")], ctors)
        deep[recv] = _rules_in(_validate_source(recv, bodies, ctors), ctors)

    return {"structs": structs, "aliases": aliases, "consts": consts,
            "duplicates": duplicates,
            "const_type": const_type, "defaults": defaults,
            "validations": validations, "deep_validations": deep}


# Structs whose Validate() legitimately yields no field rule: they check
# cross-references and delegate to nested Validate() calls rather than rejecting
# a field's own value. Asserted in test-refgen.py, so a struct cannot go silent.
#
# This list is the one guard that replaced two others. There used to be a list
# of the error constructors a Validate body was allowed to call and a list of
# the helpers it was allowed to delegate to, and a new name in either one was a
# hard refusal -- work for a developer that changed no word a reader sees. Both
# are now derived: constructors by their signatures, helpers by how their result
# is used. What remains is a claim about coverage rather than about vocabulary:
# a struct that validates something must yield a rule, or say here that it does
# not.
RULELESS_VALIDATORS = {
    "Attestors", "Config", "ServerConfig",   # check cross-references, delegate the rest
    "AttestationParams",                     # `return nil`, satisfies an interface
}


def _const_value(name):
    """A named Go constant's value, and where it is declared.

    The constant is found by name across the repo rather than read out of a
    path written down here: the package it lives in is free to move, and a
    constant that is renamed or deleted is a change worth stopping for.
    """
    # The declaration, not a mention of it. A constant named in a comment --
    # `// DefaultPollInterval = 99 * time.Second (was)` -- matched a bare
    # search first and published that number.
    # The declaration, with an optional explicit type, and not a mention of it
    # in prose. Matching is done against a copy with comments and string bodies
    # blanked; the value itself is then sliced out of the real source, because
    # a blanked copy turns `"3s"` into `"  "`.
    decl = re.compile(r"^[ \t]*(?:const\s+)?" + re.escape(name)
                      + r"(?:\s+[\w.*\[\]]+)?\s*=\s*(.+)$", re.M)
    # only the module the CLI is built from: an unrelated package elsewhere in
    # the repo declaring the same name is not this page's business
    module = _anchors()["cli_module"]
    hits = []
    for rel in _walk(".go"):
        if rel.endswith("_test.go") or not rel.startswith(module + os.sep):
            continue
        src = open(os.path.join(IBC, rel), errors="ignore").read()
        # every match, not the first: a constant of the same name declared
        # inside a function is legal Go, and taking the first one published
        # its value as the documented default
        for m in decl.finditer(_blank_noncode(src)):
            hits.append((rel, src, m))
    if len(hits) != 1:
        _problem(
            "unreadable_default",
            f"constant {name} has {len(hits)} declarations"
            + (f" (in {', '.join(sorted({h[0] for h in hits}))})" if hits else "")
            + "; a default on the page is read from it, so it cannot be "
            "guessed at. It was renamed, removed, or duplicated.",
            constant=name, found_in=[h[0] for h in hits])
        return UNREADABLE, None, None
    path, src, m = hits[0]
    # blanking preserves length, so the same span indexes the real text. What
    # the comment occupied is spaces in the blanked copy, so rstrip cuts
    # exactly the comment and nothing of the value.
    keep = len(m.group(1).rstrip())
    raw = src[m.start(1):m.start(1) + keep].strip().rstrip(",")
    line = src[:m.start()].count("\n") + 1
    if re.search(r"[*+\-/(,]$", raw):
        _problem(
            "unreadable_default",
            f"the value of {name} continues onto the next line ({raw!r}); this "
            "reads one line and would publish that fragment as the default. "
            "Put the expression on one line, or give the key its default in "
            "the config builder instead.",
            constant=name, file=path, line=line)
        return UNREADABLE, path, line
    units = {"Nanosecond": "ns", "Microsecond": "us", "Millisecond": "ms",
             "Second": "s", "Minute": "m", "Hour": "h"}
    unit = "|".join(units)
    d = re.match(r"(\d+)\s*\*\s*time\.(%s)$" % unit, raw)
    if d:
        return f"{d.group(1)}{units[d.group(2)]}", path, line
    d = re.match(r"time\.(%s)$" % unit, raw)
    if d:
        return f"1{units[d.group(1)]}", path, line
    if re.fullmatch(r'-?\d+(\.\d+)?|".*"|true|false', raw):
        return raw, path, line
    _problem(
        "unreadable_default",
        f"the value of {name} is {raw!r}, which this does not know how to write "
        "for a reader. Printing it as-is would put Go source in the Default "
        "column. Teach _const_value() the form, or give the key its default in "
        "the config builder instead.",
        constant=name, value=raw, file=path, line=line)
    return UNREADABLE, path, line


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
            _problem("scalar_top_level",
                     f"top-level key {field['yaml']} is not a block; the page "
                     "has no shape for a scalar there. Give it a section of its "
                     "own, or nest it under one.",
                     key=field["yaml"])
            continue
        walk(f"config:{field['yaml']}", child, (CONFIG_ROOT, field["yaml"]))
    return out


def _rules_for(struct, field, model):
    """Every validation rule that names this field.

    What the fingerprint is computed over, and what a `fingerprint_mismatch`
    has to report: a rule that only changed wording still moves the hash, and
    still changes what the Default-or-required column says.
    """
    return sorted(set(
        msg for msg, _a in
        model.get("deep_validations", model["validations"]).get(struct, [])
        if msg.lstrip(".").split()[0].split("[")[0] == field["yaml"]))


def _fingerprint(struct, field, model):
    """Hash the source a hand-written description depends on.

    A description is only as good as the code it describes, and a key whose
    meaning changes under a stable name moves no table cell, so nothing else
    here would notice. This covers the field's type, its yaml key, and every
    validation rule naming it: enough to catch a real change, and blind to
    whitespace and to code elsewhere in the struct.
    """
    basis = "|".join([field["type"], field["yaml"],
                      *_rules_for(struct, field, model)])
    return hashlib.sha1(basis.encode()).hexdigest()[:8]


# A clause a Go comment opens with to say which variant a key belongs to.
# Stripping one is only safe when the table it lands in is already about that
# variant -- otherwise the clause is the sole statement of the condition, and
# removing it leaves a row that reads as unconditional.
_VARIANT_CLAUSE = [
    re.compile(r"^(required|optional)[^.]*?(?:--|—)\s*"),
    re.compile(r"^required for [^.]*\.\s*"),
    re.compile(r"^(local|remote) only\.\s*"),
]


def _clean_doc(field, variant=None):
    """A Go field comment, read as a sentence about the key.

    Go comments open with the field's own name, so that comes off -- an exact
    match on the identifier, not a guess.

    A clause naming a variant comes off only when `variant` says this table is
    already about that variant. It used to come off always, on the reasoning
    that the columns restate it. They do not always: a key whose condition
    lives in a helper renders `optional`, and the stripped clause was the only
    place a reader could have learned otherwise.
    """
    doc = field["doc"]
    if not doc:
        return ""
    if doc.startswith(field["go"]):
        doc = doc[len(field["go"]):].strip()
    doc = re.sub(r"^is\s+", "", doc)
    for rx in _VARIANT_CLAUSE:
        m = rx.match(doc)
        if m and variant and variant.lower() in m.group(0).lower():
            doc = doc[m.end():]
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


# ---------------------------------------- requiredness, by asking the binary

# Whether a config key is required was read out of the English in the Go
# validation messages. That is the last thing on these pages decided by
# matching words, and the words are free to change: rewording `required` to
# `mandatory` flipped a key to optional with nothing to notice.
#
# The binary already answers the question exactly. `PathError.Path()` comes
# back as the key path, so removing one key from a working config and asking
# `ibc config validate` says whether that key is required -- and the match is
# against the path removed, not against any phrase.
#
# Two fixtures because `observability.type` cannot be `simple` and `otel` at
# once. Both are validated by the probe before anything is read from them: a
# fixture that stops loading answers nothing, and says so.
# beside this file, not under the tree being documented: the fixture belongs
# to the tool. A sandbox that renames a config key makes it stale, and a stale
# fixture refuses rather than answering wrongly, which is the intended
# behaviour rather than an accident of where the file sits.
PROBE_FIXTURES = [os.path.join(os.path.dirname(os.path.abspath(__file__)), n)
                  for n in ("probe-config.yml", "probe-config-otel.yml")]

# files a fixture refers to that must exist for it to validate at all
PROBE_SIDECARS = {"probe-key.json": "{}", "otel.yaml": "{}"}


def _yaml_join(stack):
    out = ""
    for _indent, seg in stack:
        out += seg if seg.startswith("[") else (("." + seg) if out else seg)
    return out


def _yaml_paths(text):
    """(line, path, indent, opens_a_list_item, inline value) for every key.

    Indentation and `- ` are enough to know where you are, which keeps this
    file free of a yaml dependency it otherwise does not need.
    """
    out, stack, counts = [], [], {}
    for i, raw in enumerate(text.split("\n")):
        if not raw.strip() or raw.lstrip().startswith("#"):
            continue
        indent = len(raw) - len(raw.lstrip())
        body, item = raw.lstrip(), False
        if body.startswith("- "):
            body, item, indent = body[2:], True, indent + 2
        m = re.match(r"([A-Za-z_]\w*):(.*)$", body)
        if not m:
            continue
        # a new list item also retires the previous item's index marker, which
        # sits one level shallower than the item's own keys
        floor = indent - 1 if item else indent
        while stack and stack[-1][0] >= floor:
            if not stack[-1][1].startswith("["):
                counts.pop(_yaml_join(stack), None)
            stack.pop()
        if item:
            parent = _yaml_join(stack)
            n = counts.get(parent, -1) + 1
            counts[parent] = n
            stack.append((indent - 1, f"[{n}]"))
        stack.append((indent, m.group(1)))
        out.append((i, _yaml_join(stack), indent, item, m.group(2).strip().strip('"')))
    return out


def _yaml_without(text, path):
    """`text` with `path` removed, or None if it is not there.

    Only the key's own children go with it -- a sibling sits at the same
    indent, and taking siblings too removed a whole block and produced an
    error about something else entirely.
    """
    lines = text.split("\n")
    hit = [(i, ind, item) for i, p, ind, item, _v in _yaml_paths(text) if p == path]
    if not hit:
        return None
    i, indent, item = hit[0]
    j = i + 1
    while j < len(lines) and (not lines[j].strip() or
                              len(lines[j]) - len(lines[j].lstrip()) > indent):
        j += 1
    kept = lines[:i] + lines[j:]
    if item:
        # this key carried the item's `- `; the next sibling inherits it
        if i < len(kept) and len(kept[i]) - len(kept[i].lstrip()) == indent:
            kept[i] = " " * (indent - 2) + "- " + kept[i].lstrip()
        else:
            return None
    return "\n".join(kept)


def _probe_validate(binary, text):
    """The key path the binary objects to, or None when it is content."""
    home = tempfile.mkdtemp(prefix="refgen-cfg-")
    try:
        with open(os.path.join(home, "ibc.yml"), "w") as fh:
            fh.write(text)
        for name, body in PROBE_SIDECARS.items():
            with open(os.path.join(home, name), "w") as fh:
                fh.write(body)
        r = subprocess.run([binary, "config", "validate", "--home", home],
                           capture_output=True, text=True, timeout=PROBE_TIMEOUT)
        if r.returncode == 0:
            return None
        m = re.search(r"unable to load the config: ([^:\s]+):", r.stdout + r.stderr)
        return m.group(1) if m else _WALL
    except subprocess.TimeoutExpired:
        return _WALL
    finally:
        shutil.rmtree(home, ignore_errors=True)


def _config_locations(model):
    """{(child struct, parent struct, parent key): path from the root}.

    Walks the struct graph the model already holds, so a key's place in the
    file is derived rather than spelled out anywhere. `[]` marks a collection,
    filled in later with the index of the element that matches the variant
    being documented.
    """
    structs, aliases = model["structs"], model["aliases"]

    def child_of(f):
        t = f["type"].lstrip("*")
        if t.startswith("[]"):
            t = t[2:]
            return (t, True) if t in structs else (None, False)
        if t in aliases:
            return aliases[t], True
        return (t, False) if t in structs else (None, False)

    loc = {}

    def walk(struct, prefix, seen):
        if struct in seen:
            return
        for f in structs[struct]["fields"]:
            child, collection = child_of(f)
            if not child:
                continue
            path = f"{prefix}.{f['yaml']}" if prefix else f["yaml"]
            if collection:
                path += "[]"
            loc[(child, struct, f["yaml"])] = path
            walk(child, path, seen + (struct,))

    walk(CONFIG_ROOT, "", ())
    return loc


def probe_requiredness(model, binary):
    """{(region, key): True/False/None} -- required, not, or unanswerable.

    For each documented key: remove exactly that key from a working config and
    ask the binary. It is required when the binary objects to the path that was
    removed. Nothing here reads a word of the message.

    None means no fixture holds the key, so nothing is claimed. Rendering it
    `optional` would be a guess, and a guess reads exactly like knowledge.
    """
    loc = _config_locations(model)
    fixtures = []
    for full in PROBE_FIXTURES:
        rel = os.path.basename(full)
        if not os.path.exists(full):
            raise SourceError(
                f"the requiredness probe needs {rel}, and it is not there. It "
                "is a config the binary validates, and the column is read by "
                "removing one key from it at a time.", kind="missing_probe_fixture")
        text = open(full).read()
        if _probe_validate(binary, text) is not None:
            raise SourceError(
                f"{rel} no longer loads, so nothing can be learned by removing "
                "keys from it. A key it names was probably renamed; fix the "
                "fixture to match the config package.",
                kind="stale_probe_fixture")
        fixtures.append((rel, text, _yaml_paths(text)))

    out = {}
    for sec in discover_config_sections(model):
        disc = sec["discriminator"]
        for struct, field, key, parent in sec["rows"]:
            base = loc.get((struct, parent[0], parent[1]))
            if base is None:
                out[(sec["region"], key)] = (None, None)
                continue
            answer, condition = None, None
            for _rel, text, paths in fixtures:
                path, reachable = f"{base}.{field['yaml']}", True
                while "[]" in path:
                    head = path.split("[]", 1)[0]
                    index = 0
                    if disc is not None:
                        index = _variant_index(paths, head, disc[0]["yaml"], disc[1])
                        if index is None:
                            reachable = False
                            break
                    path = path.replace("[]", f"[{index}]", 1)
                if not reachable:
                    continue
                without = _yaml_without(text, path)
                if without is None:
                    continue
                answer = _probe_validate(binary, without) == path
                # A key only one fixture holds is conditional: the others are
                # valid configs without it. The condition is whatever
                # distinguishes that fixture at the key's own level -- read
                # from the fixture, not named here.
                elsewhere = [p2 for rel2, t2, p2 in fixtures if t2 is not text]
                if answer and elsewhere and all(
                        not any(q == path for _i, q, _n, _t, _v in p2)
                        for p2 in elsewhere):
                    condition = _sibling_discriminator(paths, path)
                break
            out[(sec["region"], key)] = (answer, condition)
    return out


def _sibling_discriminator(paths, path):
    """The value of a `type` sitting beside `path`, if there is one.

    What makes a fixture the otel one rather than the simple one is that its
    `observability.type` says so. Reading it back out is how a key only that
    fixture holds gets labelled with the condition it depends on.
    """
    parent = path.rsplit(".", 1)[0]
    for _i, p, _ind, _item, inline in paths:
        if p == f"{parent}.type" and inline:
            return inline
    return None


def _variant_index(paths, collection, field, value):
    """Which element of `collection` is the variant being documented."""
    rx = re.compile(re.escape(collection) + r"\[(\d+)\]\." + re.escape(field) + r"$")
    for _i, path, _ind, _item, inline in paths:
        m = rx.match(path)
        if m and inline == value:
            return int(m.group(1))
    return None


def _requirement(go, field, model, parent=None, probed=None, variant=None):
    """The Default-or-required column.

    A key with a default is never the reader's to supply, so a default wins
    over everything below.

    Required-ness itself comes from `probed`: the binary was asked, by removing
    the key from a working config and seeing whether it objected to that path.
    Reading it out of the English in the validation messages is what this
    replaced -- a reworded message flipped a key to optional and nothing
    noticed. The old reading stays only as the answer for tables whose
    membership is still decided that way; where the probe has spoken, it wins.
    """
    key = field["yaml"]
    if (go, field["go"]) in model["defaults"]:
        return f"`{model['defaults'][(go, field['go'])]}`", None
    if (go, field["yaml"]) in DEFAULT_CONSTS:
        parts, cites = [], []
        for label, const in DEFAULT_CONSTS[(go, field["yaml"])]:
            value, path, line = _const_value(const)
            parts.append(f"`{value}` ({label})" if label else f"`{value}`")
            if path:
                # a constant the tool could not locate has nowhere to point
                cites.append((path, line))
        return ", ".join(parts), cites

    if probed is not None:
        # the binary's answer, in the variant this table is about
        return (("**required**" if not variant else f"**required** for `{variant}`")
                if probed else "optional"), None

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
    _problem("unreadable_discriminator",
             f"{go}: cannot tell what the opposite of {kind!r} is, so the "
             "column cannot say which kind the key belongs to",
             struct=go, kind=kind)
    return UNREADABLE


def _description(struct, field, model, seen, variant=None):
    """The Description cell, and the four checks that keep it honest.

    A key documented in the source uses that; a key the source leaves
    undocumented uses FALLBACK_DOCS, whose entry carries a fingerprint of the
    code it describes. Both, neither, or a fingerprint that no longer matches
    all raise, because each of those is a description nobody has re-read.
    """
    doc = _clean_doc(field, variant)
    fallback = FALLBACK_DOCS.get((struct, field["yaml"]))
    where = f"{struct}.{field['go']}"
    # Where a person -- or an agent -- goes to fix it. The better fix for a key
    # nobody has described is a doc comment on the declaration itself, so the
    # work order names the declaration rather than only the key.
    declared_in = model["structs"].get(struct, {}).get("file")
    declared_at = field.get("line")
    if doc and fallback:
        _problem("stale_fallback",
                 f"{where} now has a doc comment; drop its FALLBACK_DOCS entry",
                 field=where, yaml_key=field["yaml"],
                 file=declared_in, line=declared_at)
        return doc
    if not doc and not fallback:
        _problem("missing_description",
                 f"{where} has no doc comment and no FALLBACK_DOCS entry. The "
                 "better fix is a doc comment on the declaration; a "
                 "FALLBACK_DOCS entry is for a source you cannot edit.",
                 field=where, yaml_key=field["yaml"],
                 file=declared_in, line=declared_at,
                 fingerprint=_fingerprint(struct, field, model))
        return "TODO: describe this key"
    if not fallback:
        return doc
    text, recorded = fallback
    seen.add((struct, field["yaml"]))
    current = _fingerprint(struct, field, model)
    if current != recorded:
        _problem("fingerprint_mismatch",
                 f"{where}: the source behind its hand-written description changed "
                 f"(fingerprint {recorded} -> {current}). Re-read \"{text}\" against the "
                 "code, then record the new fingerprint. Nothing is written until you "
                 f"do. What the fingerprint covers: type {field['type']}, key "
                 f"{field['yaml']}, rules {_rules_for(struct, field, model) or 'none'}. "
                 "A validation message that only changed wording moves this "
                 "fingerprint too, and changes what the Default-or-required column "
                 "says even though the declaration you are sent to did not move.",
                 field=where, description=text, was=recorded, now=current,
                 rules=_rules_for(struct, field, model),
                 file=declared_in, line=declared_at)
    return text


# The words `_requirement` reads out of a validation message to decide what the
# Default-or-required column says. They are the config package's words, not
# this tool's, so they can be reworded -- and a rewording moves no table cell,
# it only empties one.
REQUIREMENT_VOCABULARY = ("required", "must not be set for", "must be one of")


def _requirement_canary(model):
    """Refuse if no validation message uses the words the column is read from.

    The same shape as the required-flag canary on the CLI side. It cannot see a
    single key reworded -- only the whole vocabulary going away at once, which
    is what a refactor of the error messages looks like. That is the failure
    that renders a full page of `optional` and still passes every check.
    """
    seen = [msg for rules in model["validations"].values() for msg, _a in rules
            if any(w in msg for w in REQUIREMENT_VOCABULARY)]
    if not seen and not os.environ.get("REFGEN_NO_REQUIRED_KEYS"):
        raise SourceError(kind="all_keys_optional", message=(
            "no validation message uses any of "
            f"{', '.join(repr(w) for w in REQUIREMENT_VOCABULARY)}, so every key "
            "on the page would be rendered `optional`. Either nothing is "
            "required any more, or the config package reworded its errors and "
            "this tool is now reading none of them. Confirm which, then update "
            "REQUIREMENT_VOCABULARY or set REFGEN_NO_REQUIRED_KEYS=1."))


def _example_config():
    """The config fixture the config package's own tests load and validate.

    Copied, not written here and not synthesised. It is a file the Go suite
    already parses with validation on, so a key renamed without updating it
    fails `go test` in the same pull request that renamed it -- a stronger
    guarantee than anything this tool could check for itself, and one that
    costs nothing to follow. The example on this page used to be a hand-copy
    of the tutorial's config: nothing checked it, and a rename left it showing
    a key that no longer existed.

    The fixture is found by what the tests load rather than by its path, so
    renaming it or moving testdata costs nothing. Two fixtures is a stop:
    which config a reference page should show is not this tool's decision.
    """
    pkg = _anchors()["config_pkg"]
    tests = [f for f in _walk(".go")
             if f.endswith("_test.go") and os.path.dirname(f) == pkg]
    names, validated = set(), False
    for f in tests:
        src = _blank_comments(_read(f))
        for m in re.finditer(
                r'filepath\.Join\(\s*"([^"]+)"\s*,\s*"([^"]+\.ya?ml)"\s*\)', src):
            names.add((m.group(1), m.group(2)))
        if re.search(r"LoadFromFile\([^)]*,\s*true\s*\)", src):
            validated = True
    if not names:
        _problem("no_example_config",
                 f"no yaml fixture is loaded by the tests in {pkg}, so there is "
                 "no example config that anything keeps valid. The example is "
                 "copied from the fixture the Go tests already validate.",
                 package=pkg)
        return ""
    if len(names) > 1:
        _problem("ambiguous_example_config",
                 f"{pkg} tests load more than one yaml fixture "
                 f"({', '.join('/'.join(n) for n in sorted(names))}); which one a "
                 "reference page should show is a choice this tool cannot make.",
                 package=pkg, fixtures=sorted("/".join(n) for n in names))
        return ""
    sub, name = names.pop()
    rel = os.path.join(pkg, sub, name)
    if not os.path.exists(os.path.join(IBC, rel)):
        _problem("unreadable_example_config",
                 f"the tests load {rel}, but it is not there to read", file=rel)
        return ""
    if not validated:
        _problem("unvalidated_example_config",
                 f"no test in {pkg} loads a config with validation on, so nothing "
                 f"proves {rel} is still valid. The example is published on the "
                 "strength of that test.",
                 file=rel)
        return ""
    text = open(os.path.join(IBC, rel)).read()
    # the licence header is a fact about the repository, not about the config
    text = re.sub(r"\A(\s*#[^\n]*\n)+", "", text).strip("\n")
    return "```yaml\n" + text + "\n```\n\n" + cite(rel, 1)


def gen_config():
    model = parse_go_config()
    _requirement_canary(model)
    probed = probe_requiredness(model, build_cli())
    blocks, seen_fallbacks = {}, set()
    for sec in discover_config_sections(model):
        rows, cites = [], []
        for struct, field, key, parent in sec["rows"]:
            description = _description(
                struct, field, model, seen_fallbacks,
                variant=sec["discriminator"][1] if sec["discriminator"] else None)
            if sec["discriminator"] and field is sec["discriminator"][0]:
                # the key that names this table: its value is the heading
                rows.append((f"`{key}`", f"`{sec['discriminator'][1]}`",
                             "**required**", description))
                continue
            answer, condition = probed.get((sec["region"], key), (None, None))
            if answer is None:
                _problem("unprobed_key",
                         f"no fixture the probe validates contains `{key}`, so "
                         "whether it is required could not be asked of the "
                         "binary. Add it to a probe fixture in "
                         "docs/6-ibc-cli/tools/.",
                         key=key, region=sec["region"])
            req, extra = _requirement(struct, field, model, parent,
                                      probed=answer, variant=condition)
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

    blocks["config:example"] = _example_config()
    blocks["notice"] = _notice()

    # A repeated type name only matters if both would reach a page: the tables
    # are keyed by type name, so one silently replaces the other and the page
    # describes whichever file sorted last.
    documented = {st for sec in discover_config_sections(model)
                  for st, _f, _k, _p in sec["rows"]}
    for name, first, second, line in model.get("duplicates", []):
        if name in documented:
            _problem("duplicate_struct",
                     f"{name} is declared in both {first} and {second}, and both "
                     "would describe the same keys. One silently replaces the "
                     "other. Rename one, or move it out of the config package.",
                     struct=name, files=[first, second], file=second, line=line)

    reachable = {(st, f["yaml"]) for sec in discover_config_sections(model)
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
        and (st, f["yaml"]) not in DEFAULT_CONSTS
        and (st, f["yaml"]) not in NO_NAMED_DEFAULT
        and (st, f["go"]) not in SKIP_FIELDS)
    if unclaimed:
        _problem("unclaimed_default",
                 "these pointer fields have no default mapped and are not listed in "
                 "NO_NAMED_DEFAULT, so the page would call them optional without "
                 "saying what unset means: " + ", ".join(unclaimed),
                 fields=unclaimed)
    return blocks


# ----------------------------------------------------------------- cli -> page


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
    a = _anchors()
    out = os.path.join(IBC, a["cli_module"], "bin", a["binary"])
    if os.environ.get("REFGEN_NO_BUILD") and os.path.exists(out):
        return out

    # Keyed by what the sources say, not by where they are. Two runs over the
    # same code share a binary, which matters now that the config page probes
    # one too -- and a fixed output path was a race between concurrent runs,
    # because each would overwrite the other's binary mid-read.
    # every Go file in the repository, not just this module's: the CLI depends
    # on a second module through a `replace`, and keying on one module alone
    # would serve a stale binary after the other changed -- a cache that
    # documents a CLI which no longer exists.
    digest = hashlib.sha256()
    module = os.path.join(IBC, a["cli_module"])
    for root, dirs, files in os.walk(IBC):
        dirs[:] = sorted(d for d in dirs if d not in _SKIP_DIRS)
        for name in sorted(files):
            if not name.endswith((".go", ".mod", ".sum")):
                continue
            path = os.path.join(root, name)
            digest.update(os.path.relpath(path, IBC).encode())
            with open(path, "rb") as fh:
                digest.update(fh.read())
    key = digest.hexdigest()[:16]
    cached = os.path.join(tempfile.gettempdir(), f"refgen-cli-{key}")
    if os.path.exists(cached):
        return cached

    r = subprocess.run(["go", "build", "-o", os.path.join("bin", a["binary"]),
                        "./" + a["cli_pkg"] + "/..."],
                       cwd=module, capture_output=True, text=True)
    if r.returncode != 0:
        raise SourceError(f"go build failed:\n{r.stderr}")
    shutil.copyfile(out, cached)
    os.chmod(cached, 0o755)
    return cached


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
        # The last parenthetical, not the first. `re.search` is leftmost and
        # `(.+)` is greedy, so a flag carrying both the author's `(default: x)`
        # and Cobra's `(default "y")` captured everything between the first
        # opener and the last closer, publishing `x) (default "y` as the
        # default. Cobra's comes last because Cobra appends it, and it is the
        # binary's own answer, so it wins; the author's parenthetical stays in
        # the description, which is where prose about a computed default reads.
        d = None
        if f["doc"].endswith(")"):
            for m in re.finditer(r"\s*\(default:?\s+", f["doc"]):
                d = m
        if d:
            f["default"] = f["doc"][d.end():-1].strip().strip('"')
            f["doc"] = f["doc"][:d.start()].rstrip()
        else:
            f["default"] = ""
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


# ------------------------------------------------- required flags, by asking

# Cobra reports required flags when a command is run without them, so the
# honest source is the binary, not the wiring that registered them. Reading the
# wiring meant matching Go variable names (`cmdDeployClient`), the
# `&cobra.Command{}` literal, and the root's `AddCommand(` with hand-balanced
# parentheses. None of that is visible to a reader, all of it is free to change,
# and every change to it stopped a developer to teach this file a new name.
PROBE_PLACEHOLDER = "refgen-probe"

# Cobra validates positional arguments before required flags, so a command with
# arity answers the arity question first and never mentions its flags. Read the
# count it asks for, supply that many placeholders, and ask again. Placeholders
# are never executed: validation fails before RunE runs.
_ARITY = [
    (re.compile(r"accepts between (\d+) and \d+ arg\(s\)"), 1),
    (re.compile(r"accepts (\d+) arg\(s\)"), 1),
    (re.compile(r"accepts at least (\d+) arg\(s\)"), 1),
    (re.compile(r"requires at least (\d+) arg\(s\)"), 1),
]
_REQUIRED = re.compile(r'required flag\(s\) ((?:"[\w.-]+"(?:, )?)+) not set')

# Cobra has more than one way to say it, and a command can also check by hand
# in RunE. Reading only the first sentence meant `ibc deploy core`, which exits
# with `Error: --chain is required`, rendered every flag as optional.
_REQUIRED_ALSO = [
    re.compile(r"(?:^|\s)--([\w-]+) is required"),
    re.compile(r'flag "?--?([\w-]+)"? is required'),
]

# A rule over a group of flags -- at least one of these, or all or none -- makes
# no single flag mandatory. Reading the group as a flag name produced a "flag"
# called `alpha beta`, which matches nothing, so both real flags rendered
# optional while the binary refused to run without one.
PROBE_TIMEOUT = 20

# Run once to give the throwaway home a config, so a command gets past loading
# one and reaches its own checks. Cobra's own required flags are validated
# before RunE and are seen either way; a command that checks a flag by hand
# inside RunE is only reachable once it can start. Best effort: if this command
# does not exist the probe simply sees less, and says so per command.
PROBE_SEED = ["config", "new"]

# Commands whose required-flag answer is a floor: the probe stopped on something
# that was not a missing flag, so a flag checked only afterwards is unread. The
# report names them, because `required` on one row and blank on another reads as
# a deliberate distinction rather than an incomplete answer.
INCOMPLETE_PROBES = []


def _probe_run(binary, argv, home):
    """Run the CLI once, unable to touch anything the developer owns.

    A command with no required flag is not stopped by the flag check, so the
    probe really does run it. `ibc migrate down` rolled back a migration and
    `ibc config new` wrote into the developer's home before this was sandboxed.
    So: a throwaway home, no stdin to read, and a timeout, because `relayer
    run` is a server and would otherwise never return.
    """
    env = dict(os.environ, HOME=home, XDG_CONFIG_HOME=os.path.join(home, ".config"))
    try:
        return subprocess.run([binary] + argv + ["--home", home],
                              capture_output=True, text=True, env=env,
                              stdin=subprocess.DEVNULL, timeout=PROBE_TIMEOUT)
    except subprocess.TimeoutExpired:
        return None


# The probe stopped at an error that is not about a missing flag: a value it
# supplied was rejected, or the command needs state it does not have. Anything
# the command would have asked for next is unread, so the list is a floor
# rather than the answer.
_WALL = object()


def _error_line(out):
    """The line the command failed on, and nothing after it.

    What follows is the usage block, which prints every flag with its help
    text. Reading that too let a sentence in a flag's description -- `required
    only for remote signers` -- mark the flag required.
    """
    for line in out.splitlines():
        if line.startswith("Error:"):
            return line
    return out.split("\n", 1)[0]


def _probe_once(binary, path, supplied, home, max_args=8):
    """What one run of a command says it is missing, given what it was handed.

    Returns a list of flag names, or None when the run never reached flag
    validation, which the caller reports rather than reads as `nothing is
    required`. A command group returns [] because running one prints help.
    """
    positional = []
    while True:
        r = _probe_run(binary, path.split() + positional + list(supplied), home)
        if r is None:
            return None
        line = _error_line((r.stderr or "") + (r.stdout or ""))
        m = _REQUIRED.search(line)
        if m:
            return sorted(re.findall(r'"([\w.-]+)"', m.group(1)))
        for rx in _REQUIRED_ALSO:
            extra = rx.search(line)
            if extra:
                return [extra.group(1)]
        want = None
        for rx, group in _ARITY:
            am = rx.search(line)
            if am:
                want = int(am.group(group))
                break
        if want is None:
            # ran clean, or stopped on something that is not a missing flag.
            # The difference matters: the first means nothing more is required,
            # the second means the asking stopped early.
            return [] if r.returncode == 0 else _WALL
        if want <= len(positional) or want > max_args:
            return None
        positional = [PROBE_PLACEHOLDER] * want


def _probe_required(binary, path, flag_types, home, rounds=8):
    """Every flag a command refuses to run without.

    One answer is not the answer. The command framework reports the flags it
    was told are required before the command body runs, so a command that also
    checks one by hand never mentions the second until the first is supplied:
    `deploy client` names `--counterparty-chain`, and only then `--chain`. So
    supply what has been learned and ask again, until asking stops naming
    anything new.

    What this cannot reach: a check that runs only after an earlier value is
    accepted. The probe supplies a placeholder, the command rejects it as not
    naming anything real, and whatever it would have asked for next is unread.
    `deploy render-config` requires `--signer-b` and says so only once
    `--signer-a` names a signer that exists. So a list here is a floor. It is
    the same floor the wiring scraper had -- it saw neither flag -- and the
    tables are no worse for it, but it is not the whole truth and should not
    be read as one.
    """
    found, supplied, reached = set(), [], False
    for _ in range(rounds):
        got = _probe_once(binary, path, supplied, home)
        if got is None:
            return (sorted(found), "unreachable") if reached else (None, "unreachable")
        if got is _WALL:
            # A wall before anything was found says only that the command needs
            # state the probe does not have, which is true of most of them. A
            # wall *after* a flag was found is the interesting one: the command
            # was still answering, and stopped.
            return sorted(found), "partial" if found else "blocked"
        reached = True
        new = set(got) - found
        if not new:
            return sorted(found), "complete"
        found |= new
        supplied = []
        for name in sorted(found):
            supplied.append("--" + name)
            if flag_types.get(name, "string") != "bool":
                supplied.append(PROBE_PLACEHOLDER)
    return sorted(found), "partial"


def required_flags(binary, tree):
    """{command path: {flag, ...}} for every node in the tree.

    A flag is required at a node when it is required on every runnable leaf
    beneath it, which is what a persistent required flag means. Marking it on
    the node as well as the leaf is what puts `required` on the inherited rows
    of each leaf's table.
    """
    leaves = [p for p, node in tree.items() if not node["subs"]]
    per_leaf, unreachable, incomplete = {}, [], []
    with tempfile.TemporaryDirectory(prefix="refgen-probe-") as home:
        seeded = _probe_run(binary, list(PROBE_SEED), home)
        if seeded is None or seeded.returncode != 0:
            _problem("unseeded_probe",
                     f"`{' '.join(PROBE_SEED)}` did not initialise the probe's "
                     "throwaway home, so a command that needs a config cannot "
                     "start and the checks it makes itself stay unread. Flags "
                     "the command framework was told about are still found.",
                     seed=list(PROBE_SEED))
        for leaf in sorted(leaves):
            types = {}
            for node, node_tree in tree.items():
                if node == "" or leaf == node or leaf.startswith(node + " "):
                    types.update({f["name"]: f["type"] for f in node_tree["flags"]})
            got, how = _probe_required(binary, leaf, types, home)
            if got is None:
                unreachable.append(leaf)
                got = []
            elif how != "complete" and got:
                # It stopped on something that was not a missing flag, so what
                # it found is a floor. Only worth naming when it found
                # something: a column of blanks claims nothing, but `required`
                # on one row and blank on the next reads as a distinction the
                # tool did not actually make.
                incomplete.append(leaf)
            per_leaf[leaf] = set(got)
    if unreachable:
        _problem("unprobed_command",
                 "these commands never reached flag validation, so their required "
                 f"flags could not be read: {sorted(unreachable)}. Their tables "
                 "would claim every flag is optional.",
                 commands=sorted(unreachable))

    # The canary. If Cobra ever rewords this message the probe matches nothing,
    # every table quietly loses its `required` marks, and the page still renders
    # -- which is what a green check looks like when a generator has gone blind.
    if not any(per_leaf.values()) and not os.environ.get("REFGEN_NO_REQUIRED_FLAGS"):
        raise SourceError(kind="all_flags_optional", message=(
            "no command reports a required flag. Either the CLI genuinely has "
            "none, or Cobra no longer says `required flag(s) \"x\" not set` and "
            "this probe now reads every flag as optional. Confirm which, then "
            "set REFGEN_NO_REQUIRED_FLAGS=1 if the CLI really has none."))

    INCOMPLETE_PROBES.clear()
    INCOMPLETE_PROBES.extend(sorted(incomplete))

    out = {}
    for node, info in tree.items():
        if node in per_leaf:
            # a command answers for itself, including the flags it inherits:
            # `--chain` is persistent on `deploy` and required by `deploy core`
            # alone, which only the leaf can say
            out[node] = per_leaf[node]
            continue
        under = [p for p in leaves if node == "" or p.startswith(node + " ")]
        shared = set.intersection(*(per_leaf[p] for p in under)) if under else set()
        # and only over flags the node itself declares, so a group is never
        # credited with a flag private to the one command beneath it
        out[node] = shared & {f["name"] for f in info["flags"]}
    return out


def cli_citation_lines():
    """Where to point a reader, which is the one thing only the source knows.

    Best effort by design. A citation names a file and a line; a line this
    cannot find degrades to the file, and nothing here raises. Facts come from
    the binary, so a miss here costs a reader some precision and cannot make a
    table wrong.
    """
    lines = {}
    try:
        wiring = _read(_anchors()["cli_main"])
    except OSError:
        return lines

    src = "\n".join(open(os.path.join(IBC, _anchors()["cli_src"], f)).read()
                    for f in sorted(os.listdir(os.path.join(IBC, _anchors()["cli_src"])))
                    if f.endswith(".go") and not f.endswith("_test.go"))
    consts = {m.group(1): m.group(2)
              for m in re.finditer(r'(\w+)\s*=\s*"([\w-]+)"', src)}

    # Any identifier assigned a cobra.Command, not just the ones spelled
    # `cmdX`. Matching the prefix meant a command variable renamed to
    # `deployClientCmd` fell out of the tree and took its citation with it.
    use = {}
    for m in re.finditer(r"(\w+)\s*=\s*&cobra\.Command\{", src):
        tail = src[m.end():m.end() + 400]
        u = re.search(r'Use:\s*(?:"([\w-]+)|(\w+))', tail)
        if u:
            use[m.group(1)] = u.group(1) or consts.get(u.group(2), u.group(2))

    parent = {}
    for m in re.finditer(r"(\w+)\.AddCommand\(", wiring):
        depth, i = 1, m.end()
        while depth and i < len(wiring):
            depth += {"(": 1, ")": -1}.get(wiring[i], 0)
            i += 1
        for child in re.findall(r"\w+", wiring[m.end():i - 1]):
            if child in use:
                parent[child] = m.group(1)

    # The root command is the one nothing adds as a child. Its own name is the
    # binary, which is not part of any command's path.
    roots = set(use) - set(parent)

    def path_of(var):
        parts, seen = [], set()
        while var in use and var not in seen:
            seen.add(var)
            if var not in roots:
                parts.insert(0, use[var])
            var = parent.get(var, "")
        return " ".join(parts)

    # where the tree is assembled, cited by tables that span commands. Found by
    # whichever variable is the root rather than by it being called `rootCmd`.
    for m in re.finditer(r"(\w+)\.AddCommand\(", wiring):
        if m.group(1) in roots:
            lines[""] = wiring[:m.start()].count("\n") + 1
            break
    try:
        flags_src = _read(_anchors()["global_flags_file"])
        # the function that declares them, whatever it is called: the one whose
        # body makes the cobra call
        for m in re.finditer(r"^func \w+\(", flags_src, re.M):
            body, _end = _go_block(flags_src, m.end())
            if "PersistentFlags()" in body:
                lines["__global__"] = flags_src[:m.start()].count("\n") + 1
                break
    except (OSError, SourceError, ValueError):
        pass

    for var in use:
        m = re.search(r"\b" + var + r"\.(?:Persistent)?Flags\(\)", wiring)
        if m:
            lines[path_of(var)] = wiring[:m.start()].count("\n") + 1
    for m in re.finditer(r"for _, c := range \[\]\*cobra\.Command\{([^}]*)\}\s*\{", wiring):
        loop_line = wiring[:m.start()].count("\n") + 1
        for var in re.findall(r"\w+", m.group(1)):
            if var in use:
                lines.setdefault(path_of(var), loop_line)
    return lines


def _flag_name(f):
    """The flag as a reader types it, with its type in the signature.

    A separate Type column spends a column on five characters. Cobra already
    writes the type after the flag, and a bool takes no value at all.
    """
    lead = f"-{f['short']}, --{f['name']}" if f["short"] else f"--{f['name']}"
    return f"`{lead}`" if f["type"] == "bool" else f"`{lead} <{f['type']}>`"


def _flag_rows(flags, path, required_map):
    """Flag, Default, Description. Required-ness shows in the Default column,
    because a required flag is precisely one with no default."""
    required = required_map.get(path, set())
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
    required = required_flags(binary, tree)
    lines = cli_citation_lines()
    blocks = {}

    leaves = sorted(p for p, c in tree.items() if p and not c["subs"])
    main_go = _anchors()["cli_main"]

    def where(path):
        """Cite the line in main.go that registers this command's flags, or
        the line that assembles the tree for a table that spans commands."""
        return cite(main_go, lines.get(path, lines.get("")))

    tree_citation = where("")

    blocks["cli:global-flags"] = (
        table(FLAG_COLUMNS,
              _flag_rows(_parse_flags(tree[""]["help"], "Flags:"), "", required))
        + "\n\n" + cite(_anchors()["global_flags_file"], lines.get("__global__")))

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
    # What a command inherits is read from the command's own help, not inferred
    # from its ancestor's. Cobra prints a group's local and persistent flags
    # together under `Flags:`, so `.Flags()` where `.PersistentFlags()` was
    # meant was indistinguishable there -- and put a row on all eight commands
    # under `deploy`, each of which answers `unknown flag`. The child's help
    # separates them: `Global Flags:` is exactly what it inherits. The binary
    # already answered this question; the tool was throwing the answer away.
    root_flags = {f["name"] for f in _parse_flags(tree[""]["help"], "Flags:")}

    def inherited_by(path):
        """Flags `path` inherits from a group, the root's excluded.

        Root flags are documented once at the top of the page rather than on
        each of the 28 commands, so they come out here.
        """
        return [f for f in _parse_flags(tree[path]["help"], "Global Flags:")
                if f["name"] not in root_flags]

    for path in leaves:
        rows = sorted(_flag_rows(tree[path]["flags"], path, required))
        for f in sorted(inherited_by(path), key=lambda e: e["name"]):
            # the flag is declared on the group and required, or not, by this
            # command
            rows += _flag_rows([f], path, required)
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
            continue
        if not node_tree["subs"]:
            continue          # a leaf's own flags are documented in its own table
        # A group flag no command under it actually inherits is registered with
        # `.Flags()` rather than `.PersistentFlags()`. It reaches no table now
        # that inheritance is read from the child, and a flag documented
        # nowhere is the same failure as one documented everywhere falsely --
        # so it stops here rather than going quiet.
        reaches = set()
        for p in under:
            reaches.update(f["name"] for f in inherited_by(p))
        orphans = [f["name"] for f in node_tree["flags"]
                   if f["name"] not in reaches and f["name"] not in root_flags]
        if orphans:
            _problem("uninherited_flag",
                     f"`ibc {node}` registers {', '.join('--' + o for o in orphans)} "
                     "but no command under it inherits them, so they appear in no "
                     "table. `.Flags()` registers a flag on the group alone; "
                     "`.PersistentFlags()` is what makes it reach the commands.",
                     node=node, flags=orphans)

    blocks["notice"] = _notice()
    for region, body in blocks.items():
        if "IBC Link" in body:
            _problem("retired_name",
                     f"{region} carries the retired product name; the page cannot",
                     region=region)
    return blocks


# Every page says, where only a writer can see it, which half of it is derived
# and which half is theirs. Generated like everything else, so it cannot drift
# from the truth and cannot be quietly deleted: the page owes a marker for it.
NOTICE = """Tables between GEN markers on this page are generated from this
repository by docs/6-ibc-cli/tools/refgen.py. Do not edit inside them: the next
run overwrites whatever is there, so a hand edit looks like a fix and is not.
The prose around them is written by hand and is yours to change.

After changing cli/, proto/ or gen/, follow docs/6-ibc-cli/tools/AGENTS.md
before opening a pull request."""


def _notice():
    open_, close = COMMENT
    return f"{open_}\n{NOTICE}\n{close}"


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


def _dropped_requirements(text, blocks):
    """Keys the page calls required that the regenerated tables do not.

    The most consequential diff this tool produces and the least obvious: it
    reads as ordinary drift. A key stops being required when the code stops
    requiring it, and also when a validation message is reworded past the words
    `_requirement` reads -- and only the first of those is a real change.
    """
    was = _requirements(text)
    now = {}
    for region, body in blocks.items():
        for line in body.split("\n"):
            row = ROW.match(line)
            if row:
                now[(region, row.group(1))] = bool(_SAYS_REQUIRED.search(row.group(2)))
    return [{
        "kind": "requirement_dropped",
        "message": f"{key} in {region} is documented as required and the "
                   "regenerated table does not call it required. A key stops "
                   "being required when the code stops requiring it, and also "
                   "when a validation message is reworded past the words this "
                   "tool reads. Confirm which before accepting.",
        "region": region, "key": key,
    } for (region, key), required in sorted(was.items())
        if required and region in blocks and not now.get((region, key), True)]


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

    # the config walk runs more than once per generation, so a problem raised
    # inside it is recorded each time. One gap should be one line of work.
    seen, unique = set(), []
    for c in curation:
        fingerprint = (c["kind"], c["message"])
        if fingerprint not in seen:
            seen.add(fingerprint)
            unique.append(c)
    curation = unique

    text = open(path).read()
    regions = find_regions(text)
    present = [i for i, *_ in regions]
    order = list(blocks)

    # A key the page calls required and the regeneration does not is the most
    # consequential diff this tool produces and the least obvious: it reads as
    # ordinary drift. It belongs in the work order, and from there in the pull
    # request, rather than in a line of stderr nobody keeps.
    curation += _dropped_requirements(text, blocks)

    def after(region):
        """The last region already on the page that precedes this one in source
        order, which is where a new section belongs."""
        if region == "notice":
            return None     # belongs at the top, under the frontmatter
        i = order.index(region)
        earlier = [r for r in order[:i] if r in present]
        return earlier[-1] if earlier else None

    return {
        "page": os.path.relpath(path, ROOT) if path.startswith(ROOT) else path,
        "kind": kind,
        "regions": len(blocks),
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
    if region == "notice":
        return ""          # not a section: an invisible comment, placed first
    parts = region.split(":")
    if parts[0] == "config":
        name = re.sub(r"Config$", "", parts[-1])
        return "### `" + name[0].lower() + name[1:] + "`"
    if parts[0] == "api" and parts[1] == "msg":
        return f"#### `{parts[-1]}`"
    if parts[0] == "api" and parts[1] == "enum":
        return f"#### `{parts[-1]}`"
    if parts[:2] == ["cli", "cmd"]:
        return "### `ibc " + parts[-1].replace("-", " ") + "`"
    if parts[:2] == ["cli", "group-flags"]:
        return "### Flags every `" + parts[-1].replace("-", " ") + "` command accepts"
    return f"## {parts[-1]}"


ROW = re.compile(r"^\|\s*(`[^`]+`)\s*\|(.*)\|\s*$")


# The config tables bold it and the flag tables do not, so both spellings
# count. Reading only one meant this saw the config page and not the CLI one --
# and required-ness on the CLI page is the half read out of a running binary.
_SAYS_REQUIRED = re.compile(r"(?:^|\|)\s*(?:\*\*required\*\*|required)\s*(?:\||$)")


def _requirements(text):
    """{row key: True when the row says the key is required}, for one page.

    Rows outside a generated region are not this tool's to judge, so they are
    skipped rather than allowed to overwrite a generated row with the same key.
    """
    out = {}
    region = ""
    for line in text.split("\n"):
        m = START.search(line)
        if m:
            region = m.group("id")
            continue
        if END.search(line):
            region = ""
            continue
        if not region:
            continue
        r = ROW.match(line)
        if r:
            out[(region, r.group(1))] = bool(_SAYS_REQUIRED.search(r.group(2)))
    return out


def _downgrades(before, after):
    """Keys this regeneration stops calling required.

    A reworded validation message moves a key from required to optional and
    shows up as an ordinary-looking diff. It is a diff worth reading twice,
    because accepting it tells every reader a mandatory key is theirs to omit.
    """
    was, now = _requirements(before), _requirements(after)
    return sorted(k for k, req in was.items() if req and not now.get(k, True))


def _report_downgrades(path, before, after):
    lost = _downgrades(before, after)
    if not lost:
        return
    print(f"{path}: NOTE -- this drops `required` from "
          f"{len(lost)} key{'s' if len(lost) > 1 else ''}:", file=sys.stderr)
    for region, key in lost:
        print(f"  {key} in {region}", file=sys.stderr)
    print("  A key stops being required when the code stops requiring it, and "
          "also when a validation message is reworded past the words this tool "
          "reads. Confirm which before accepting.", file=sys.stderr)


def run(kind, path, check):
    blocks = GENERATORS[kind]()
    text = open(path).read()
    _check_notice_placement(text, path)
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
        _report_downgrades(path, text, new)
        return 1
    _report_downgrades(path, text, new)
    open(path, "w").write(new)
    print(f"{path}: regenerated {len(wanted)} regions")
    return 0


def report(plans):
    """A short account of what the tool did and what it could not do.

    Two audiences and one rule: a reader has to be able to tell what was
    derived from source from what a person or an agent wrote. The first is as
    true as the code; the second is only as true as whoever wrote it, and is
    what a reviewer needs to spend their attention on.

    The tool can only account for its own half. Whatever an agent writes --
    prose, a doc comment, a description -- it appends under the second heading,
    one line each, naming where it went.
    """
    out = ["## Reference documentation", ""]
    refused = [p for p in plans if p.get("refused")]
    readable = [p for p in plans if not p.get("refused")]
    derived = sum(p["regions"] for p in readable)
    out.append(f"Derived from source: {derived} regions across "
               f"{len(readable)} page{'' if len(readable) == 1 else 's'}.")
    if refused:
        out += ["", "**Could not be read at all, so nothing below accounts for "
                    f"{'it' if len(refused) == 1 else 'them'}:**"]
        for p in refused:
            out.append(f"- {os.path.basename(p['page'])}: {p['refused']}")

    stale = [(p, r) for p in plans for r in p["stale"]]
    if stale:
        out.append(f"{len(stale)} do not match the source:")
        for pl in plans:
            if pl["stale"]:
                out.append(f"- {os.path.basename(pl['page'])}: "
                       + ", ".join(f"`{r}`" for r in pl["stale"]))

    gaps = [(pl, c) for pl in plans for c in pl["curation"]]
    missing = [(pl, m) for pl in plans for m in pl["missing_marker"]]
    orphans = [(pl, o) for pl in plans for o in pl["orphaned_marker"]]

    if missing:
        out += ["", "The source has these and the pages have nowhere to put them:"]
        for _pl, m in missing:
            out.append(f"- `{m['region']}` -- suggested heading {m['suggested_heading']}")
    if orphans:
        out += ["", "The pages describe these and the source no longer has them:"]
        for _pl, o in orphans:
            out.append(f"- `{o['region']}`")
    if INCOMPLETE_PROBES:
        out += ["", "- `required` on the command tables is a floor. It is read by "
                    "running each command, which stops at the first value it "
                    "rejects, so a flag a command checks only after that is "
                    "unread. A blank cell means *not seen*, not *optional*."]
    if gaps:
        out += ["", "Needs a decision the source cannot make:"]
        for _pl, c in gaps:
            at = f" ({c['file']}:{c['line']})" if c.get("file") and c.get("line") else ""
            # the first sentence, not the first period: a message names files,
            # and `config.go` is not the end of one
            first = re.split(r"(?<=[a-z0-9)])\.\s", c["message"], maxsplit=1)[0]
            out.append(f"- {c['kind']}{at}: {first.rstrip('.')}.")

    if not (stale or missing or orphans or gaps or refused):
        out += ["", "Nothing to do: every table matches the source."]
    elif refused and not (stale or missing or orphans or gaps):
        out += ["", "Nothing else to report, but the page above was never read, "
                    "so that is not the same as nothing being wrong."]
    out += ["", "### Written by hand, not derived", "",
            "_Nothing yet. Anything written rather than generated belongs here, "
            "one line each, so a reviewer knows where to look._"]
    return "\n".join(out)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("kind", choices=sorted(GENERATORS) + ["all"])
    ap.add_argument("page", nargs="?", help="omit for kind `all`")
    ap.add_argument("--check", action="store_true")
    ap.add_argument("--list-regions", action="store_true",
                    help="print the stale region ids, one per line, and nothing else")
    ap.add_argument("--plan", action="store_true",
                    help="print, as JSON, every gap and the work each one needs")
    ap.add_argument("--report", action="store_true",
                    help="print a short account of what was derived and what "
                         "still needs a person, for a pull request description")
    a = ap.parse_args()
    jobs = [(k, os.path.join(ROOT, p)) for k, p in sorted(PAGES.items())] \
        if a.kind == "all" else [(a.kind, a.page)]
    if a.kind != "all" and not a.page:
        ap.error("a page is required unless kind is `all`")
    rc, plans = 0, []
    for kind, page in jobs:
        try:
            if a.plan or a.report:
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
            if a.plan or a.report:
                # a page the tool could not read has to appear in the work
                # order. Leaving it out is how a report comes to say every
                # table matches the source about a page nobody could parse.
                plans.append({
                    "page": os.path.relpath(page, ROOT) if page.startswith(ROOT) else page,
                    "kind": kind, "regions": 0, "refused": str(e),
                    "stale": [], "missing_marker": [], "orphaned_marker": [],
                    "curation": [],
                })
    if a.plan or a.report:
        print(json.dumps(plans, indent=2) if a.plan else report(plans))
        work = sum(len(p["stale"]) + len(p["missing_marker"])
                   + len(p["orphaned_marker"]) + len(p["curation"]) for p in plans)
        # a refusal outranks staleness; never report a quiet exit over a page
        # the tool could not read
        rc = max(rc, 1 if work else 0)
    return rc


if __name__ == "__main__":
    sys.exit(main())
