#!/usr/bin/env node

/**
 * Fixture tests for check-github-refs.js
 *
 * Run: node check-github-refs.test.js
 *
 * Covers the parsing and mapping logic only, which is where the real bugs were.
 * Network and git behavior is not mocked; those are exercised by running the
 * checker in report mode.
 *
 * These live here rather than in a top-level tests/ directory because
 * .gitignore ignores `tests/*` and they need to be committed.
 */

import assert from 'assert';
import {
  normalizePath,
  classifyRef,
  treeHas,
  parseHunks,
  mapLine,
  extractLinks,
  applyRewrites,
  minorGap,
  isVersionSpecific,
  MAX_ANCHOR_GAP,
  VERDICTS,
} from './check-github-refs.js';

let pass = 0;
const failures = [];
function t(name, fn) {
  try {
    fn();
    pass++;
  } catch (e) {
    failures.push(`${name}: ${e.message}`);
  }
}

const SDK_TRACKING = /^release\/v\d+\.\d+\.x$/;
const CMT_TRACKING = /^v\d+\.\d+\.x$/;

// --- TRAP 1: URL-encoded paths -------------------------------------------
// abci%2B%2B_methods.md is abci++_methods.md in the tree. Comparing the raw
// string reported a live CometBFT link as broken.

t('normalizePath decodes %2B', () => {
  assert.strictEqual(
    normalizePath('spec/abci/abci%2B%2B_methods.md'),
    'spec/abci/abci++_methods.md'
  );
});

t('normalizePath leaves a plain path alone', () => {
  assert.strictEqual(normalizePath('store/iavl/store.go'), 'store/iavl/store.go');
});

t('normalizePath survives a malformed escape', () => {
  assert.strictEqual(normalizePath('a/b%zz.md'), 'a/b%zz.md');
});

t('treeHas matches an encoded path against the literal tree entry', () => {
  const trees = { blobs: new Set(['spec/abci/abci++_methods.md']), dirs: new Set() };
  assert.ok(treeHas(trees, 'spec/abci/abci%2B%2B_methods.md'));
});

// --- TRAP 2: /blob/ pointing at a directory ------------------------------
// GitHub redirects /blob/<ref>/<dir> to /tree/, returning 200. Requiring a blob
// match reported four live CometBFT spec links as broken.

t('treeHas accepts a directory for a blob-style link', () => {
  const trees = { blobs: new Set(), dirs: new Set(['spec/light-client/accountability']) };
  assert.ok(treeHas(trees, 'spec/light-client/accountability'));
});

t('treeHas rejects a path in neither set', () => {
  const trees = { blobs: new Set(['a/b.go']), dirs: new Set(['a']) };
  assert.ok(!treeHas(trees, 'store/tracekv/store.go'));
});

// --- Ref classification --------------------------------------------------

t('two-segment release ref is version-tracking, not "other"', () => {
  assert.strictEqual(classifyRef('release/v0.55.x', SDK_TRACKING), 'version-tracking');
});

t('cometbft bare version ref is version-tracking', () => {
  assert.strictEqual(classifyRef('v0.40.x', CMT_TRACKING), 'version-tracking');
});

t('main and master are moving', () => {
  assert.strictEqual(classifyRef('main', SDK_TRACKING), 'moving');
  assert.strictEqual(classifyRef('master', SDK_TRACKING), 'moving');
});

t('a 40-char sha is pinned', () => {
  assert.strictEqual(
    classifyRef('2bec9d2021918650d3938c3ab242f84289daef80', SDK_TRACKING),
    'pinned'
  );
});

t('a release-candidate tag is pinned, not version-tracking', () => {
  assert.strictEqual(classifyRef('v0.47.0-rc1', SDK_TRACKING), 'pinned');
});

t('a branch name like cosmovisor is "other"', () => {
  assert.strictEqual(classifyRef('cosmovisor', SDK_TRACKING), 'other');
});

// --- Link extraction -----------------------------------------------------

t('extracts a two-segment ref without eating the version into the path', () => {
  const [l] = extractLinks(
    'see [keys](https://github.com/cosmos/cosmos-sdk/blob/release/v0.54.x/x/bank/types/keys.go)',
    'd.mdx'
  );
  assert.strictEqual(l.refOld, 'release/v0.54.x');
  assert.strictEqual(l.path, 'x/bank/types/keys.go');
  assert.strictEqual(l.anchor, null);
});

t('extracts a single-line anchor', () => {
  const [l] = extractLinks(
    '[s](https://github.com/cosmos/cosmos-sdk/blob/release/v0.54.x/store/iavl/store.go#L36)',
    'd.mdx'
  );
  assert.deepStrictEqual(l.anchor, { start: 36, end: null });
});

