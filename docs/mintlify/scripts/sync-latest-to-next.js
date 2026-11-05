#!/usr/bin/env node

/**
 * sync-latest-to-next.js
 *
 * Copies one or more files from a product's latest/ directory to the
 * equivalent path in next/, rewriting internal links along the way.
 *
 * Usage:
 *   node scripts/sync-latest-to-next.js <file|dir> [file2|dir2 ...]
 *
 * Examples:
 *   node scripts/sync-latest-to-next.js sdk/latest/learn/concepts/baseapp.mdx
 *   node scripts/sync-latest-to-next.js sdk/latest/
 *   node scripts/sync-latest-to-next.js sdk/latest/ hub/latest/overview.mdx
 *
 * The script rewrites /sdk/latest/ → /sdk/next/ in link text (preserving
 * external https:// URLs). Paths must be relative to the repo root.
 */

import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = path.join(__dirname, '..');

const PRODUCTS = ['evm', 'sdk', 'hub', 'cometbft', 'ibc', 'skip-go', 'enterprise'];

// Paths, relative to <product>/latest/, where next/ is the canonical copy and
// syncing into it destroys content. Each refusal fired for real once.
const REFUSED = [
  {
    prefix: 'tutorials/example/',
    why: 'a bot syncs these from cosmos/example into next/, so next/ is canonical',
    instead: 'node scripts/sync-next-to-latest.js <path>',
  },
  {
    prefix: 'api-reference/grpc/',
    why: 'generated per version from a different upstream commit, so latest/ content under next/\'s version stamp would document the wrong version',
    instead: 'cd scripts/api-reference && npm run sync -- --version next',
  },
  {
    prefix: 'api-reference/rest/',
    why: 'generated per version from a different upstream commit',
    instead: 'cd scripts/api-reference && npm run sync -- --version next',
  },
];

function refusalFor(subPath) {
  return REFUSED.find((entry) => subPath.startsWith(entry.prefix));
}

function usage() {
  console.error('Usage: node scripts/sync-latest-to-next.js <file> [file2 ...]');
  console.error('  Files must be paths relative to the repo root, e.g.:');
  console.error('  sdk/latest/learn/concepts/baseapp.mdx');
  process.exit(1);
}

function extractFrontMatter(content) {
  const match = content.match(/^---\n[\s\S]*?\n---\n/);
  return match ? match[0] : '';
}

function extractBody(content) {
  const match = content.match(/^---\n[\s\S]*?\n---\n([\s\S]*)$/);
  return match ? match[1] : content;
}

function rewriteLinks(content, product) {
  // Only rewrite /<product>/latest/ → /<product>/next/ for the product being synced.
  // Cross-product links (e.g. /cometbft/latest/) are left unchanged — next/ files
  // intentionally reference other products' latest/ versions.
  const escapedProduct = product.replace(/-/g, '\\-');
  const re = new RegExp(`(https?:\\/\\/\\S+)|\\/${escapedProduct}\\/latest\\/`, 'g');
  return content.replace(re, (match, externalUrl) => {
    if (externalUrl) return externalUrl;
    return `/${product}/next/`;
  });
}

function syncFile(relPath) {
  // Normalise: strip leading ./
  relPath = relPath.replace(/^\.\//, '');

  // Validate the path contains /latest/
  const latestMatch = relPath.match(/^([^/]+)\/latest\/(.+)$/);
  if (!latestMatch) {
    console.error(`✗ ${relPath}`);
    console.error('  Path must be under a product\'s latest/ directory.');
    return false;
  }

  const [, product, subPath] = latestMatch;

  if (!PRODUCTS.includes(product)) {
    console.error(`✗ ${relPath}`);
    console.error(`  Unknown product "${product}". Expected one of: ${PRODUCTS.join(', ')}`);
    return false;
  }

  const refusal = refusalFor(subPath);
  if (refusal) {
    console.error(`✗ ${relPath}`);
    console.error(`  Refusing: this script runs the wrong way for these files.`);
    console.error(`  ${refusal.why}.`);
    console.error(`  Copying latest/ over next/ here reverts whatever next/ has that latest/ does not.`);
    console.error(`  Instead: ${refusal.instead}`);
    return false;
  }

  const srcPath = path.join(REPO_ROOT, relPath);
  const destPath = path.join(REPO_ROOT, product, 'next', subPath);

  if (!fs.existsSync(srcPath)) {
    console.error(`✗ ${relPath}`);
    console.error(`  File not found: ${srcPath}`);
    return false;
  }

  const srcContent = fs.readFileSync(srcPath, 'utf8');
  const srcBody = extractBody(srcContent);
  const rewrittenBody = rewriteLinks(srcBody, product);

  let rewritten;
  if (fs.existsSync(destPath)) {
    // Keep the destination's front matter exactly as-is (preserves noindex, canonical,
    // and their positions). Only the body content is synced from latest/.
    const destContent = fs.readFileSync(destPath, 'utf8');
    const destFrontMatter = extractFrontMatter(destContent);
    rewritten = destFrontMatter + rewrittenBody;
  } else {
    // New file — use source front matter with links rewritten
    rewritten = rewriteLinks(srcContent, product);
  }

  const destDir = path.dirname(destPath);
  if (!fs.existsSync(destDir)) {
    fs.mkdirSync(destDir, { recursive: true });
    console.log(`  Created directory: ${path.relative(REPO_ROOT, destDir)}`);
  }

  const destExists = fs.existsSync(destPath);
  fs.writeFileSync(destPath, rewritten, 'utf8');

  const destRelPath = path.relative(REPO_ROOT, destPath);
  console.log(`✓ ${relPath} → ${destRelPath} ${destExists ? '(updated)' : '(created)'}`);
  return true;
}

function collectFiles(argPath) {
  const absPath = path.isAbsolute(argPath)
    ? argPath
    : path.join(REPO_ROOT, argPath);

  if (!fs.existsSync(absPath)) {
    console.error(`✗ Not found: ${argPath}`);
    return [];
  }

  const stat = fs.statSync(absPath);
  if (stat.isFile()) {
    return [argPath.replace(/^\.\//, '')];
  }

  if (stat.isDirectory()) {
    const files = [];
    function walk(dir) {
      for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
        const full = path.join(dir, entry.name);
        if (entry.isDirectory()) {
          walk(full);
        } else if (entry.name.endsWith('.mdx')) {
          files.push(path.relative(REPO_ROOT, full));
        }
      }
    }
    walk(absPath);
    return files;
  }

  return [];
}

// --- main ---

const args = process.argv.slice(2);
if (args.length === 0) usage();

const allFiles = args.flatMap(collectFiles);

let ok = 0;
let fail = 0;

for (const f of allFiles) {
  if (syncFile(f)) ok++; else fail++;
}

console.log('');
if (fail === 0) {
  console.log(`Done. ${ok} file(s) synced to next/. Review the diff before committing.`);
} else {
  console.log(`Done. ${ok} succeeded, ${fail} failed.`);
  process.exit(1);
}
