#!/usr/bin/env node
// SPDX-License-Identifier: Apache-2.0

/**
 * Fixture tests for tag-archived.js
 *
 * Run: node tag-archived.test.js
 *
 * Covers where the injected keys land in the front matter. The SPDX comment
 * must stay on its first line: `make lint-license` only looks for the header
 * within the first 80 characters, and noindex plus a canonical push it past
 * that.
 */

import assert from 'assert';
import { prependKeys } from './tag-archived.js';

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

const KEYS = "noindex: true\ncanonical: '/ibc-go/latest/intro'\n";

t('keeps a leading SPDX comment first', () => {
  assert.strictEqual(
    prependKeys('# SPDX-License-Identifier: CC-BY-4.0\ntitle: Intro', KEYS),
    "# SPDX-License-Identifier: CC-BY-4.0\nnoindex: true\ncanonical: '/ibc-go/latest/intro'\ntitle: Intro",
  );
});

t('prepends when there is no SPDX comment', () => {
  assert.strictEqual(prependKeys('title: Intro', KEYS), `${KEYS}title: Intro`);
});

t('handles front matter that is only the SPDX comment', () => {
  assert.strictEqual(
    prependKeys('# SPDX-License-Identifier: CC-BY-4.0', KEYS),
    `# SPDX-License-Identifier: CC-BY-4.0\n${KEYS}`,
  );
});

t('keeps CRLF front matter intact after the SPDX line', () => {
  assert.strictEqual(
    prependKeys('# SPDX-License-Identifier: CC-BY-4.0\r\ntitle: Intro', KEYS),
    `# SPDX-License-Identifier: CC-BY-4.0\n${KEYS}title: Intro`,
  );
});

t('ignores an SPDX comment that is not the first line', () => {
  const fm = 'title: Intro\n# SPDX-License-Identifier: CC-BY-4.0';
  assert.strictEqual(prependKeys(fm, KEYS), KEYS + fm);
});

console.log(`${pass} passed, ${failures.length} failed`);
if (failures.length) {
  for (const f of failures) console.error(`  ${f}`);
  process.exit(1);
}