t('extracts a line range anchor', () => {
  const [l] = extractLinks(
    '[p](https://github.com/cosmos/cosmos-sdk/blob/v0.47.0-rc1/proto/x/tx.proto#L29-L41)',
    'd.mdx'
  );
  assert.deepStrictEqual(l.anchor, { start: 29, end: 41 });
});

t('records the 1-indexed line the link sits on', () => {
  const links = extractLinks(
    'intro\n\n[a](https://github.com/cosmos/cosmos-sdk/blob/main/go.mod)\n',
    'd.mdx'
  );
  assert.strictEqual(links[0].line, 3);
});

t('does not swallow a trailing markdown paren or bracket', () => {
  const [l] = extractLinks(
    '[a](https://github.com/cosmos/cosmos-sdk/tree/main/store) and text',
    'd.mdx'
  );
  assert.strictEqual(l.path, 'store');
  assert.strictEqual(l.kind, 'tree');
});

t('finds multiple links on one line', () => {
  const links = extractLinks(
    '[a](https://github.com/cosmos/cosmos-sdk/blob/main/a.go) [b](https://github.com/cosmos/cosmos-sdk/blob/main/b.go)',
    'd.mdx'
  );
  assert.strictEqual(links.length, 2);
});

// --- Hunk parsing and line mapping --------------------------------------

t('parseHunks reads counts, defaulting an omitted count to 1', () => {
  const h = parseHunks('@@ -10 +10 @@\n@@ -20,3 +20,5 @@\n');
  assert.deepStrictEqual(h[0], { oldStart: 10, oldCount: 1, newStart: 10, newCount: 1 });
  assert.deepStrictEqual(h[1], { oldStart: 20, oldCount: 3, newStart: 20, newCount: 5 });
});

t('a line before every hunk is unchanged', () => {
  const h = parseHunks('@@ -50,2 +50,4 @@');
  assert.deepStrictEqual(mapLine(10, h), { status: 'same', line: 10 });
});

t('a line after a net-negative hunk shifts up', () => {
  // 3 old lines became 1: everything after moves up by 2
  const h = parseHunks('@@ -10,3 +10,1 @@');
  assert.deepStrictEqual(mapLine(50, h), { status: 'moved', line: 48 });
});

t('a line after a net-positive hunk shifts down', () => {
  const h = parseHunks('@@ -10,1 +10,4 @@');
  assert.deepStrictEqual(mapLine(50, h), { status: 'moved', line: 53 });
});

t('a line inside a changed hunk is "changed", never guessed', () => {
  const h = parseHunks('@@ -10,3 +10,3 @@');
  assert.strictEqual(mapLine(11, h).status, 'changed');
  assert.strictEqual(mapLine(11, h).line, null);
});

t('a pure insertion (oldCount 0) cannot swallow a line', () => {
  const h = parseHunks('@@ -10,0 +11,5 @@');
  assert.strictEqual(mapLine(10, h).status, 'same');
  assert.deepStrictEqual(mapLine(11, h), { status: 'moved', line: 16 });
});

t('offsets accumulate across several hunks', () => {
  const h = parseHunks('@@ -10,5 +10,2 @@\n@@ -30,1 +27,3 @@\n');
  // -3 then +2 => net -1 for a line after both
  assert.deepStrictEqual(mapLine(100, h), { status: 'moved', line: 99 });
});

t('an equal-size rewrite still reports changed, not same', () => {
  // net offset is 0, so a naive implementation would call this "same"
  const h = parseHunks('@@ -10,2 +10,2 @@');
  assert.strictEqual(mapLine(10, h).status, 'changed');
});

// --- Regression: the real cases we already know the answers to -----------

t('iavl#L36 case: no hunks means the anchor holds', () => {
  assert.deepStrictEqual(mapLine(36, parseHunks('')), { status: 'same', line: 36 });
});

t('root.go#L50 case: anchor shifted to 47 via a net -3 hunk above it', () => {
  const h = parseHunks('@@ -20,5 +20,2 @@');
  assert.deepStrictEqual(mapLine(50, h), { status: 'moved', line: 47 });
});

// --- Version gap guard ---------------------------------------------------
// Hunk mapping is only semantic for small deltas. Mapping node/node.go across
// v0.34.x to v0.40.x returned L684 and L730 where the right lines were L699 and
// L764, so anchored auto-fixes are refused beyond MAX_ANCHOR_GAP.

t('minorGap measures adjacent versions as 1', () => {
  assert.strictEqual(minorGap('release/v0.54.x', 'release/v0.55.x'), 1);
  assert.strictEqual(minorGap('v0.39.x', 'v0.40.x'), 1);
});

t('minorGap measures a wide gap', () => {
  assert.strictEqual(minorGap('v0.34.x', 'v0.40.x'), 6);
  assert.strictEqual(minorGap('release/v0.50.x', 'release/v0.55.x'), 5);
});

