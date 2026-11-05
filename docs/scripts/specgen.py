#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0

"""Generate docs/ibc/spec/ from the canonical spec/ at the repository root.

The specs are versioned source, not prose written for the docs site. They live
at spec/ and are edited there. This script renders them as Mintlify pages so
the site can publish them without a second copy drifting out of date -- which
is exactly what happened to the copy in cosmos/docs, frozen at 2026-04-09 and
missing the ICS-027 GMP spec entirely.

    python3 docs/scripts/specgen.py            # write docs/ibc/spec/
    python3 docs/scripts/specgen.py --check    # exit 1 if the tree is stale

The transform is deliberately narrow:

  1. .md -> .mdx
  2. internal links lose their .md suffix; a link written at a directory
     resolves to that directory's published README
  3. internal links become absolute site paths, resolved per file against the
     containing directory; a target that is excluded from publication or lies
     outside spec/ becomes a GitHub blob URL instead
     relative image paths become absolute site paths
  4. image files are copied alongside
  5. a file with no frontmatter gets one carrying only `title`, taken from its
     first H1 (11 of the publishable specs have none, and would otherwise be
     untitled pages). A file that already has frontmatter keeps it untouched.
     No frontmatter and no H1 is an error, never a filename fallback.
  6. constructs valid Markdown but not valid MDX are made valid, because the
     site compiles every page as MDX and one bad character fails the build:
       <!-- comment -->     ->  {/* comment */}
       <https://example>    ->  [https://example](https://example)
       <name,other>         ->  &lt;name,other>   (prose, not a tag)
  7. every page gets a two-line MDX comment header saying it is generated and
     how to regenerate it, so a contributor who lands on the file from a
     search result sees the contract without reading docs/AGENTS.md

Text inside fenced code blocks is never rewritten. A source whose code fences
do not pair up is rejected rather than rendered: an unclosed fence flips the
parity of every fence after it, so prose is silently treated as code and every
transform above is skipped for the rest of the file.
"""
import json
import os
import re
import shutil
import sys

ROOT = os.path.dirname(os.path.dirname(
    os.path.dirname(os.path.abspath(__file__))))
SRC = os.path.join(ROOT, "spec")
OUT = os.path.join(ROOT, "docs", "ibc", "spec")
SITE_BASE = "/ibc/spec"

# Governance and process documents. They describe how the standards process
# works, not what the standards say, and are read on GitHub.
EXCLUDE = {
    "CONTRIBUTING.md",
    "PROCESS.md",
    "STANDARDS_COMMITTEE.md",
    "ics-template.md",
}

IMAGE_EXT = (".png", ".svg", ".jpg", ".jpeg", ".gif", ".webp")

_FENCE = re.compile(r"(```.*?```|~~~.*?~~~)", re.S)
_LINK = re.compile(r"\]\((?![a-z][a-z0-9+.-]*:|/)([^)]+?\.md)(#[^)]*)?\)", re.I)
# A link written at a directory rather than a file: `](../ics-002-client-semantics)`.
# Published as <dir>/README, so without this the reader lands on a route that does
# not exist. `mint broken-links` does not resolve relative targets and will not
# catch it. The negative lookbehind keeps image syntax out.
_DIRLINK = re.compile(
    r"(?<!!)\[([^\]]*)\]\((?![a-z][a-z0-9+.-]*:|/|#)([^)#]+?)(#[^)]*)?\)")
GITHUB_BLOB = "https://github.com/cosmos/ibc/blob/main/"
_INLINE_CODE = re.compile(r"(`[^`\n]+`)")
_COMMENT = re.compile(r"<!--(.*?)-->", re.S)
_AUTOLINK = re.compile(r"<((?:https?://|mailto:)[^\s<>]+)>")
_PROSE_LT = re.compile(r"<(?=[A-Za-z_][\w-]*,)")
_IMAGE = re.compile(r"!\[([^\]]*)\]\((?!https?://|/)([^)]+?)\)")
_FENCE_MARK = re.compile(r"```|~~~")
_FRONTMATTER = re.compile(r"\A---[ \t]*\r?\n.*?\n---[ \t]*(?:\r?\n|$)", re.S)

# Emitted into every generated page. The three refgen pages carry an equivalent
# GEN:notice banner; before this, the specgen contract lived only in
# docs/AGENTS.md, invisible to anyone who arrived at the file directly.
HEADER = ("{{/* Generated from spec/{rel} by docs/scripts/specgen.py. "
          "Edits here are overwritten. */}}\n"
          "{{/* Regenerate: python3 docs/scripts/specgen.py */}}\n")


def excluded(rel):
    """True if this spec/-relative path is not published."""
    return rel in EXCLUDE or os.path.basename(rel) in EXCLUDE


