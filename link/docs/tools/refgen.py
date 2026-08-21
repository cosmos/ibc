#!/usr/bin/env python3
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
THIS IS THE MOVED COPY
--------------------------------------------------------------------------
This file imports nothing from the docs project, so it can live beside the
code it reads and let the team that changes a flag own the docs that describe
it. This checklist lives here, rather than in the docs repo's notes, because
this file is the thing that travels.

  1. Copy `refgen.py`, `test-refgen.py`, and `test-refgen-e2e.py`.
  2. Set IBC to the repo root. Here it points at a pinned clone in `repos/ibc`;
     upstream it is the repo itself, so `--check` runs against the working tree
     and a developer sees their own change.
  3. Copy the reference pages, or point PAGES at wherever they live. PAGES is
     the coverage contract: a region with no marker on its owning page is an
     error, which is what stops a new key from landing in the generator and
     never reaching a reader.
  4. Copy the workflow. Switch its `paths` to `link/**` and `proto/**`, which
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
import os
import re
import subprocess
import sys

# upstream layout: this file is link/docs/tools/refgen.py, so the repo root is
# three levels up, and the source it reads is the working tree itself
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
    """Yield (doc, statement) for each `;`-terminated statement in a body,
    carrying the comment lines that precede it. Statements may share a line."""
    doc = []
    for raw in body.split("\n"):
        line = raw.strip()
        if line.startswith("//"):
            doc.append(line[2:].strip())
            continue
        for part in line.split(";"):
            part = part.strip()
            if not part:
                continue
            yield " ".join(doc), part
            doc = []


