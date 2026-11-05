#!/usr/bin/env node
// Three drift conditions, all of which ship silently: a page that exists but
// is unreachable from the sidebar, a nav entry pointing at a file that was
// renamed or removed, and a page listed in navigation more than once.
//
// This is also the only check that a newly generated spec page under
// docs/ibc/spec/ was added to docs.json: specgen.py --check verifies page
// content, not navigation.
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const DOCS = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "..");
const config = JSON.parse(fs.readFileSync(path.join(DOCS, "docs.json"), "utf8"));

// Collect every page string in the navigation, at any depth. A "pages" array
// may mix strings with nested group objects, which themselves carry "pages".
// Returns an array (not a Set) so duplicates stay visible.
function collectPages(node, out = []) {
  if (Array.isArray(node)) {
    node.forEach((n) => collectPages(n, out));
  } else if (node && typeof node === "object") {
    for (const [key, value] of Object.entries(node)) {
      if (key === "pages" && Array.isArray(value)) {
        value.forEach((p) => (typeof p === "string" ? out.push(p) : collectPages(p, out)));
      } else {
        collectPages(value, out);
      }
    }
  }
  return out;
}

function walk(dir, base) {
  if (!fs.existsSync(dir)) return [];
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap((e) => {
    const full = path.join(dir, e.name);
    if (e.isDirectory()) return walk(full, base);
    if (!e.name.endsWith(".mdx")) return [];
    return [path.relative(base, full).split(path.sep).join("/").replace(/\.mdx$/, "")];
  });
}

const navList = collectPages(config.navigation);
const inNav = new Set(navList);
const roots = ["ibc", "ibc-go"];
// Root-level pages too (index.mdx is the site's landing page). Anything at the
// root that is not a page -- README.md, AGENTS.md -- is listed in .mintignore
// and never served, so it is not a page this check should expect in the nav.
const ignored = new Set(
  fs.existsSync(path.join(DOCS, ".mintignore"))
    ? fs
        .readFileSync(path.join(DOCS, ".mintignore"), "utf8")
        .split("\n")
        .map((l) => l.trim())
        .filter((l) => l && !l.startsWith("#"))
        .map((l) => l.replace(/\.mdx?$/, ""))
    : [],
);
const rootPages = fs
  .readdirSync(DOCS, { withFileTypes: true })
  .filter((e) => e.isFile() && e.name.endsWith(".mdx"))
  .map((e) => e.name.replace(/\.mdx$/, ""))
  .filter((n) => !ignored.has(n));
const onDisk = [...rootPages, ...roots.flatMap((d) => walk(path.join(DOCS, d), DOCS))];

const orphans = onDisk.filter((p) => !inNav.has(p));
const dangling = [...inNav].filter((p) => !fs.existsSync(path.join(DOCS, p + ".mdx")));
const seen = new Set();
const duplicates = [...new Set(navList.filter((p) => (seen.has(p) ? true : (seen.add(p), false))))];

if (onDisk.length === 0) {
  console.error(`no pages found under ${roots.join(", ")}`);
  process.exit(1);
}

const sections = [
  [orphans, "page(s) not in navigation"],
  [dangling, "navigation entr(ies) with no file"],
  [duplicates, "page(s) listed in navigation more than once"],
];
if (sections.some(([list]) => list.length)) {
  for (const [list, label] of sections) {
    if (!list.length) continue;
    console.error(`${list.length} ${label}:`);
    list.forEach((p) => console.error(`  ${p}`));
  }
  process.exit(1);
}

const counts = [["ibc", "ibc/"], ["ibc-go/latest", "ibc-go/latest/"], ["ibc-go/v10.1.x", "ibc-go/v10.1.x/"]]
  .map(([name, prefix]) => `${name}: ${onDisk.filter((p) => p.startsWith(prefix)).length}`)
  .join(", ");
console.log(`navigation and files agree (${onDisk.length} pages; ${counts})`);
