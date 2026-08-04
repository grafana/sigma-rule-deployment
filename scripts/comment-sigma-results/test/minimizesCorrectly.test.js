import { test } from "node:test";
import assert from "node:assert";
import {
  buildCommentBody,
  splitCommentIntoChunks,
} from "../lib/comment-body.js";

/** The value actions/convert/action.yml passes as COMMENT_IDENTIFIER (and COMMENT_TITLE). */
const COMMENT_IDENTIFIER = "Sigma Rule Conversions";

function buildComment() {
  return buildCommentBody({
    commentTitle: COMMENT_IDENTIFIER,
    changedFiles: Array.from({ length: 8 }, (_, i) => `rules/rule_${i}.json`),
    deletedFiles: ["rules/old_rule.json"],
    testResults: null,
    conversionErrors: null,
    repoUrl: "https://github.com/grafana/sigma-internal",
    headRef: "my-branch",
  });
}

test("a comment posted in one part is matched by the minimize filter", () => {
  const comment = buildComment();

  const chunks = splitCommentIntoChunks(comment, COMMENT_IDENTIFIER);

  assert.strictEqual(chunks.length, 1);
  assert(chunks[0].startsWith(`<!-- ${COMMENT_IDENTIFIER} -->`));
});

test("every part of a split comment is matched by the minimize filter", () => {
  const comment = buildComment();

  const chunks = splitCommentIntoChunks(comment, COMMENT_IDENTIFIER, 300);

  assert(chunks.length > 1, "expected the comment to split into several parts");
  // comment.js only minimizes old comments whose bodyText starts with the
  // identifier, so a part it cannot match is left expanded on every re-run.
  chunks.forEach((chunk, i) => {
    assert(
      chunk.startsWith(`<!-- ${COMMENT_IDENTIFIER} -->`),
      `part ${i + 1} of ${chunks.length} would not be minimized, it starts with: ${JSON.stringify(chunk.split("\n")[0])}`,
    );
  });
});