def check_fences(text, rel):
    """Raise if `text` has an unclosed code fence.

    _FENCE pairs markers by position, so one unclosed fence inverts the
    code/prose split for the whole rest of the document: links are left
    unrewritten and prose is wrapped in a code block on the published page.
    Nothing downstream can see this -- --check compares the generator with
    itself, and the dead links it leaves point at .md files, which the site's
    broken-link check does not resolve. So refuse the source instead.
    """
    n = len(_FENCE_MARK.findall(text))
    if n % 2:
        raise ValueError(
            f"spec/{rel}: {n} code-fence markers, an odd number, so one fence "
            "is never closed. Close it in the source: an unclosed fence makes "
            "the rest of the file render as code and skips every rewrite.")


def _outside_fences(text, fn):
    """Apply fn to the parts of text that are not fenced code blocks."""
    return "".join(part if i % 2 else fn(part)
                   for i, part in enumerate(_FENCE.split(text)))


def _mdx_safe(text):
    """Make Markdown-only constructs valid MDX. Never called on code."""
    text = _COMMENT.sub(
        lambda m: "{/*" + m.group(1).replace("*/", "* /") + "*/}", text)
    text = _AUTOLINK.sub(lambda m: f"[{m.group(1)}]({m.group(1)})", text)
    return _PROSE_LT.sub("&lt;", text)


def _link(path, rel):
    """Resolve a relative .md link written in spec/<rel> to a published URL."""
    target = os.path.normpath(os.path.join(os.path.dirname(rel), path))
    if target.startswith("..") or excluded(target):
        # Never a page: point at the source on GitHub, derived from the path.
        return GITHUB_BLOB + os.path.normpath(os.path.join("spec", target))
    return SITE_BASE + "/" + target[:-len(".md")]


def _dir_link(path, rel):
    """Resolve a link written at a directory to its published README, or None.

    Returns None when the target is not a directory we publish, so the link is
    left exactly as written rather than rewritten to a guess.
    """
    if path.endswith("/"):
        path = path[:-1]
    if os.path.splitext(path)[1]:
        return None
    target = os.path.normpath(os.path.join(os.path.dirname(rel), path))
    if target.startswith(".."):
        return None
    readme = os.path.join(target, "README.md")
    if not os.path.isfile(os.path.join(SRC, readme)) or excluded(readme):
        return None
    return SITE_BASE + "/" + target + "/README"


def transform(text, rel):
    """Rewrite one document. `rel` is its path relative to spec/."""
    site_dir = SITE_BASE + "/" + os.path.dirname(rel) if os.path.dirname(rel) \
        else SITE_BASE

    def fix(chunk):
        chunk = _LINK.sub(lambda m: "](" + _link(m.group(1), rel)
                          + (m.group(2) or "") + ")", chunk)

        def dirlink(m):
            url = _dir_link(m.group(2), rel)
            if url is None:
                return m.group(0)
            return f"[{m.group(1)}]({url}{m.group(3) or ''})"

        chunk = _DIRLINK.sub(dirlink, chunk)

        def img(m):
            alt, path = m.group(1), m.group(2)
            if not path.lower().endswith(IMAGE_EXT):
                return m.group(0)
            clean = path[2:] if path.startswith("./") else path
            return f"![{alt}]({site_dir}/{clean})"

        chunk = _IMAGE.sub(img, chunk)
        return "".join(part if i % 2 else _mdx_safe(part)
                       for i, part in enumerate(_INLINE_CODE.split(chunk)))

    return _outside_fences(text, fix)


def _has_frontmatter(text):
    """True if text opens with a ----delimited block. Raises if never closed."""
    if not re.match(r"---[ \t]*\r?\n", text):
        return False
    if not re.search(r"\n---[ \t]*(\r?\n|$)", text[3:]):
        raise ValueError("frontmatter opens with --- but is never closed")
    return True


def first_h1(text):
    """The first level-1 ATX heading outside fenced code, or None."""
    for i, part in enumerate(_FENCE.split(text)):
        if i % 2:
            continue
        m = re.search(r"^# +(.+?)[ \t]*#*[ \t]*$", part, re.M)
        if m:
            return m.group(1).strip()
    return None


def ensure_title(text, rel):
    """Give a frontmatter-less document a `title` frontmatter from its H1.

    A document that already has frontmatter is returned unchanged: no field is
    added, reordered or normalised. Only `title` is ever synthesised. The title
    is written as a JSON string, which is valid double-quoted YAML, so a colon
    ("CCV: Overview") cannot break the page.
    """
    try:
        if _has_frontmatter(text):
            return text
    except ValueError as e:
        raise ValueError(f"spec/{rel}: {e}") from None
    title = first_h1(text)
    if not title:
        raise ValueError(
            f"spec/{rel}: no frontmatter and no H1 heading; cannot derive a "
            "title. Add frontmatter or a heading to the source.")
    return f"---\ntitle: {json.dumps(title, ensure_ascii=False)}\n---\n\n{text}"


def add_header(text, rel):
    """Insert the generated-file header after the frontmatter block."""
    m = _FRONTMATTER.match(text)
    end = m.end() if m else 0
    return text[:end] + HEADER.format(rel=rel) + "\n" + text[end:].lstrip("\n")


