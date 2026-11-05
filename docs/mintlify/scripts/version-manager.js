#!/usr/bin/env node

/**
 * Documentation Version Manager
 *
 * Freezes the current latest/ of a product as an archived version:
 *
 *   1. Copy <product>/latest/ to <product>/<version>/ and rewrite internal links
 *   2. Stamp the copy with noindex and a site-relative canonical (tag-archived.js)
 *   3. Add the archive to docs.json navigation and to mintlify/versions.json
 *
 * Ported from cosmos/docs. That site froze next/ -> latest/ -> archive. This
 * site has no next/ tier: latest/ is the working tree, so the freeze is
 * latest/ -> vX.Y.x/ plus stamping plus registry update, and latest/ is left
 * untouched. (sync-latest-to-next.js is carried unused in case a next/ tier is
 * stood up later.) The release-notes fetch and changelog generation steps were
 * dropped with manage-changelogs.js, which was not ported.
 *
 * Usage:
 *   npm run freeze              # interactive
 *   NON_INTERACTIVE=1 SUBDIR=ibc-go ARCHIVE_VERSION=v11.0.x NEW_DISPLAY_VERSION=v12.0.x npm run freeze
 *
 * ARCHIVE_VERSION defaults to the product's latestDisplayVersion in versions.json.
 */

import fs from 'fs';
import path from 'path';
import readline from 'readline';
import { fileURLToPath } from 'url';
import { execSync, execFileSync } from 'child_process';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
// docs/mintlify/scripts -> docs/
const DOCS_ROOT = path.join(__dirname, '..', '..');
const VERSIONS_JSON = path.join(__dirname, '..', 'versions.json');
const DOCS_JSON = path.join(DOCS_ROOT, 'docs.json');

// The docs.json dropdown label for each product directory.
const DROPDOWN_LABELS = { 'ibc-go': 'IBC-Go' };

// ---------------------------------------------------------------------------
// Output helpers
// ---------------------------------------------------------------------------

const colors = { red: '\x1b[31m', green: '\x1b[32m', yellow: '\x1b[33m', blue: '\x1b[34m', reset: '\x1b[0m' };

function printInfo(msg)    { console.log(`${colors.blue}ℹ${colors.reset} ${msg}`); }
function printSuccess(msg) { console.log(`${colors.green}✓${colors.reset} ${msg}`); }
function printWarning(msg) { console.log(`${colors.yellow}⚠${colors.reset} ${msg}`); }
function printError(msg)   { console.log(`${colors.red}✗${colors.reset} ${msg}`); }

async function prompt(question) {
  const rl = readline.createInterface({ input: process.stdin, output: process.stdout });
  return new Promise(resolve => rl.question(question, answer => { rl.close(); resolve(answer); }));
}

// ---------------------------------------------------------------------------
// Product / version discovery
// ---------------------------------------------------------------------------

function listDocsSubdirs() {
  if (!fs.existsSync(DOCS_ROOT)) return [];
  const ignore = new Set(['node_modules', 'scripts', 'snippets', 'assets', 'mintlify']);
  return fs.readdirSync(DOCS_ROOT, { withFileTypes: true })
    .filter(d => d.isDirectory() && !d.name.startsWith('.') && !ignore.has(d.name))
    .map(d => d.name)
    .filter(name => {
      const contents = fs.readdirSync(path.join(DOCS_ROOT, name), { withFileTypes: true });
      return contents.some(d => d.isDirectory() && (d.name === 'latest' || /^v\d+/.test(d.name)));
    });
}

// ---------------------------------------------------------------------------
// Versions registry (mintlify/versions.json)
// ---------------------------------------------------------------------------

