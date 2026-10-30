#!/usr/bin/env node

/**
 * check-github-refs.js
 *
 * Audits GitHub links in a product's docs and, for links pinned to a
 * version-tracking release branch, bumps them to the branch that is now
 * shipping. Reports everything it will not touch.
 *
 * Usage:
 *   node check-github-refs.js --product ibc-go --targets latest
 *   node check-github-refs.js --product sdk
 *   node check-github-refs.js --product sdk --targets latest,next --fix
 *   node check-github-refs.js --product cometbft --json flags.json
 *
 * Options:
 *   --product <name>    ibc-go | sdk | cometbft (products with a version-tracking ref shape)
 *   --targets <list>    comma-separated version dirs [default: latest,next]
 *   --new-ref <ref>     override the target ref (default: derived from mintlify/versions.json)
 *   --fix               apply the two safe rewrites; never touches a flagged link
 *   --json <path>       write the machine-readable flag report
 *   --limit <n>         only process the first n links (for smoke tests)
 *
 * WHY THIS EXISTS
 *
 * A version-tracking ref does not follow a freeze, so after promoting v0.55 the
 * pages still pointed at release/v0.54.x. Bumping them with sed is wrong: about a
 * quarter of line-anchored links land on different code at the new ref, and the
 * link keeps working, so the error is silent. See the GitHub link section of
 * CLAUDE.md in this directory for the measurements.
 *
 * THE FLAG CONTRACT
 *
 * Every link produces one record. Records with an actionable verdict are written
 * to --json for an agent to adjudicate. The skill that reads this report lives
 * in cosmos/docs-skills, not here: https://github.com/cosmos/docs-skills
 *
 *   {
 *     doc:        "sdk/latest/learn/concepts/store.mdx",  // file containing the link
 *     line:       159,                                    // 1-indexed line in doc
 *     url:        "https://github.com/...",               // the link as written
 *     repo:       "cosmos/cosmos-sdk",
 *     kind:       "blob" | "tree",
 *     refOld:     "release/v0.54.x",
 *     refNew:     "release/v0.55.x",                      // null when not bumpable
 *     path:       "store/tracekv/store.go",               // URL-decoded
 *     anchor:     { start: 36, end: null } | null,
 *     category:   "version-tracking" | "pinned" | "moving" | "other",
 *     verdict:    see VERDICTS below,
 *     newAnchor:  { start, end } | null,                  // when relocated
 *     evidence:   string | null,                          // upstream diff hunk
 *     reason:     string                                  // one-line explanation
 *   }
 */

import fs from 'fs';
import path from 'path';
import { execFileSync } from 'child_process';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
// docs/mintlify/scripts -> docs/
const REPO_ROOT = path.join(__dirname, '..', '..');
const VERSIONS_JSON = path.join(__dirname, '..', 'versions.json');
const CACHE = path.join(__dirname, '.cache', 'refcheck');

/** Verdicts, ordered from "safe to automate" to "needs a human". */
export const VERDICTS = {
  OK: 'ok', // already on the target ref, nothing to do
  BUMP: 'bump', // path exists at new ref, no anchor to invalidate
  RELOCATE: 'relocate', // anchor only shifted; new line number computed
  ANCHOR_CHANGED: 'anchor-changed', // anchored code was edited; prose may be stale
  PATH_GONE: 'path-gone', // renamed or deleted at the new ref
  DEAD_NOW: 'dead-now', // already 404 at its current ref, before any bump
  DRIFT: 'drift', // pinned ref whose anchored code has since changed
  UNMAINTAINABLE: 'unmaintainable', // line anchor on a moving ref
  UNASSESSED: 'unassessed', // could not be checked; not the same as "fine"
  SKIP: 'skip', // deliberately left alone, and confirmed fine
};

const AUTO_FIX = new Set([VERDICTS.BUMP, VERDICTS.RELOCATE]);

/**
 * Pages whose content is ABOUT a specific version must keep that version's refs.
 * A v0.54 upgrade guide citing the v0.54 CHANGELOG is correct; bumping it to
 * v0.55 makes the page contradict itself. Bumping these produced exactly that:
 * "breaking changes in v0.54.0, see the Changelog" pointing at v0.55's changelog,
 * and "the v0.53.x to v0.54.x upgrade reference" pointing at v0.55's UPGRADING.md.
 */