def generate(root, out):
    """Render spec/ into `out`. Returns the written paths, relative to `out`."""
    src = os.path.join(root, "spec")
    written = []
    for dirpath, _, names in os.walk(src):
        for name in sorted(names):
            full = os.path.join(dirpath, name)
            rel = os.path.relpath(full, src)
            if excluded(rel):
                continue
            if name.endswith(".md"):
                dest_rel = rel[:-len(".md")] + ".mdx"
                dest = os.path.join(out, dest_rel)
                os.makedirs(os.path.dirname(dest), exist_ok=True)
                with open(full, encoding="utf-8") as fh:
                    src_text = fh.read()
                check_fences(src_text, rel)
                body = add_header(
                    ensure_title(transform(src_text, rel), rel), rel)
                with open(dest, "w", encoding="utf-8") as fh:
                    fh.write(body)
                written.append(dest_rel)
            elif name.lower().endswith(IMAGE_EXT):
                dest = os.path.join(out, rel)
                os.makedirs(os.path.dirname(dest), exist_ok=True)
                _refuse_lfs_pointer(full, rel)
                shutil.copyfile(full, dest)
                written.append(rel)
    return written



_LFS_POINTER = b"version https://git-lfs.github.com/spec/v1"


def _refuse_lfs_pointer(path, rel):
    """Stop a Git LFS pointer from being published in place of an image.

    `spec/` keeps its images in LFS. A clone without the LFS client leaves a
    132-byte pointer on disk in place of each one, and copying that into
    docs/ibc/spec/ publishes the pointer: the site renders the text, and
    --check still passes because the copy does match its source. The docs
    copies are plain blobs, so the pointer would be committed as one.
    """
    with open(path, "rb") as fh:
        if fh.read(len(_LFS_POINTER)) == _LFS_POINTER:
            raise ValueError(
                f"spec/{rel} is a Git LFS pointer, not an image. Run "
                "`git lfs pull` and generate again: copying the pointer "
                "would publish it as the image.")


def check(root, out):
    """Compare `out` with what generate() would write. Returns problem lines.

    Reports files that are missing, stale, or orphaned (present in `out` but not
    generated: a deleted or renamed spec leaves its old page published).
    """
    import tempfile
    problems = []
    with tempfile.TemporaryDirectory() as tmp:
        expected = set(generate(root, tmp))
        for rel in sorted(expected):
            b = os.path.join(out, rel)
            if not os.path.exists(b):
                problems.append(f"missing: docs/ibc/spec/{rel}")
            elif open(os.path.join(tmp, rel), "rb").read() != open(b, "rb").read():
                problems.append(f"stale: docs/ibc/spec/{rel}")
    for dirpath, _, names in os.walk(out):
        for name in names:
            rel = os.path.relpath(os.path.join(dirpath, name), out)
            if rel not in expected:
                problems.append(f"orphan: docs/ibc/spec/{rel}")
    return sorted(problems)


def write_tree(root, out):
    """Regenerate `out` atomically. Refuses to touch a path it does not expect.

    OUT is derived from this file's location; if the script is moved the
    computed path is wrong, and deleting it would be silent damage. The new tree
    is built beside the old one and swapped in, so a failure partway through
    leaves the existing tree intact.
    """
    allowed = os.path.realpath(os.path.join(root, "docs", "ibc")) + os.sep
    target = os.path.realpath(out)
    if not os.path.isdir(os.path.join(root, "spec")):
        raise SystemExit(f"specgen: no spec/ directory under {root}")
    if not target.startswith(allowed):
        raise SystemExit(f"specgen: refusing to write {target}: "
                         f"not under {allowed}")
    os.makedirs(os.path.dirname(target), exist_ok=True)
    new = target + ".new"
    old = target + ".old"
    # A previous run killed between the two renames leaves the only copy in
    # `old`. Put it back before doing anything else.
    if os.path.isdir(old) and not os.path.exists(target):
        os.rename(old, target)
    shutil.rmtree(new, ignore_errors=True)
    shutil.rmtree(old, ignore_errors=True)
    swapped = False
    try:
        written = generate(root, new)
        had_old = os.path.isdir(target)
        if had_old:
            os.rename(target, old)
        try:
            os.rename(new, target)
        except BaseException:
            if had_old:
                os.rename(old, target)  # restore: never lose the only copy
            raise
        swapped = True
    finally:
        shutil.rmtree(new, ignore_errors=True)
        if swapped:
            shutil.rmtree(old, ignore_errors=True)
    return written


def main(argv):
    if "--check" in argv:
        problems = check(ROOT, OUT)
        if problems:
            print("\n".join(problems))
            print("run: python3 docs/scripts/specgen.py")
            return 1
        print("docs/ibc/spec is up to date with spec/")
        return 0
    written = write_tree(ROOT, OUT)
    print(f"wrote {len(written)} files to docs/ibc/spec/")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