function loadVersionsRegistry() {
  let data = {};
  if (fs.existsSync(VERSIONS_JSON)) {
    try { data = JSON.parse(fs.readFileSync(VERSIONS_JSON, 'utf8')); } catch { data = {}; }
  }
  if (!data.products || typeof data.products !== 'object') data = { products: {} };

  // Auto-discover products and versions from the filesystem
  for (const subdir of listDocsSubdirs()) {
    const base = path.join(DOCS_ROOT, subdir);
    const entries = fs.readdirSync(base, { withFileTypes: true })
      .filter(d => d.isDirectory()).map(d => d.name);

    const discovered = [];
    if (entries.includes('latest')) discovered.push('latest');
    for (const name of entries) {
      if (validateVersionFormat(name)) discovered.push(name);
    }

    if (!data.products[subdir]) {
      data.products[subdir] = {
        versions: discovered,
        defaultVersion: 'latest',
        repository: `cosmos/${subdir}`,
        changelogPath: 'CHANGELOG.md'
      };
    } else {
      const existing = data.products[subdir].versions || [];
      const merged = Array.from(new Set([...discovered, ...existing]));
      const stable = merged.filter(validateVersionFormat).sort(compareVersionsDesc);
      const special = ['latest'].filter(s => merged.includes(s));
      data.products[subdir].versions = [...special, ...stable];
      if (!data.products[subdir].repository)    data.products[subdir].repository    = `cosmos/${subdir}`;
      if (!data.products[subdir].changelogPath) data.products[subdir].changelogPath = 'CHANGELOG.md';
    }
  }

  return data;
}

function saveVersionsRegistry(registry) {
  fs.writeFileSync(VERSIONS_JSON, JSON.stringify(registry, null, 2) + '\n');
}

function validateVersionFormat(version) {
  return /^v\d+\.\d+(?:\.(?:\d+|x))?$/.test(version);
}

function parseVersionTuple(v) {
  const m = v.match(/^v(\d+)\.(\d+)(?:\.(\d+|x))?$/);
  if (!m) return null;
  const patch = m[3] === undefined ? 0 : (m[3] === 'x' ? Number.POSITIVE_INFINITY : parseInt(m[3], 10));
  return [parseInt(m[1], 10), parseInt(m[2], 10), patch, m[3] === 'x'];
}

function compareVersionsDesc(a, b) {
  const at = parseVersionTuple(a) || [0, 0, -1, false];
  const bt = parseVersionTuple(b) || [0, 0, -1, false];
  if (bt[0] !== at[0]) return bt[0] - at[0];
  if (bt[1] !== at[1]) return bt[1] - at[1];
  if (bt[2] !== at[2]) return bt[2] - at[2];
  return at[3] ? 1 : -1;
}

// ---------------------------------------------------------------------------
// Link rewriting (URL-aware — preserves external https:// links)
// ---------------------------------------------------------------------------

/**
 * Rewrites internal doc links in all MDX files under dirPath.
 * Uses perl alternation: matches a full external URL first (preserves it),
 * or the internal path pattern (replaces it).
 * GitHub links with version strings in their paths are never modified.
 */
function rewriteInternalLinks(dirPath, fromSlug, toSlug) {
  const perlCmd = `find "${dirPath}" -name "*.mdx" -type f -exec perl -i'' -pe 's{(https?://\\S+)|/${fromSlug}/}{defined($1)?$1:"/${toSlug}/"}ge' {} \\;`;
  execSync(perlCmd);
}

// ---------------------------------------------------------------------------
// Archive: latest/ → <version>/
// ---------------------------------------------------------------------------

function archiveLatest(archiveVersion, subdir) {
  const latestPath = path.join(DOCS_ROOT, subdir, 'latest');
  const targetPath = path.join(DOCS_ROOT, subdir, archiveVersion);

  printInfo(`Archiving ${subdir}/latest/ → ${subdir}/${archiveVersion}/...`);
  execSync(`rm -rf "${targetPath}" && mkdir -p "${targetPath}"`);
  execSync(`cp -R "${latestPath}/." "${targetPath}/"`);

  rewriteInternalLinks(targetPath, `${subdir}/latest`, `${subdir}/${archiveVersion}`);
  printSuccess(`Archived latest/ → ${archiveVersion}/`);
}

/**
 * Stamp the archive with noindex and a site-relative canonical by delegating to
 * tag-archived.js, so a freeze and a manual re-stamp cannot disagree.
 */
function stampArchive(archiveVersion, subdir) {
  execFileSync('node', [
    path.join(__dirname, 'tag-archived.js'), '--product', subdir, '--version', archiveVersion,
  ], { stdio: 'inherit' });
}

// ---------------------------------------------------------------------------
// Navigation (docs.json)
// ---------------------------------------------------------------------------

/**
 * Deep-clones obj, replacing string values that start with fromPrefix.
 */
