import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { checkMarkdownLinks, headingAnchors } from "./check-docs-coverage.mjs";

test("headingAnchors ignores headings inside fenced code blocks", () => {
  const anchors = headingAnchors(`# Real Heading

\`\`\`md
# Not A Heading
## Duplicate
\`\`\`

## Duplicate
## Duplicate

~~~text
# Also Not A Heading
~~~
`);

  assert.equal(anchors.has("not-a-heading"), false);
  assert.equal(anchors.has("also-not-a-heading"), false);
  assert.deepEqual([...anchors], ["real-heading", "duplicate", "duplicate-1"]);
});

test("headingAnchors follows GitHub-style heading slugs", () => {
  const anchors = headingAnchors(`# What's new?
## Привет non-latin 你好
## A  B
## foo
## foo
## foo-1
## Heading ##
`);

  assert.deepEqual([...anchors], [
    "whats-new",
    "привет-non-latin-你好",
    "a--b",
    "foo",
    "foo-1",
    "foo-1-1",
    "heading",
  ]);
});

test("checkMarkdownLinks accepts encoded Unicode anchors", (t) => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "gog-doc-links-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));

  fs.writeFileSync(path.join(dir, "target.md"), "# Привет мир\n");
  fs.writeFileSync(
    path.join(dir, "index.md"),
    [
      "[valid](target.md#%D0%BF%D1%80%D0%B8%D0%B2%D0%B5%D1%82-%D0%BC%D0%B8%D1%80)",
      "[broken](target.md#missing)",
      "",
    ].join("\n"),
  );

  const broken = checkMarkdownLinks(dir);
  assert.equal(broken.length, 1);
  assert.match(broken[0], /target\.md#missing$/);
});