const VERSION_SPECIFIC = [
  /\/upgrade\//, // <product>/<ver>/upgrade/v0.54.mdx and friends (cosmos/docs layout)
  /\/migrations\//, // this repo's name for the same thing: ibc-go/<ver>/migrations/
  /\/changelog\//, // generated release notes
];

export const isVersionSpecific = doc => VERSION_SPECIFIC.some(re => re.test(doc));

// Per-product ref shape. The version-tracking ref is the only one ever bumped.
const PRODUCTS = {
  'ibc-go': {
    repo: 'cosmos/ibc-go',
    // release/v11.0.x
    trackingRe: /^release\/v\d+\.\d+\.x$/,
    refFor: v => `release/v${v.replace(/^v/, '')}`,
  },
  sdk: {
    repo: 'cosmos/cosmos-sdk',
    // release/v0.54.x
    trackingRe: /^release\/v\d+\.\d+\.x$/,
    refFor: v => `release/v${v.replace(/^v/, '')}.x`,
  },
  cometbft: {
    repo: 'cometbft/cometbft',
    // v0.38.x
    trackingRe: /^v\d+\.\d+\.x$/,
    refFor: v => `v${v.replace(/^v/, '')}.x`,
  },
};

// ---------------------------------------------------------------------------
// Pure helpers (exported for the fixture tests)
// ---------------------------------------------------------------------------

/**
 * A two-segment ref (release/v0.55.x) must be tried before a single segment,
 * or "release" is taken as the whole ref and "v0.55.x/..." becomes the path.
 * That miscounted 126 SDK links as an unknown category on the first pass.
 */
const LINK_RE = new RegExp(
  'https://github\\.com/([\\w.-]+/[\\w.-]+)/(blob|tree)/' +
    '(release/v\\d+\\.\\d+\\.x|[^/\\s)"\'\\]]+)' +
    '/([^\\s)"\'\\]#]+)' +
    '(?:#L(\\d+)(?:-L(\\d+))?)?',
  'g'
);

/** TRAP 1: a path is URL-encoded in the link but literal in the git tree. */
export function normalizePath(p) {
  // A bare URL ending a sentence pulls the punctuation into the path. Markdown
  // links are terminated by ")" so this only bites on unwrapped URLs, but a
  // spurious trailing "." would look like a deleted file.
  const trimmed = p.replace(/[.,;:]+$/, '');
  try {
    return decodeURIComponent(trimmed);
  } catch {
    return trimmed; // malformed escape; compare as written
  }
}

export function classifyRef(ref, trackingRe) {
  if (trackingRe.test(ref)) return 'version-tracking';
  if (ref === 'main' || ref === 'master') return 'moving';
  if (/^[0-9a-f]{7,40}$/.test(ref)) return 'pinned';
  if (/^v\d/.test(ref)) return 'pinned';
  return 'other';
}

/**
 * TRAP 2: GitHub redirects /blob/<ref>/<dir> to /tree/, so a "blob" link
 * pointing at a directory resolves fine. Accept either type.
 */
export function treeHas(trees, p) {
  const q = normalizePath(p);
  return trees.blobs.has(q) || trees.dirs.has(q);
}

export function parseHunks(diffText) {
  const hunks = [];
  for (const line of diffText.split('\n')) {
    const m = line.match(/^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/);
    if (m) {
      hunks.push({
        oldStart: +m[1],
        oldCount: m[2] === undefined ? 1 : +m[2],
        newStart: +m[3],
        newCount: m[4] === undefined ? 1 : +m[4],
      });
    }
  }
  return hunks;
}

/**
 * Map an old line number to its new position using diff hunk offsets.
 *
 * Outside every hunk the line only shifted, which is a safe rewrite. Inside a
 * hunk the content itself changed, which is a human decision. This is what
 * content comparison could not distinguish: identical text elsewhere in the file
 * produced six candidate lines for one .proto anchor.
 *
 * @returns {{status:'same'|'moved'|'changed', line:number|null}}
 */
