#!/usr/bin/env node

/**
 * tag-archived.js
 *
 * Retroactively injects `noindex: true` and `canonical` front matter into
 * existing archived (versioned) documentation directories.
 *
 * This is a one-time or per-product maintenance tool. Run it whenever:
 *   - A product's existing versioned directories (e.g. sdk/v0.50, sdk/v0.47)
 *     need to be marked as non-indexable by search engines.
 *   - You want to point archived pages at the canonical /latest/ equivalent.
 *
 * Usage:
 *   node tag-archived.js --product sdk --version v0.50
 *   node tag-archived.js --product sdk --all
 *   node tag-archived.js --product ibc --version v8.5.x
 *   node tag-archived.js --all-products --all
 *
 * Options:
 *   --product <name>    Product name (sdk, ibc, evm, cometbft, hub, etc.)
 *   --version <ver>     Single version directory to tag (e.g. v0.53)
 *   --all               Tag all non-latest, non-next versioned dirs for the product
 *   --all-products      Run across every product discovered in the repo
 *   --dry-run           Print what would change without modifying files
 *   --base-url <url>    Prefix for canonical URLs (default: empty, i.e. site-relative)
 *
 * Differences from the cosmos/docs original:
 *   - Paths resolve against docs/ in this monorepo (this script lives in
 *     docs/mintlify/scripts/).
 *   - Canonicals are site-relative (/ibc-go/latest/<page>). This site has no
 *     deployed domain yet, so an absolute URL would bake in a guess.
 *   - A page that already carries noindex is no longer skipped blindly: if its
 *     canonical is missing or absolute (e.g. it still points at the old
 *     docs.cosmos.network site) it is restamped. A site-relative canonical is
 *     left alone, so a hand-chosen target survives a re-run.
 *   - A page with no counterpart in latest/ falls back to the nearest surviving
 *     page (section overview/index, walking up), not to the bare /latest/ root,
 *     which is not a page on this site.
 */

import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
// docs/mintlify/scripts -> docs/
const REPO_ROOT = path.join(__dirname, '..', '..');
const BASE_URL = '';

// ---------------------------------------------------------------------------
// CLI argument parsing
// ---------------------------------------------------------------------------

function parseArgs(argv) {
  const args = { product: null, version: null, all: false, allProducts: false, dryRun: false, baseUrl: BASE_URL };
  for (let i = 0; i < argv.length; i++) {
    switch (argv[i]) {
      case '--product':     args.product     = argv[++i]; break;
      case '--version':     args.version     = argv[++i]; break;
      case '--all':         args.all         = true;      break;
      case '--all-products':args.allProducts = true;      break;
      case '--dry-run':     args.dryRun      = true;      break;
      case '--base-url':    args.baseUrl     = argv[++i]; break;
    }
  }
  return args;
}

// ---------------------------------------------------------------------------
// Output helpers
// ---------------------------------------------------------------------------

const colors = { red: '\x1b[31m', green: '\x1b[32m', yellow: '\x1b[33m', blue: '\x1b[34m', reset: '\x1b[0m' };
function info(msg)    { console.log(`${colors.blue}ℹ${colors.reset}  ${msg}`); }
function success(msg) { console.log(`${colors.green}✓${colors.reset}  ${msg}`); }
function warn(msg)    { console.log(`${colors.yellow}⚠${colors.reset}  ${msg}`); }
function skip(msg)    { console.log(`   ${msg}`); }

// ---------------------------------------------------------------------------
// Product / version discovery
// ---------------------------------------------------------------------------

function discoverProducts() {
  const ignore = new Set(['node_modules', 'scripts', 'snippets', 'assets', 'mintlify', '.git']);
  return fs.readdirSync(REPO_ROOT, { withFileTypes: true })
    .filter(d => d.isDirectory() && !d.name.startsWith('.') && !ignore.has(d.name))
    .map(d => d.name)
    .filter(name => {
      const entries = fs.readdirSync(path.join(REPO_ROOT, name), { withFileTypes: true });
      return entries.some(d => d.isDirectory() && /^v\d+/.test(d.name));
    });
}

function discoverArchivedVersions(product) {
  const productDir = path.join(REPO_ROOT, product);
  if (!fs.existsSync(productDir)) return [];
  return fs.readdirSync(productDir, { withFileTypes: true })
    .filter(d => d.isDirectory() && /^v\d+/.test(d.name))
    .map(d => d.name);
}

// ---------------------------------------------------------------------------
// Front matter injection
// ---------------------------------------------------------------------------

const FRONTMATTER_RE = /^---\r?\n([\s\S]*?)\r?\n---/;

/**
 * Returns the relative page path within a product+version dir.
 * e.g. sdk/v0.50/learn/concepts/accounts.mdx → learn/concepts/accounts
 */
function relativePagePath(filePath, productDir) {
  const rel = path.relative(productDir, filePath);
  return rel.replace(/\.mdx$/, '');
}

const SECTION_LANDING = ['overview', 'index', 'README'];

/**
 * Nearest surviving page for a page that has no counterpart in latest/:
 * the landing page (overview/index/README) of the closest ancestor section
 * that has one, else the product's intro. Returns a page path without .mdx.
 */
function nearestSurvivor(latestDir, pageRelativeNoExt) {
  const parts = pageRelativeNoExt.split('/').slice(0, -1);
  while (true) {
    const dir = parts.join('/');
    for (const name of SECTION_LANDING) {
      const candidate = dir ? `${dir}/${name}` : name;
      if (fs.existsSync(path.join(latestDir, `${candidate}.mdx`))) return candidate;
    }
    if (parts.length === 0) break;
    parts.pop();
  }
  return fs.existsSync(path.join(latestDir, 'intro.mdx')) ? 'intro' : null;
}