t('minorGap is 0 for the same ref', () => {
  assert.strictEqual(minorGap('v0.40.x', 'v0.40.x'), 0);
});

t('minorGap refuses across a major version', () => {
  assert.strictEqual(minorGap('v0.40.x', 'v1.0.x'), null);
});

t('minorGap is null when a ref has no version', () => {
  assert.strictEqual(minorGap('main', 'v0.40.x'), null);
  assert.strictEqual(minorGap('2bec9d2021918650d3938c3ab242f84289daef80', 'v0.40.x'), null);
});

t('MAX_ANCHOR_GAP admits one minor version and no more', () => {
  assert.strictEqual(MAX_ANCHOR_GAP, 1);
  assert.ok(minorGap('release/v0.54.x', 'release/v0.55.x') <= MAX_ANCHOR_GAP);
  assert.ok(minorGap('release/v0.50.x', 'release/v0.55.x') > MAX_ANCHOR_GAP);
});

// --- Rewrite application -------------------------------------------------
// The prefix hazard: an unanchored link is a substring of the anchored link to
// the same path. store.mdx has exactly this shape.

const BASE = 'https://github.com/cosmos/cosmos-sdk/blob/release/v0.54.x/store/iavl/store.go';
const rec = (over = {}) => ({
  doc: 'd.mdx', line: 1, kind: 'blob',
  refOld: 'release/v0.54.x', refNew: 'release/v0.55.x',
  url: BASE, anchor: null, newAnchor: null, verdict: VERDICTS.BUMP, ...over,
});

t('bumps a plain link', () => {
  const r = applyRewrites(`see [s](${BASE})`, [rec()]);
  assert.ok(r.text.includes('release/v0.55.x/store/iavl/store.go'));
  assert.strictEqual(r.count, 1);
});

t('prefix collision: short link does not strand the anchored one', () => {
  const anchored = `${BASE}#L36`;
  const text = `[a](${BASE}) and [b](${anchored})`;
  const r = applyRewrites(text, [
    rec(),
    rec({ url: anchored, anchor: { start: 36, end: null }, newAnchor: { start: 40, end: null }, verdict: VERDICTS.RELOCATE }),
  ]);
  // the anchored link must have BOTH a new ref and the relocated line
  assert.ok(r.text.includes('release/v0.55.x/store/iavl/store.go#L40'), 'anchor not relocated');
  assert.ok(!r.text.includes('#L36'), 'old line number survived');
  // and the plain link must still be bumped
  assert.ok(r.text.includes(`[a](https://github.com/cosmos/cosmos-sdk/blob/release/v0.55.x/store/iavl/store.go)`));
  assert.ok(!r.text.includes('v0.54.x'), 'an old ref survived');
});

t('same URL twice counts occurrences, not records', () => {
  const text = `[a](${BASE}) then again [b](${BASE})`;
  const r = applyRewrites(text, [rec(), rec({ line: 2 })]);
  assert.strictEqual(r.count, 2);
  assert.ok(!r.text.includes('v0.54.x'));
});

t('relocates a line range', () => {
  const url = `${BASE}#L29-L41`;
  const r = applyRewrites(`[p](${url})`, [
    rec({ url, anchor: { start: 29, end: 41 }, newAnchor: { start: 26, end: 38 }, verdict: VERDICTS.RELOCATE }),
  ]);
  assert.ok(r.text.includes('#L26-L38'));
});

t('warns and skips when the anchor is not where expected', () => {
  const r = applyRewrites(`[p](${BASE})`, [
    rec({ anchor: { start: 99, end: null }, newAnchor: { start: 1, end: null }, verdict: VERDICTS.RELOCATE }),
  ]);
  assert.strictEqual(r.count, 0);
  assert.strictEqual(r.warnings.length, 1);
  assert.ok(r.text.includes('v0.54.x'), 'should have been left alone');
});

t('leaves an unrelated link untouched', () => {
  const other = 'https://github.com/cosmos/cosmos-sdk/blob/main/go.mod';
  const r = applyRewrites(`[a](${BASE}) [b](${other})`, [rec()]);
  assert.ok(r.text.includes(other));
});

t('isVersionSpecific protects migration guides and not ordinary pages', () => {
  assert.strictEqual(isVersionSpecific('docs/ibc-go/latest/migrations/v7-to-v8.mdx'), true);
  assert.strictEqual(isVersionSpecific('docs/ibc-go/v10.1.x/migrations/v9-to-v10.mdx'), true);
  assert.strictEqual(isVersionSpecific('docs/ibc-go/latest/apps/transfer/overview.mdx'), false);
});

console.log(`\n${pass} passed, ${failures.length} failed`);
for (const f of failures) console.log(`  FAIL  ${f}`);
process.exit(failures.length ? 1 : 0);