function cloneWithPathRewrite(obj, fromPrefix, toPrefix) {
  if (typeof obj === 'string') return obj.startsWith(fromPrefix) ? toPrefix + obj.slice(fromPrefix.length) : obj;
  if (Array.isArray(obj)) return obj.map(x => cloneWithPathRewrite(x, fromPrefix, toPrefix));
  if (obj && typeof obj === 'object') {
    const out = {};
    for (const k of Object.keys(obj)) out[k] = cloneWithPathRewrite(obj[k], fromPrefix, toPrefix);
    return out;
  }
  return obj;
}

/**
 * Adds the archive to the product's dropdown in docs.json.
 *
 * Clones the `latest` navigation entry with paths rewritten to the archive,
 * replacing any existing entry for that version. `latest` stays first (it is
 * the default) and archives follow newest-first.
 */
/**
 * The navigation container holding a product's `versions` array.
 *
 * This site puts IBC-Go under `navigation.tabs` with a `tab` key. Mintlify's
 * own docs show `navigation.dropdowns`, and cosmos/docs uses that shape, so
 * both are accepted: looking only for dropdowns made every freeze throw after
 * the archive had already been copied and stamped.
 */
function findVersionedNav(docsJson, label) {
  const nav = docsJson.navigation || {};
  const candidates = [
    ...(nav.tabs || []).filter(t => t.tab === label),
    ...(nav.dropdowns || []).filter(d => d.dropdown === label),
  ];
  return candidates.find(c => Array.isArray(c.versions)) || null;
}

function updateNavigation(subdir, archiveVersion) {
  const docsJson = JSON.parse(fs.readFileSync(DOCS_JSON, 'utf8'));

  const dropdownLabel = DROPDOWN_LABELS[subdir] || subdir.toUpperCase();
  const dropdown = findVersionedNav(docsJson, dropdownLabel);
  if (!dropdown) {
    throw new Error(`No "${dropdownLabel}" tab or dropdown with a versions array found in docs.json.`);
  }

  const latestEntry = dropdown.versions.find(v => v.version === 'latest');
  if (!latestEntry) throw new Error(`No 'latest' nav entry found for ${dropdownLabel}.`);

  const archiveEntry = cloneWithPathRewrite(latestEntry, `${subdir}/latest/`, `${subdir}/${archiveVersion}/`);
  archiveEntry.version = archiveVersion;
  delete archiveEntry.tag;
  delete archiveEntry.hidden;

  const archived = dropdown.versions
    .filter(v => v.version !== 'latest' && v.version !== archiveVersion)
    .concat(archiveEntry)
    .sort((a, b) => compareVersionsDesc(a.version, b.version));
  dropdown.versions = [latestEntry, ...archived];

  fs.writeFileSync(DOCS_JSON, JSON.stringify(docsJson, null, 2) + '\n');
  printSuccess(`Navigation updated: ${archiveVersion} added to ${dropdownLabel}`);
}

// ---------------------------------------------------------------------------
// Versions registry update
// ---------------------------------------------------------------------------

function updateVersionsRegistry({ subdir, archiveVersion, newDisplayVersion }) {
  const data = loadVersionsRegistry();
  if (!data.products[subdir]) data.products[subdir] = { versions: [], defaultVersion: 'latest' };
  const product = data.products[subdir];

  if (archiveVersion && !product.versions.includes(archiveVersion)) {
    product.versions.push(archiveVersion);
  }
  if (fs.existsSync(path.join(DOCS_ROOT, subdir, 'latest')) && !product.versions.includes('latest')) {
    product.versions.unshift('latest');
  }

  // Sort: latest first, then stable newest-first
  const stable  = product.versions.filter(validateVersionFormat).sort(compareVersionsDesc);
  const special = ['latest'].filter(s => product.versions.includes(s));
  product.versions = [...special, ...stable];

  product.defaultVersion = 'latest';
  if (newDisplayVersion) product.latestDisplayVersion = newDisplayVersion;

  saveVersionsRegistry(data);
  printSuccess(`versions.json updated for ${subdir} (latestDisplayVersion: ${newDisplayVersion})`);
}

// ---------------------------------------------------------------------------
// Version metadata
// ---------------------------------------------------------------------------