export function mapLine(oldLine, hunks) {
  let offset = 0;
  for (const h of hunks) {
    if (h.oldCount === 0) {
      // Pure insertion. Git writes "@@ -10,0 +11,5 @@" to mean the new lines go
      // AFTER old line 10, so line 10 itself is untouched.
      if (oldLine <= h.oldStart) break;
      offset += h.newCount;
      continue;
    }
    if (oldLine < h.oldStart) break; // no later hunk can affect it
    if (oldLine < h.oldStart + h.oldCount) return { status: 'changed', line: null };
    offset += h.newCount - h.oldCount;
  }
  const mapped = oldLine + offset;
  return { status: offset === 0 ? 'same' : 'moved', line: mapped };
}

/**
 * Minor-version distance between two version-tracking refs, or null if either
 * cannot be parsed.
 *
 * Hunk-offset mapping assumes git's diff alignment is semantic. For a file that
 * has been heavily rewritten, git aligns on textual similarity instead, so a line
 * can sit "outside every hunk" while no longer meaning the same thing. Measured:
 * mapping node/node.go across v0.34.x to v0.40.x returned L684 and L730 where the
 * semantically correct lines were L699 and L764. One minor version is reliable;
 * six is not.
 */
export function minorGap(refA, refB) {
  const pick = r => {
    const m = String(r).match(/v(\d+)\.(\d+)/);
    return m ? [+m[1], +m[2]] : null;
  };
  const a = pick(refA);
  const b = pick(refB);
  if (!a || !b || a[0] !== b[0]) return null;
  return Math.abs(b[1] - a[1]);
}

/** Anchored auto-fixes are only trusted within this many minor versions. */
export const MAX_ANCHOR_GAP = 1;

export function extractLinks(text, docPath) {
  const out = [];
  const lines = text.split('\n');
  lines.forEach((lineText, i) => {
    for (const m of lineText.matchAll(LINK_RE)) {
      const [url, repo, kind, ref, rawPath, l1, l2] = m;
      out.push({
        doc: docPath,
        line: i + 1,
        url,
        repo,
        kind,
        refOld: ref,
        path: normalizePath(rawPath),
        rawPath,
        anchor: l1 ? { start: +l1, end: l2 ? +l2 : null } : null,
      });
    }
  });
  return out;
}

/**
 * Rewrite one document's auto-fixable links. Pure, so the ordering hazard below
 * is covered by fixture tests.
 *
 * @returns {{text:string, count:number, warnings:string[]}}
 */
export function applyRewrites(text, items) {
  const warnings = [];
  let count = 0;
  // Longest URL first. An unanchored link is a string prefix of the anchored
  // link to the same path, so replacing the short one first would rewrite the ref
  // inside the long one; the anchored record would then no longer match and its
  // line number would be stranded at the old value. That is the exact
  // silent-wrong-line failure this tool exists to prevent.
  const ordered = [...items].sort((a, b) => b.url.length - a.url.length);
  for (const l of ordered) {
    let replacement = l.url.replace(`/${l.kind}/${l.refOld}/`, `/${l.kind}/${l.refNew}/`);
    if (l.verdict === VERDICTS.RELOCATE) {
      const oldFrag = `#L${l.anchor.start}${l.anchor.end ? `-L${l.anchor.end}` : ''}`;
      const newFrag = `#L${l.newAnchor.start}${l.newAnchor.end ? `-L${l.newAnchor.end}` : ''}`;
      if (!replacement.endsWith(oldFrag)) {
        warnings.push(`${l.doc}:${l.line} expected to end with ${oldFrag}; left alone`);
        continue;
      }
      replacement = replacement.slice(0, -oldFrag.length) + newFrag;
    }
    // The same URL can appear on several lines and yields one record each; the
    // first record rewrites them all, so count occurrences, not records.
    //
    // Match whole links only. A plain link and a `#L..` anchored link to the
    // same file share a prefix, so splitting on the plain URL also cut the
    // anchored one and moved it to the new branch, leaving its old line number
    // pointing at whatever now sits there -- including for records this pass
    // deliberately flagged rather than fixed.
    const pattern = new RegExp(
      l.url.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + '(?![^\\s)"\'>\\]])',
      'g',
    );
    const occurrences = (text.match(pattern) || []).length;
    if (occurrences > 0) {
      count += occurrences;
      text = text.replace(pattern, () => replacement);
    }
  }
  return { text, count, warnings };
}

