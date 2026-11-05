#!/usr/bin/env node

/**
 * verify-links-live.js
 *
 * Requests every GitHub link in the docs and reports its real HTTP status, plus
 * whether each `#Lnnn` anchor actually exists in the file.
 *
 * Usage:
 *   node verify-links-live.js ibc-go/latest
 *   node verify-links-live.js ibc-go --json /tmp/live.json
 *
 * WHY THIS EXISTS
 *
 * `npx mint broken-links` validates internal page paths only. It never requests
 * an external URL, so a dead GitHub link ships silently. check-github-refs.js
 * infers liveness from the git tree, which is cheap and correct about paths but
 * says nothing about whether a line anchor points past end of file: GitHub
 * clamps `#L32` on a 31-line file rather than erroring, so a wrong anchor looks
 * healthy. This makes the request and counts the lines.
 */

import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

// docs/mintlify/scripts -> docs/
const REPO_ROOT = path.join(path.dirname(fileURLToPath(import.meta.url)), '..', '..');
const CONCURRENCY = 8;

// Backtick is excluded from the path charset: a URL deliberately shown as inline
// code would otherwise absorb its own closing backtick into the path.
const LINK_RE = new RegExp(
  'https://github\\.com/([\\w.-]+/[\\w.-]+)/(blob|tree)/' +
    '(release/v\\d+\\.\\d+\\.x|[^/\\s)"\'\\]`]+)' +
    '/([^\\s)"\'\\]`#]+)' +
    '(?:#L(\\d+)(?:-L(\\d+))?)?',
  'g'
);

/**
 * Is this occurrence an actual markdown link, or a URL shown as text?
 *
 * A dead URL that has deliberately been unlinked (shown as plain text or inline
 * code, typically because the upstream repo moved and there is no successor) is
 * not a broken link. Reporting it as one means the check can never go green, so
 * the two cases have to be told apart.
 */
function isLinked(lineText, index) {
  const before = lineText.slice(Math.max(0, index - 2), index);
  return before.endsWith('](') || before.endsWith('(<');
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

function collect(roots) {
  const seen = new Map();
  for (const root of roots) {
    for (const f of walk(path.join(REPO_ROOT, root))) {
      const rel = path.relative(REPO_ROOT, f);
      const text = fs.readFileSync(f, 'utf8');
      text.split('\n').forEach((lineText, i) => {
        for (const m of lineText.matchAll(LINK_RE)) {
          const [url, repo, kind, ref, rawPath, l1, l2] = m;
          const key = url;
          if (!seen.has(key)) {
            seen.set(key, {
              url, repo, kind, ref,
              path: decodeURIComponent(rawPath.replace(/[.,;:]+$/, '')),
              anchor: l1 ? { start: +l1, end: l2 ? +l2 : null } : null,
              sites: [],
              linked: false,
            });
          }
          const rec = seen.get(key);
          rec.sites.push(`${rel}:${i + 1}`);
          // linked if ANY occurrence is a real markdown link
          if (isLinked(lineText, m.index)) rec.linked = true;
        }
      });
    }
  }
  return [...seen.values()];
}

const headers = { 'User-Agent': 'cosmos-docs-verify-links' };
if (process.env.GITHUB_TOKEN) headers.Authorization = `Bearer ${process.env.GITHUB_TOKEN}`;

/** Line count per (repo, ref, path), so anchors can be range-checked. */
const lineCache = new Map();
async function lineCount(repo, ref, p) {
  const key = `${repo}@${ref}/${p}`;
  if (lineCache.has(key)) return lineCache.get(key);
  let n = null;
  try {
    const r = await fetch(`https://raw.githubusercontent.com/${repo}/${ref}/${p}`, { headers });
    if (r.ok) n = (await r.text()).split('\n').length;
  } catch { /* network */ }
  lineCache.set(key, n);
  return n;
}

async function check(link) {
  const out = { ...link, status: null, anchorOk: null, note: '' };
  try {
    const r = await fetch(link.url, { headers, redirect: 'follow' });
    out.status = r.status;
  } catch (e) {
    out.status = 0;
    out.note = e.message;
  }
  if (out.status === 200 && link.anchor && link.kind === 'blob') {
    const n = await lineCount(link.repo, link.ref, link.path);
    if (n === null) out.note = 'could not count lines';
    else {
      const hi = link.anchor.end || link.anchor.start;
      out.anchorOk = hi <= n;
      if (!out.anchorOk) out.note = `anchor L${hi} exceeds ${n} lines`;
    }
  }
  return out;
}

async function main() {
  const args = process.argv.slice(2);
  const jsonAt = args.indexOf('--json');
  const jsonPath = jsonAt >= 0 ? args[jsonAt + 1] : null;
  const roots = args.filter((a, i) => !a.startsWith('--') && i !== jsonAt + 1);
  if (!roots.length) {
    console.error('Usage: node verify-links-live.js <dir> [dir...] [--json out.json]');
    process.exit(1);
  }

  const links = collect(roots);
  console.log(`${links.length} unique GitHub URLs across ${roots.join(', ')}`);
  console.log(`(${links.filter(l => l.anchor).length} carry line anchors)\n`);

  const results = [];
  let done = 0;
  const queue = [...links];
  await Promise.all(
    Array.from({ length: CONCURRENCY }, async () => {
      while (queue.length) {
        const link = queue.shift();
        results.push(await check(link));
        if (++done % 50 === 0) console.log(`  checked ${done}/${links.length}`);
      }
    })
  );

  const allDead = results.filter(r => r.status !== 200);
  const dead = allDead.filter(r => r.linked);
  const deadUnlinked = allDead.filter(r => !r.linked);
  const badAnchor = results.filter(r => r.anchorOk === false && r.linked);
  console.log(`\n${results.length} checked`);
  console.log(`  ${results.length - allDead.length} returned 200`);
  console.log(`  ${dead.length} BROKEN LINKS (markdown links that do not resolve)`);
  console.log(`  ${badAnchor.length} resolve but the anchor is past end of file`);
  console.log(`  ${deadUnlinked.length} dead URLs shown as text, not linked (deliberate, not counted as broken)`);

  for (const r of dead) {
    console.log(`\n[HTTP ${r.status}] ${r.url}`);
    r.sites.forEach(s => console.log(`    ${s}`));
    if (r.note) console.log(`    ${r.note}`);
  }
  for (const r of badAnchor) {
    console.log(`\n[BAD ANCHOR] ${r.url}`);
    console.log(`    ${r.note}`);
    r.sites.forEach(s => console.log(`    ${s}`));
  }

  if (jsonPath) {
    fs.writeFileSync(jsonPath, JSON.stringify(results, null, 2));
    console.log(`\nwrote ${results.length} records to ${jsonPath}`);
  }
  if (deadUnlinked.length) {
    console.log('\nDead URLs shown as text rather than linked (no action needed):');
    for (const r of deadUnlinked) console.log(`  [${r.status}] ${r.url}\n      ${r.sites[0]}`);
  }
  process.exit(dead.length || badAnchor.length ? 1 : 0);
}

main();