function createVersionMetadata(archiveVersion, subdir, newDisplayVersion) {
  const metadataPath = path.join(DOCS_ROOT, subdir, archiveVersion, '.version-metadata.json');
  const frozenPath   = path.join(DOCS_ROOT, subdir, archiveVersion, '.version-frozen');
  const metadata = {
    version: archiveVersion,
    frozenDate: new Date().toISOString().split('T')[0],
    frozenTimestamp: new Date().toISOString(),
    nextDisplayVersion: newDisplayVersion
  };
  fs.writeFileSync(metadataPath, JSON.stringify(metadata, null, 2));
  fs.writeFileSync(frozenPath, `${archiveVersion} - Frozen on ${metadata.frozenDate}`);
  printSuccess('Version metadata created');
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

async function main() {
  const interactive = !['1', 'true', 'yes'].includes(String(process.env.NON_INTERACTIVE || '').toLowerCase());

  // 1. Subdir
  const choices = listDocsSubdirs();
  let subdir;
  if (interactive) {
    subdir = (await prompt(`Enter the docs subdirectory to freeze [${choices.join(', ')}]: `)).trim();
  } else {
    subdir = process.env.DOCS_SUBDIR || process.env.SUBDIR;
  }
  if (!subdir) { printError('No subdirectory provided'); process.exit(1); }

  const latestPath = path.join(DOCS_ROOT, subdir, 'latest');
  if (!fs.existsSync(latestPath)) {
    printError(`${subdir}/latest/ does not exist. There is nothing to freeze.`);
    process.exit(1);
  }

  const registry = loadVersionsRegistry();
  const product = registry.products && registry.products[subdir];
  const storedDisplayVersion = product && product.latestDisplayVersion;

  console.log('\n' + '='.repeat(50));
  console.log('   Documentation Version Manager');
  console.log('='.repeat(50));
  console.log(`\n Subdir:  ${colors.blue}${subdir}${colors.reset}`);
  if (storedDisplayVersion) console.log(` Current display version: ${storedDisplayVersion}`);

  // 2. Archive version (what to call the outgoing latest/)
  let archiveVersion;
  if (interactive) {
    const suggested = storedDisplayVersion ? ` [default: ${storedDisplayVersion}]` : '';
    const input = (await prompt(`Archive version label (what to call the frozen latest/)${suggested}: `)).trim();
    archiveVersion = input || storedDisplayVersion || '';
  } else {
    archiveVersion = process.env.ARCHIVE_VERSION || storedDisplayVersion;
  }
  if (!archiveVersion || !validateVersionFormat(archiveVersion)) {
    printError(`Invalid archive version: ${archiveVersion}`); process.exit(1);
  }

  // 3. Display version that latest/ carries from now on (e.g. "v12.0.x")
  let newDisplayVersion;
  if (interactive) {
    newDisplayVersion = (await prompt('New display version for latest/ (e.g. v12.0.x): ')).trim();
  } else {
    newDisplayVersion = process.env.NEW_DISPLAY_VERSION;
  }
  if (!newDisplayVersion || !validateVersionFormat(newDisplayVersion)) {
    printError(`Invalid display version: ${newDisplayVersion}`); process.exit(1);
  }

  console.log(`\n Freezing: ${colors.yellow}${archiveVersion}${colors.reset} → archive`);
  console.log(` Latest label: ${colors.green}${newDisplayVersion}${colors.reset}\n`);

  // Fail before writing anything. Everything below mutates the tree, and a
  // nav lookup that misses would otherwise leave a copied, stamped archive
  // with no navigation entry behind it.
  const navLabel = DROPDOWN_LABELS[subdir] || subdir.toUpperCase();
  if (!findVersionedNav(JSON.parse(fs.readFileSync(DOCS_JSON, 'utf8')), navLabel)) {
    printError(`No "${navLabel}" tab or dropdown with a versions array in docs.json; nothing was changed.`);
    process.exit(1);
  }

  try {
    archiveLatest(archiveVersion, subdir);

    printInfo('Injecting noindex and canonical into archived version...');
    stampArchive(archiveVersion, subdir);

    updateNavigation(subdir, archiveVersion);
    updateVersionsRegistry({ subdir, archiveVersion, newDisplayVersion });
    createVersionMetadata(archiveVersion, subdir, newDisplayVersion);

    console.log('\n' + '='.repeat(50));
    console.log(' Version freeze completed!');
    console.log('='.repeat(50));
    console.log(`   ✓ ${archiveVersion} archived at ${subdir}/${archiveVersion}/ (noindex injected)`);
    console.log(`   ✓ Navigation and registry updated`);
    console.log(`\n   latest/ is unchanged and continues as the working tree.\n`);

  } catch (error) {
    printError(`Version freeze failed: ${error.message}`);
    process.exit(1);
  }
}

main();