// ---------------------------------------------------------------------------
// Network and git
// ---------------------------------------------------------------------------

async function fetchTree(repo, ref) {
  const key = `${repo}@${ref}`.replace(/[^\w.@-]/g, '_');
  const cached = path.join(CACHE, `tree_${key}.json`);
  if (fs.existsSync(cached)) return JSON.parse(fs.readFileSync(cached, 'utf8'));

  const url = `https://api.github.com/repos/${repo}/git/trees/${ref}?recursive=1`;
  const headers = { 'User-Agent': 'cosmos-docs-check-github-refs' };
  if (process.env.GITHUB_TOKEN) headers.Authorization = `Bearer ${process.env.GITHUB_TOKEN}`;
  const res = await fetch(url, { headers });
  if (!res.ok) throw new Error(`tree fetch failed for ${repo}@${ref}: ${res.status}`);
  const data = await res.json();
  if (data.truncated) {
    console.warn(`  ⚠ tree for ${repo}@${ref} is TRUNCATED; path checks may be wrong`);
  }
  fs.mkdirSync(CACHE, { recursive: true });
  fs.writeFileSync(cached, JSON.stringify(data));
  return data;
}

function toSets(treeData) {
  const blobs = new Set();
  const dirs = new Set();
  for (const e of treeData.tree || []) {
    (e.type === 'blob' ? blobs : dirs).add(e.path);
  }
  return { blobs, dirs };
}

/** Bare partial clone: blobs are fetched lazily, so only diffed files download. */
function ensureRepo(repo) {
  const dir = path.join(CACHE, repo.replace('/', '__') + '.git');
  if (!fs.existsSync(dir)) {
    fs.mkdirSync(dir, { recursive: true });
    execFileSync('git', ['init', '-q', '--bare', dir]);
    execFileSync('git', ['-C', dir, 'remote', 'add', 'origin', `https://github.com/${repo}.git`]);
  }
  return dir;
}

function fetchRef(gitDir, ref) {
  // Custom namespace, not refs/heads. An annotated tag resolves to a tag object,
  // and git refuses to store one as a branch head ("invalid new value provided"),
  // which silently failed every tag fetch and left 64 pinned links unassessed.
  const local = `refs/docs/${ref.replace(/[^\w.-]/g, '_')}`;
  try {
    execFileSync('git', ['-C', gitDir, 'rev-parse', '--verify', '-q', local], {
      stdio: 'ignore',
    });
    return local;
  } catch {
    /* not fetched yet */
  }
  // A bare name works for a branch; a tag needs its full refs/tags/ path.
  const errors = [];
  for (const src of [ref, `refs/tags/${ref}`, `refs/heads/${ref}`]) {
    try {
      execFileSync(
        'git',
        ['-C', gitDir, 'fetch', '-q', '--filter=blob:none', '--depth=1', 'origin', `${src}:${local}`],
        { stdio: ['ignore', 'ignore', 'pipe'] }
      );
      execFileSync('git', ['-C', gitDir, 'rev-parse', '--verify', '-q', local], { stdio: 'ignore' });
      return local;
    } catch (e) {
      errors.push(`${src}: ${String(e.stderr || e.message).trim().split('\n')[0]}`);
    }
  }
  throw new Error(`could not fetch ${ref} (tried ${errors.length} refspecs)`);
}