def _fields(body):
    """Fields of a message body, in declaration order, with a oneof folded
    into one entry."""
    oneofs = {name: {"name": name, "type": "oneof", "doc": doc,
                     "opts": [f.group(3) for _d, s in _statements(inner)
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
            for rdoc, stmt in _statements(body):
                m = RPC.match(stmt)
                if m:
                    rpcs.append({"name": m.group(1), "req": m.group(2),
                                 "resp": m.group(3), "doc": rdoc})
            out["services"].append({"name": name, "doc": doc, "line": line, "rpcs": rpcs})
        elif kind == "message":
            out["messages"].append({"name": name, "doc": doc, "line": line,
                                    "fields": _fields(body)})
        else:
            values = []
            for vdoc, stmt in _statements(body):
                m = re.match(r"(\w+)\s*=\s*\d+", stmt)
                if m:
                    values.append({"name": m.group(1), "doc": vdoc})
            out["enums"].append({"name": name, "doc": doc, "line": line, "values": values})
    return out

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


def gen_api():
    blocks = {}
    for fname, short in (("proto/link/relayer.proto", "relayer"),
                         ("proto/link/attestor.proto", "attestor")):
        p = parse_proto(fname)
        for svc in p["services"]:
            rows = [(f"`{r['name']}`", f"`{r['req']}`", f"`{r['resp']}`",
                     _lead_strip(r["name"], r["doc"])) for r in svc["rpcs"]]
            blocks[f"api:{short}:rpcs"] = (
                table(["RPC", "Request", "Response", "What it does"], rows)
                + "\n\n" + cite(fname, svc["line"]))
        for msg in p["messages"]:
            if not msg["fields"]:
                continue
            rows, names = [], {f["name"] for f in msg["fields"]}
            for f in msg["fields"]:
                t = f"oneof: {' or '.join('`'+o+'`' for o in f['opts'])}" if f["type"] == "oneof" else f"`{f['type']}`"
                doc = _fence(_lead_strip(f["name"], f["doc"]), names - {f["name"]})
                rows.append((f"`{f['name']}`", t, doc))
            blocks[f"api:msg:{msg['name']}"] = (
                table(["Field", "Type", "Description"], rows)
                + "\n\n" + cite(fname, msg["line"]))
        for en in p["enums"]:
            rows = [(f"`{v['name']}`", v["doc"]) for v in en["values"] if not v["name"].endswith("UNSPECIFIED")]
            blocks[f"api:enum:{en['name']}"] = (
                table(["Value", "Meaning"], rows) + "\n\n" + cite(fname, en["line"]))
    return blocks


# ------------------------------------------------------------- go -> config

CONFIG_FILES = ("link/internal/config/config.go", "link/internal/config/relayer.go")

# Where a pointer field's default lives when the struct itself carries no
# value: a named constant in the code that consumes the field. The label in
# each entry is prose; the value is always read from source, and a missing
# constant is an error rather than a stale number.
DEFAULT_CONSTS = {
    ("RelayerConfig", "DispatchPollInterval"): [
        ("", "link/internal/relay/dispatch/dispatcher.go", "DefaultPollInterval")],
    ("RelayerChainOverride", "TxSubmissionDelay"): [
        ("", "link/internal/txsubmitter/evm/evm.go", "DefaultTxSubmissionDelay")],
    ("RelayerChainOverride", "PacketBatchSize"): [
        ("", "link/internal/relay/pipeline/opts.go", "DefaultBatchSize")],
    ("RelayerChainOverride", "PacketBatchTimeout"): [
        ("receive and acknowledge", "link/internal/relay/pipeline/opts.go", "DefaultBatchTimeout"),
        ("timeout", "link/internal/relay/pipeline/opts.go", "DefaultTimeoutBatchTimeout")],
}

# Keys the Go source does not document. Values never come from here, only
# wording. A key that gains a doc comment upstream raises, so this map cannot
# quietly outlive the gap it fills.
FALLBACK_DOCS = {
    ("ServerConfig", "ListenAddress"): "Address the gRPC server binds. It serves the relayer and attestor APIs together.",
    ("DBConfig", "Type"): "Database backend.",
    ("DBConfig", "URL"): "File path for sqlite, connection string for postgres. `:memory:` is rejected.",
    ("ChainConfig", "ChainID"): "The chain's id, as the chain reports it.",
    ("ChainConfig", "EVM"): "EVM connection details for the chain. See the table below.",
    ("EVMChainConfig", "RPC"): "JSON-RPC endpoint for the chain.",
    ("EVMChainConfig", "ICS26Router"): "Address of the ICS26 router on the chain.",
    ("AttestorConfig", "Type"): "Whether this process runs the attestor or queries it.",
    ("SignerConfig", "Type"): "Whether the key is a file on disk or a key held by a remote signer.",
    ("RelayerConfig", "DispatchPollInterval"): "How often the dispatcher polls the store for unfinished packets.",
    ("RelayerConfig", "ChainOverrides"): "Per-chain relay settings. See the table below.",
    ("RelayerConfig", "Connections"): "The connections this relayer relays over. See the table below.",
    ("RelayerChainOverride", "ChainID"): "The chain these settings apply to.",
    ("RelayerChainOverride", "EVM"): "EVM fee settings for the chain. See the table below.",
    ("RelayerChainOverride", "TxSubmissionDelay"): "Minimum delay between two transaction submissions on the chain.",
    ("RelayerChainOverride", "PacketBatchSize"): "How many packets the relayer puts in one transaction.",
    ("RelayerChainOverride", "PacketBatchTimeout"): "How long the relayer waits to fill a batch before submitting it.",
    ("RelayerEVMConfig", "GasFeeCapMultiplier"): "Multiplies the fee cap the node suggests.",
    ("RelayerEVMConfig", "GasTipCapMultiplier"): "Multiplies the tip cap the node suggests.",
    ("ConnectionConfig", "Alias"): "Name for the connection, unique in the file.",
    ("ConnectionConfig", "ClientA"): "One end of the connection. See the table below.",
    ("ConnectionConfig", "ClientB"): "The other end, on a different chain. Same keys as `clientA`.",
    ("ClientEnd", "ChainID"): "The chain this end's client lives on.",
    ("ClientEnd", "Signer"): "`signers` alias that submits relay transactions on this chain.",
    ("ClientEnd", "ClientID"): "The light client's id on this chain.",
    ("ClientEnd", "Type"): "Light client type.",
}

# autoRelay parses and has no consumer at this pin, so no reader-facing row
# describes it. See the TODO(autorelay) comment on the page.
SKIP_FIELDS = {("ClientEnd", "AutoRelay")}

# Each config block, and the structs that get their own table rather than
# being flattened into their parent's. A third element names the parent block
# and key, for a struct whose required-ness is validated one level up as a
# dotted path.
CONFIG_BLOCKS = [
    ("config:server", "ServerConfig"),
    ("config:db", "DBConfig"),
    ("config:chains", "ChainConfig"),
    ("config:chains:evm", "EVMChainConfig", ("ChainConfig", "evm")),
    ("config:relayer", "RelayerConfig"),
    ("config:relayer:chainOverrides", "RelayerChainOverride"),
    ("config:relayer:evm", "RelayerEVMConfig"),
    ("config:relayer:connections", "ConnectionConfig"),
    ("config:relayer:clientEnd", "ClientEnd"),
    ("config:attestors", "AttestorConfig"),
    ("config:signers", "SignerConfig"),
]

GO_TYPES = {"string": "string", "uint": "uint", "uint64": "uint64", "int": "int",
            "bool": "bool", "float64": "float64", "time.Duration": "duration"}


class SourceError(Exception):
    pass


def _read(path):
    return open(os.path.join(IBC, path)).read()


def parse_go_config():
    """Structs, string constants, literal defaults, and validation rules from
    the config package."""
    structs, aliases, consts, const_type, defaults, validations = {}, {}, {}, {}, {}, {}
    src = "\n".join(_read(f) for f in CONFIG_FILES)

    for m in re.finditer(r"type\s+(\w+)\s+\[\](\w+)", src):
        aliases[m.group(1)] = m.group(2)

    for m in re.finditer(r'(\w+)\s+(\w+)?\s*=\s*"([^"]*)"', src):
        consts[m.group(1)] = m.group(3)
        if m.group(2):
            const_type[m.group(1)] = m.group(2)

    for path in CONFIG_FILES:
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
                        tag = re.search(r'yaml:"([^",]+)', f.group(3) or "")
                        fields.append({"go": f.group(1), "type": f.group(2),
                                       "yaml": tag.group(1) if tag else f.group(1),
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
    for m in re.finditer(r"func \(c (\w+)\) Validate\(\) error \{", src):
        recv = m.group(1)
        tail = src[m.end():]
        tail = tail[:tail.index("\n}\n")]
        msgs = []
        for e in re.finditer(r'errors\.(?:New|Errorf)\("(\.[^"]+)"((?:,\s*\w+)*)', tail):
            msgs.append((e.group(1), [a.strip() for a in e.group(2).split(",") if a.strip()]))
        validations[recv] = msgs

    return {"structs": structs, "aliases": aliases, "consts": consts,
            "const_type": const_type, "defaults": defaults, "validations": validations}


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


def gen_config():
    model = parse_go_config()
    blocks = {}
    for region, struct, *rest in CONFIG_BLOCKS:
        parent = rest[0] if rest else None
        if struct not in model["structs"]:
            raise SourceError(f"struct {struct} not found in the config package")
        rows, cites = [], []
        for field in model["structs"][struct]["fields"]:
            if (struct, field["go"]) in SKIP_FIELDS:
                continue
            doc = _clean_doc(field)
            fallback = FALLBACK_DOCS.get((struct, field["go"]))
            if doc and fallback:
                raise SourceError(
                    f"{struct}.{field['go']} now has a doc comment; drop its FALLBACK_DOCS entry")
            if not doc and not fallback:
                raise SourceError(
                    f"{struct}.{field['go']} has no doc comment and no FALLBACK_DOCS entry")
            req, extra = _requirement(struct, field, model, parent)
            if extra:
                cites.extend(extra)
            rows.append((f"`{field['yaml']}`", _type_cell(struct, field, model),
                         req, doc or fallback))
        body = table(["Key", "Type", "Default or required", "Description"], rows)
        info = model["structs"][struct]
        body += "\n\n" + cite(info["file"], info["line"])
        for path, line in dict.fromkeys(cites):
            body += " " + cite(path, line)
        blocks[region] = body
    return blocks


# ----------------------------------------------------------------- cli -> page

CLI_DIR = "link"
CLI_SRC = "link/cmd/ibc"
CLI_BIN = "link/bin/ibc"

# Cobra generates these and nobody reads a page about them. Excluded here so
# the coverage assertion below still accounts for every other command.
CLI_EXCLUDED = {"completion", "help"}

# The reader's tasks, which are a human's call and not the command tree's.
# Every leaf command belongs to exactly one group, and gen_cli fails if a new
# command lands in none of them.
CLI_TASKS = [
    ("cli:task:setup", ["config new", "config add-chain", "config validate",
                        "keys new", "keys import"]),
    ("cli:task:deploy", ["deploy core", "deploy client", "deploy gmp", "deploy ift",
                         "deploy ift-bridge", "deploy render-config"]),
    ("cli:task:run", ["relayer run", "attestor run"]),
    ("cli:task:move", ["tx ift mint", "tx ift send", "relayer relay"]),
    ("cli:task:inspect", ["relayer status", "deploy status", "deploy show",
                          "query ift balance", "attestor info",
                          "attestor latest-height", "attestor state-attestation"]),
    ("cli:task:maintain", ["migrate up", "migrate down", "migrate status",
                           "keys list", "keys show"]),
]

# Command groups whose own flags every subcommand inherits.
CLI_GROUP_FLAGS = [("cli:group-flags:deploy", "deploy"),
                   ("cli:group-flags:query-ift", "query ift"),
                   ("cli:group-flags:tx-ift", "tx ift")]

# Commands that get their flags spelled out. A command with three or more of
# its own flags earns a subsection, and the generator fails if that set moves,
# because a new subsection needs hand-written prose and an example.
CLI_FLAG_SECTIONS = ["config add-chain", "deploy client", "deploy ift",
                     "deploy ift-bridge", "relayer relay", "relayer status",
                     "tx ift send"]
CLI_FLAG_THRESHOLD = 3


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
    lines[""] = wiring[:root.start()].count("\n") + 1
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


def _flag_rows(flags, path, source):
    """One row per flag, with required-ness folded into the default column the
    way the config tables do it."""
    required = source["required"].get(path, set())
    rows = []
    for f in flags:
        name = f"`--{f['name']}`"
        if f["short"]:
            name = f"`-{f['short']}, --{f['name']}`"
        if f["name"] in required:
            default = "**required**"
        elif f["default"] in ("", "false", "[]", "0"):
            default = ""
        elif " " in f["default"]:
            default = _prose(f["default"])   # a phrase, so only its tokens fence
        else:
            default = f"`{f['default']}`"
        rows.append((name, f"`{f['type']}`", default, _prose(f["doc"])))
    return rows


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
    missing = set(leaves) - wired
    if missing:
        raise SourceError(f"commands the binary has and the wiring does not: {sorted(missing)}")

    main_go = os.path.join(CLI_SRC, "main.go")

    def where(path):
        """Cite the line in main.go that registers this command's flags, or
        the line that assembles the tree for a table that spans commands."""
        return cite(main_go, source["lines"].get(path, source["lines"][""]))

    tree_citation = where("")

    blocks["cli:global-flags"] = (
        table(["Flag", "Type", "Default", "Description"],
              _flag_rows(_parse_flags(tree[""]["help"], "Flags:"), "", source))
        + "\n\n" + tree_citation)

    covered = []
    for region, commands in CLI_TASKS:
        rows = []
        for c in commands:
            if c not in tree:
                raise SourceError(f"{region} lists `ibc {c}`, which the binary does not have")
            rows.append((f"`ibc {c}`", _prose(tree[c]["short"])))
            covered.append(c)
        blocks[region] = table(["Command", "What it does"], rows) + "\n\n" + tree_citation

    if sorted(covered) != leaves:
        raise SourceError("every command needs a task group; unassigned: "
                          f"{sorted(set(leaves) - set(covered))}; "
                          f"listed twice: {sorted(c for c in covered if covered.count(c) > 1)}")

    for region, group in CLI_GROUP_FLAGS:
        flags = _parse_flags(tree[group]["help"], "Flags:")
        blocks[region] = (table(["Flag", "Type", "Default", "Description"],
                                _flag_rows(flags, group, source))
                          + "\n\n" + where(group))

    earned = sorted(p for p in leaves if len(tree[p]["flags"]) >= CLI_FLAG_THRESHOLD)
    if earned != sorted(CLI_FLAG_SECTIONS):
        raise SourceError(
            "the commands with their own flag section changed. "
            f"expected {sorted(CLI_FLAG_SECTIONS)}, source says {earned}. "
            "each one needs hand-written prose and an example, so add or remove "
            "the page section and its marker before updating CLI_FLAG_SECTIONS")
    for path in CLI_FLAG_SECTIONS:
        blocks[f"cli:flags:{_slug(path)}"] = (
            table(["Flag", "Type", "Default", "Description"],
                  _flag_rows(tree[path]["flags"], path, source))
            + "\n\n" + where(path))

    # Every flag has to appear somewhere. The commands below the threshold do
    # not earn a section of their own, so their flags collect in one table,
    # and the assertion after it means no flag can be silently undocumented.
    rest = [(p, f) for p in leaves if p not in CLI_FLAG_SECTIONS for f in tree[p]["flags"]]
    blocks["cli:remaining-flags"] = (
        table(["Command", "Flag", "Type", "Default", "Description"],
              [(f"`ibc {p}`",) + tuple(_flag_rows([f], p, source)[0])
               for p, f in rest])
        + "\n\n" + tree_citation)

    documented = set()
    for path in CLI_FLAG_SECTIONS:
        documented |= {(path, f["name"]) for f in tree[path]["flags"]}
    documented |= {(p, f["name"]) for p, f in rest}
    for group in (g for _r, g in CLI_GROUP_FLAGS):
        documented |= {(group, f["name"]) for f in _parse_flags(tree[group]["help"], "Flags:")}
    every = {(p, f["name"]) for p in leaves for f in tree[p]["flags"]}
    missing = sorted(f"ibc {p} --{n}" for p, n in every - documented)
    if missing:
        raise SourceError("flags on the page nowhere: " + ", ".join(missing))

    blocks["cli:all-commands"] = (
        table(["Command", "What it does"],
              [(f"`ibc {p}`", _prose(tree[p]["short"])) for p in leaves])
        + "\n\n" + tree_citation)

    for region, body in blocks.items():
        if "IBC Link" in body:
            raise SourceError(f"{region} carries the retired product name; the page cannot")
    return blocks


GENERATORS = {"api": gen_api, "cli": gen_cli, "config": gen_config}


# The page each generator owns. A page named here must carry a marker for
# every region its generator produces, so a new region cannot land in the
# generator and go missing from the page.
PAGES = {"config": "link/docs/configuration-generated.md",
         "cli": "link/docs/cli-commands.md",
         "api": "link/docs/api.md"}


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
    a = ap.parse_args()
    jobs = [(k, os.path.join(ROOT, p)) for k, p in sorted(PAGES.items())] \
        if a.kind == "all" else [(a.kind, a.page)]
    if a.kind != "all" and not a.page:
        ap.error("a page is required unless kind is `all`")
    rc = 0
    for kind, page in jobs:
        try:
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
    return rc


if __name__ == "__main__":
    sys.exit(main())