/**
 * Determines the canonical URL for an archived page.
 *
 * Prefers the page's own equivalent in latest/; otherwise the nearest
 * surviving page. `fallback` is set when the second case applies.
 */
function canonicalUrl(filePath, product, baseUrl) {
  const latestDir = path.join(REPO_ROOT, product, 'latest');
  // parts: ['ibc-go', 'v10.1.x', 'light-clients', 'proposals.mdx']
  const parts = path.relative(REPO_ROOT, filePath).split(path.sep);
  const noExt = parts.slice(2).join('/').replace(/\.mdx$/, '');

  if (fs.existsSync(path.join(latestDir, `${noExt}.mdx`))) {
    return { url: `${baseUrl}/${product}/latest/${noExt}`, fallback: null };
  }
  const target = nearestSurvivor(latestDir, noExt);
  return {
    url: `${baseUrl}/${product}/latest/${target || ''}`,
    fallback: target,
  };
}

/**
 * Injects `noindex: true` and `canonical: <url>` into MDX front matter.
 * Skips files that already have noindex set.
 * Returns true if the file was (or would be) modified.
 */
const fallbacks = [];

function injectFrontMatter(filePath, product, baseUrl, dryRun) {
  const raw = fs.readFileSync(filePath, 'utf8');
  const match = raw.match(FRONTMATTER_RE);
  const rel = path.relative(REPO_ROOT, filePath);
  const { url, fallback } = canonicalUrl(filePath, product, baseUrl);
  const note = () => { if (fallback !== null) fallbacks.push({ page: rel, target: url }); };

  if (match) {
    const fm = match[1];
    if (/^noindex\s*:/m.test(fm)) {
      // Already tagged. Restamp only a canonical that is missing or absolute.
      const cm = fm.match(/^canonical\s*:\s*'?([^'\n]*)'?\s*$/m);
      if (cm && !/^[a-z]+:\/\//i.test(cm[1])) return false;
      note();
      const newFm = cm
        ? fm.replace(/^canonical\s*:.*$/m, `canonical: '${url}'`)
        : `canonical: '${url}'\n${fm}`;
      if (!dryRun) fs.writeFileSync(filePath, raw.replace(FRONTMATTER_RE, () => `---\n${newFm}\n---`), 'utf8');
      return true;
    }
    note();
    const newFm = `noindex: true\ncanonical: '${url}'\n${fm}`;
    if (!dryRun) fs.writeFileSync(filePath, raw.replace(FRONTMATTER_RE, () => `---\n${newFm}\n---`), 'utf8');
    return true;
  } else {
    // No front matter — prepend one
    note();
    const newContent = `---\nnoindex: true\ncanonical: '${url}'\n---\n\n${raw}`;
    if (!dryRun) fs.writeFileSync(filePath, newContent, 'utf8');
    return true;
  }
}

// ---------------------------------------------------------------------------
// Walk and tag a version directory
// ---------------------------------------------------------------------------

function walkMdx(dir, results = []) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) walkMdx(full, results);
    else if (entry.name.endsWith('.mdx')) results.push(full);
  }
  return results;
}

function tagVersion(product, version, baseUrl, dryRun) {
  const versionDir = path.join(REPO_ROOT, product, version);
  if (!fs.existsSync(versionDir)) {
    warn(`Directory not found: ${product}/${version} — skipping`);
    return { tagged: 0, skipped: 0 };
  }

  const files = walkMdx(versionDir);
  let tagged = 0, skipped = 0;

  for (const file of files) {
    const modified = injectFrontMatter(file, product, baseUrl, dryRun);
    if (modified) {
      tagged++;
      const rel = path.relative(REPO_ROOT, file);
      if (dryRun) skip(`[dry-run] would tag: ${rel}`);
    } else {
      skipped++;
    }
  }
  return { tagged, skipped };
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

async function main() {
  const args = parseArgs(process.argv.slice(2));

  if (!args.product && !args.allProducts) {
    console.error('Error: --product <name> or --all-products is required');
    console.error('Run with --help for usage.');
    process.exit(1);
  }

  if (!args.version && !args.all) {
    console.error('Error: --version <ver> or --all is required');
    process.exit(1);
  }

  if (args.dryRun) info('Dry-run mode — no files will be modified.\n');

  const products = args.allProducts ? discoverProducts() : [args.product];
  const baseUrl  = args.baseUrl;

  let totalTagged = 0, totalSkipped = 0;

  for (const product of products) {
    const productDir = path.join(REPO_ROOT, product);
    if (!fs.existsSync(productDir)) {
      warn(`Product directory not found: ${product} — skipping`);
      continue;
    }

    const versions = args.all ? discoverArchivedVersions(product) : [args.version];
    if (versions.length === 0) {
      warn(`No archived versions found for ${product}`);
      continue;
    }

    info(`Product: ${product}  |  versions to tag: ${versions.join(', ')}`);

    for (const version of versions) {
      const { tagged, skipped } = tagVersion(product, version, baseUrl, args.dryRun);
      const label = args.dryRun ? '[dry-run] ' : '';
      success(`${label}${product}/${version}: tagged ${tagged} files, skipped ${skipped} already-tagged`);
      totalTagged  += tagged;
      totalSkipped += skipped;
    }
  }

  console.log('');
  if (fallbacks.length) {
    warn(`${fallbacks.length} page(s) have no counterpart in latest/; canonical set to the nearest surviving page:`);
    for (const f of fallbacks) skip(`${f.page} -> ${f.target}`);
  }
  info(`Total: ${totalTagged} files tagged, ${totalSkipped} already had noindex.`);
  if (args.dryRun) warn('Re-run without --dry-run to apply changes.');
}

main().catch(err => { console.error(err); process.exit(1); });