function diffFile(gitDir, refA, refB, filePath) {
  try {
    return execFileSync(
      'git',
      ['-C', gitDir, 'diff', '-U0', '--no-color', refA, refB, '--', filePath],
      { encoding: 'utf8', maxBuffer: 64 * 1024 * 1024 }
    );
  } catch (e) {
    return e.stdout || '';
  }
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

function parseArgs(argv) {
  const cfg = { product: null, targets: ['latest', 'next'], newRef: null, fix: false, json: null, limit: null };
  for (let i = 0; i < argv.length; i++) {
    switch (argv[i]) {
      case '--product': cfg.product = argv[++i]; break;
      case '--targets': cfg.targets = argv[++i].split(','); break;
      case '--new-ref': cfg.newRef = argv[++i]; break;
      case '--fix': cfg.fix = true; break;
      case '--json': cfg.json = argv[++i]; break;
      case '--limit': cfg.limit = +argv[++i]; break;
      case '--help':
        console.log(fs.readFileSync(fileURLToPath(import.meta.url), 'utf8').split('*/')[0]);
        process.exit(0);
    }
  }
  return cfg;
}

function walk(dir, acc = []) {
  if (!fs.existsSync(dir)) return acc;
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name);
    if (e.isDirectory()) walk(p, acc);
    else if (/\.(mdx|md)$/.test(e.name)) acc.push(p);
  }
  return acc;
}

