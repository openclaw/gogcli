import assert from "node:assert/strict";
import test from "node:test";

import { headingAnchors } from "./check-docs-coverage.mjs";

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