async function main() {
  const cfg = parseArgs(process.argv.slice(2));
  const product = PRODUCTS[cfg.product];
  if (!product) {
    console.error(`Error: --product must be one of ${Object.keys(PRODUCTS).join(', ')}`);
    process.exit(1);
  }

  const versions = JSON.parse(fs.readFileSync(VERSIONS_JSON, 'utf8'));
  const display = versions.products[cfg.product].latestDisplayVersion;
  const newRef = cfg.newRef || product.refFor(display);

  console.log(`check-github-refs: ${cfg.product} (${product.repo})`);
  console.log(`  shipping ref: ${newRef}`);
  console.log(`  targets:      ${cfg.targets.join(', ')}`);

  // 1. extract
  let links = [];
  for (const t of cfg.targets) {
    const root = path.join(REPO_ROOT, cfg.product, t);
    for (const f of walk(root)) {
      const rel = path.relative(REPO_ROOT, f);
      links.push(...extractLinks(fs.readFileSync(f, 'utf8'), rel));
    }
  }
  links = links.filter(l => l.repo === product.repo);
  if (cfg.limit) links = links.slice(0, cfg.limit);
  console.log(`  links:        ${links.length} (${links.filter(l => l.anchor).length} anchored)\n`);

  // 2. classify, and collect the refs we need trees for
  for (const l of links) l.category = classifyRef(l.refOld, product.trackingRe);
  // Every ref gets a tree, including `main` and unrecognized branch names, so
  // that every link is liveness-checked. A path present in the tree is a link
  // that resolves; absent means the link is dead at the ref it carries.
  const needTrees = new Set([newRef, ...links.map(l => l.refOld)]);

  const trees = {};
  for (const ref of needTrees) {
    try {
      trees[ref] = toSets(await fetchTree(product.repo, ref));
      console.log(`  tree ${ref}: ${trees[ref].blobs.size} blobs`);
    } catch (e) {
      console.warn(`  ⚠ ${e.message}`);
      trees[ref] = null;
    }
  }
  console.log('');

  // 3. adjudicate
  const gitDir = ensureRepo(product.repo);
  const localRefs = {};
  const needDiff = l =>
    l.anchor &&
    (l.category === 'version-tracking' || l.category === 'pinned') &&
    l.refOld !== newRef;
  const refsToClone = new Set(links.filter(needDiff).map(l => l.refOld));
  if (refsToClone.size) {
    for (const ref of [newRef, ...refsToClone]) {
      if (localRefs[ref]) continue;
      try {
        localRefs[ref] = fetchRef(gitDir, ref);
      } catch (e) {
        console.warn(`  ⚠ could not fetch ${ref}: ${e.message}`);
        localRefs[ref] = null;
      }
    }
  }

  const diffCache = new Map();
  const getHunks = (refOld, p) => {
    const key = `${refOld}\u0000${p}`;
    if (!diffCache.has(key)) {
      const a = localRefs[refOld];
      const b = localRefs[newRef];
      diffCache.set(key, a && b ? parseHunks(diffFile(gitDir, a, b, p)) : null);
    }
    return diffCache.get(key);
  };

  for (const l of links) {
    l.refNew = null;
    l.newAnchor = null;
    l.evidence = null;

    // Liveness first, for every category. A link whose path is absent at its own
    // ref is dead now, which matters more than which category it belongs to.
    const ownTree = trees[l.refOld];
    if (ownTree && !treeHas(ownTree, l.path)) {
      l.verdict = VERDICTS.DEAD_NOW;
      l.reason = `path absent at its current ref ${l.refOld}; this link 404s today`;
      continue;
    }

    // A page about one version keeps that version's refs, even when superseded.
    if (isVersionSpecific(l.doc)) {
      l.verdict = VERDICTS.SKIP;
      l.reason = `version-specific page; keeps its own version's refs and is never bumped`;
      continue;
    }

    if (l.category === 'other') {
      l.verdict = VERDICTS.SKIP;
      l.reason = `unrecognized ref "${l.refOld}"; needs a look`;
      continue;
    }

    if (l.category === 'moving') {
      if (l.anchor) {
        l.verdict = VERDICTS.UNMAINTAINABLE;
        l.reason = `line anchor on "${l.refOld}", which moves continuously; repoint at a pinned sha or ${newRef}`;
      } else {
        l.verdict = VERDICTS.SKIP;
        l.reason = `"${l.refOld}" tracks current code; no version bump applies`;
      }
      continue;
    }

    if (l.category === 'pinned') {
      if (!l.anchor) {
        l.verdict = VERDICTS.SKIP;
        l.reason = `pinned to ${l.refOld}; deliberate citation, not bumped`;
        continue;
      }
      const newTree = trees[newRef];
      if (newTree && !treeHas(newTree, l.path)) {
        l.verdict = VERDICTS.DRIFT;
        l.reason = `pinned at ${l.refOld}; that path no longer exists at ${newRef}, so surrounding prose may be stale`;
        continue;
      }
      // Line-level drift is only measurable within a small version distance, for
      // the same reason anchored auto-fixes are. A SHA gives no distance at all.
      const pgap = minorGap(l.refOld, newRef);
      if (pgap === null || pgap > MAX_ANCHOR_GAP) {
        l.verdict = VERDICTS.UNASSESSED;
        l.reason =
          `pinned at ${l.refOld}, ${pgap === null ? 'no comparable version distance' : pgap + ' minor versions'} ` +
          `from ${newRef}; line-level drift cannot be measured reliably, so this needs a human read`;
        continue;
      }
      const hunks = getHunks(l.refOld, l.path);
      if (hunks === null) {
        l.verdict = VERDICTS.UNASSESSED;
        l.reason = `pinned at ${l.refOld}; could not diff the file, so drift is unknown`;
      } else {
        const span = [l.anchor.start, ...(l.anchor.end ? [l.anchor.end] : [])];
        const changed = span.some(n => mapLine(n, hunks).status === 'changed');
        if (changed) {
          l.verdict = VERDICTS.DRIFT;
          l.reason = `pinned at ${l.refOld}; the anchored code changed by ${newRef}, so check the prose still holds`;
          l.evidence = diffFile(gitDir, localRefs[l.refOld], localRefs[newRef], l.path).slice(0, 4000);
        } else {
          l.verdict = VERDICTS.SKIP;
          l.reason = `pinned at ${l.refOld}; anchored code unchanged at ${newRef}, prose still matches`;
        }
      }
      continue;
    }

    // version-tracking
    if (l.refOld === newRef) {
      l.verdict = VERDICTS.OK;
      l.reason = 'already on the shipping ref';
      continue;
    }
    l.refNew = newRef;

    const newTree = trees[newRef];
    // A null tree means the fetch failed or the ref does not exist yet. Falling
    // through would reach the BUMP branch below, whose reason claims the path
    // exists at the new ref -- rewriting a working link to one nothing checked.
    if (!newTree) {
      l.verdict = VERDICTS.UNASSESSED;
      l.reason = `could not read the tree at ${newRef}; the path was never checked`;
      continue;
    }
    if (!treeHas(newTree, l.path)) {
      l.verdict = VERDICTS.PATH_GONE;
      l.reason = `path does not exist at ${newRef}; renamed or deleted upstream`;
      continue;
    }

    if (!l.anchor) {
      l.verdict = VERDICTS.BUMP;
      l.reason = `path exists at ${newRef} and there is no anchor to invalidate`;
      continue;
    }

    // Across more than one minor version, git's diff alignment stops being a
    // reliable proxy for "the same line", so no anchored rewrite is trustworthy.
    const gap = minorGap(l.refOld, newRef);
    if (gap === null || gap > MAX_ANCHOR_GAP) {
      l.verdict = VERDICTS.ANCHOR_CHANGED;
      l.reason =
        `anchored link spanning ${gap === null ? 'an unparseable ref pair' : gap + ' minor versions'} ` +
        `(${l.refOld} to ${newRef}); line mapping is not reliable beyond ${MAX_ANCHOR_GAP}, verify by hand`;
      continue;
    }

    const hunks = getHunks(l.refOld, l.path);
    if (hunks === null) {
      l.verdict = VERDICTS.ANCHOR_CHANGED;
      l.reason = 'could not diff the file to verify the anchor';
      continue;
    }
    const startMap = mapLine(l.anchor.start, hunks);
    const endMap = l.anchor.end ? mapLine(l.anchor.end, hunks) : null;
    if (startMap.status === 'changed' || endMap?.status === 'changed') {
      l.verdict = VERDICTS.ANCHOR_CHANGED;
      l.reason = `the anchored lines were edited between ${l.refOld} and ${newRef}`;
      l.evidence = diffFile(gitDir, localRefs[l.refOld], localRefs[newRef], l.path).slice(0, 4000);
    } else if (startMap.status === 'same' && (!endMap || endMap.status === 'same')) {
      l.verdict = VERDICTS.BUMP;
      l.reason = `anchor unaffected between ${l.refOld} and ${newRef}`;
    } else {
      l.verdict = VERDICTS.RELOCATE;
      l.newAnchor = { start: startMap.line, end: endMap ? endMap.line : null };
      l.reason = `anchor shifted from L${l.anchor.start} to L${startMap.line}`;
    }
  }

  // 4. report
  const byVerdict = {};
  for (const l of links) (byVerdict[l.verdict] ||= []).push(l);
  console.log('VERDICTS');
  for (const v of Object.values(VERDICTS)) {
    const n = byVerdict[v]?.length || 0;
    if (n) console.log(`  ${String(n).padStart(4)}  ${v}${AUTO_FIX.has(v) ? '  (auto-fixable)' : ''}`);
  }

  const flags = links.filter(l => !AUTO_FIX.has(l.verdict) && l.verdict !== VERDICTS.OK && l.verdict !== VERDICTS.SKIP);
  if (flags.length) {
    console.log(`\nNEEDS REVIEW (${flags.length})`);
    for (const f of flags) {
      console.log(`  [${f.verdict}] ${f.doc}:${f.line}`);
      console.log(`      ${f.path}${f.anchor ? `#L${f.anchor.start}` : ''}  ${f.refOld}`);
      console.log(`      ${f.reason}`);
    }
  }

  if (cfg.json) {
    const out = links.filter(l => l.verdict !== VERDICTS.OK);
    fs.writeFileSync(cfg.json, JSON.stringify({ product: cfg.product, repo: product.repo, newRef, records: out }, null, 2));
    console.log(`\nwrote ${out.length} records to ${cfg.json}`);
  }

  // 5. fix
  if (cfg.fix) {
    const fixable = links.filter(l => AUTO_FIX.has(l.verdict) && l.refNew);
    const byDoc = {};
    for (const l of fixable) (byDoc[l.doc] ||= []).push(l);
    let count = 0;
    for (const [doc, items] of Object.entries(byDoc)) {
      const abs = path.join(REPO_ROOT, doc);
      const res = applyRewrites(fs.readFileSync(abs, 'utf8'), items);
      for (const w of res.warnings) console.warn(`  ⚠ ${w}`);
      count += res.count;
      fs.writeFileSync(abs, res.text);
    }
    console.log(`\napplied ${count} rewrites across ${Object.keys(byDoc).length} files`);
    console.log('flagged links were not touched');
  } else {
    console.log('\n(report only; pass --fix to apply the auto-fixable verdicts)');
  }
}

const isMain = process.argv[1] && fileURLToPath(import.meta.url) === path.resolve(process.argv[1]);
if (isMain) main();
