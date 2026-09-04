import { test } from 'node:test';
import assert from 'node:assert';
import { commentByteSize, splitCommentIntoChunks } from '../lib/comment-body.js';

const TITLE = '### Sigma Rule Results';
const CHANGED_HEADING = '### Changed Files';
const DELETED_HEADING = '### Deleted Files';
const RESULTS_HEADING = '### Test Results';
const RESULTS_TABLE_HEADER = '| File name | Link | Result count | Execution time | Bytes processed | Errors |';
const RESULTS_TABLE_SEPARATOR = '| --- | --- | --- | --- | --- | --- |';

const changedFile = i => `- [rule_${i}.json](https://github.com/grafana/sigma-internal/blob/my-branch/rules/rule_${i}.json)`;
const resultsRow = i => `| rule_${i}.json | [See in Explore](https://grafana.example/explore?left=abc) | 3 | - | - | 0 |`;
const partLabel = (part, total) => `:open_book: Part ${part} of ${total}`;

/** The value the workflow passes as COMMENT_IDENTIFIER, hidden at the top of every chunk. */
const COMMENT_IDENTIFIER = 'Sigma Rule Conversions';
const IDENTIFIER_MARKER = `<!-- ${COMMENT_IDENTIFIER} -->`;

/** The room splitCommentIntoChunks keeps free in every chunk for its part label and identifier marker. */
const SAFETY_MARGIN = commentByteSize(`${partLabel(100, 100)}\n\n${IDENTIFIER_MARKER}\n`);

/** A comment shaped like the ones buildCommentBody renders, built here to keep the test isolated. */
function buildComment() {
  return buildCommentWithInjectedLine();
}

function buildCommentWithInjectedLine(
  injectedTableLine = "", 
  injectedTableLineIndex = -1, 
  injectedChangedFilesLine = "", 
  injectedChangedFilesLineIndex = -1) {
  return [
    '',
    TITLE,
    '',
    '| Changed | Deleted |',
    '| --- | --- |',
    '| 20 | 1 |',
    '',
    CHANGED_HEADING,
    '',
    ...Array.from({ length: 20 }, (_, i) => i == injectedChangedFilesLineIndex ? injectedChangedFilesLine : changedFile(i)),
    '',
    DELETED_HEADING,
    '',
    '- rules/old_rule.json',
    '',
    RESULTS_HEADING,
    '',
    RESULTS_TABLE_HEADER,
    RESULTS_TABLE_SEPARATOR,
    ...Array.from({ length: 20 }, (_, i) => i == injectedTableLineIndex ? injectedTableLine : resultsRow(i)),
    '',
  ].join('\n');
}

/** Index of `line` in the comment. */
function lineIndex(lines, line) {
  const index = lines.indexOf(line);
  assert(index !== -1, `line not found in the comment: ${line}`);
  return index;
}

/** The byte limit that makes the comment split right before the line at `index`. */
function limitSplittingBefore(lines, index) {
  const bytesThroughLine = lines.slice(0, index + 1).reduce((total, line) => total + commentByteSize(line + '\n'), 0);
  return bytesThroughLine - 1 + SAFETY_MARGIN;
}

/** Chunk content without the blank lines, which markdown ignores anyway. */
const contentLines = chunk => chunk.split('\n').filter(line => line.trim() !== '');

/** Chunk without the identifier marker and part label, to assert on the comment content alone. */
const commentContent = chunk => chunk.split('\n')
  .filter(line => !line.startsWith(':open_book: Part ') && line !== IDENTIFIER_MARKER)
  .join('\n');

test('splitCommentIntoChunks - a split in the middle of a table repeats the table header', () => {
  const comment = buildComment();
  const lines = comment.split('\n');

  const chunks = splitCommentIntoChunks(comment, COMMENT_IDENTIFIER, limitSplittingBefore(lines, lineIndex(lines, resultsRow(2))));

  // The first chunk keeps the table it started ...
  assert(chunks[0].includes(`${RESULTS_TABLE_HEADER}\n${RESULTS_TABLE_SEPARATOR}\n${resultsRow(0)}`));
  // ... and the second one repeats the header above the remaining rows
  assert.deepStrictEqual(
    contentLines(commentContent(chunks[1])).slice(0, 3),
    [RESULTS_TABLE_HEADER, RESULTS_TABLE_SEPARATOR, resultsRow(2)],
  );
});

test('splitCommentIntoChunks - a split right below a table header moves heading and header to the next chunk', () => {
  const comment = buildComment();
  const lines = comment.split('\n');

  const chunks = splitCommentIntoChunks(comment, COMMENT_IDENTIFIER, limitSplittingBefore(lines, lineIndex(lines, resultsRow(0))));

  // An empty table head is worthless, so nothing of it stays behind in the first chunk
  assert(!chunks[0].includes(RESULTS_HEADING));
  assert(!chunks[0].includes(RESULTS_TABLE_HEADER));
  assert(!chunks[0].includes(RESULTS_TABLE_SEPARATOR));
  // Heading and table head lead the second chunk, followed by the first row
  assert.deepStrictEqual(
    contentLines(commentContent(chunks[1])).slice(0, 4),
    [RESULTS_HEADING, RESULTS_TABLE_HEADER, RESULTS_TABLE_SEPARATOR, resultsRow(0)],
  );
});

test('splitCommentIntoChunks - a split right after a heading moves the heading to the next chunk', () => {
  const comment = buildComment();
  const lines = comment.split('\n');
  const heading = lineIndex(lines, DELETED_HEADING);

  // Split before the blank line below the heading
  const chunks = splitCommentIntoChunks(comment, COMMENT_IDENTIFIER, limitSplittingBefore(lines, heading + 1));

  assert(!chunks[0].includes(DELETED_HEADING));
  assert.deepStrictEqual(contentLines(commentContent(chunks[1])).slice(0, 2), [DELETED_HEADING, '- rules/old_rule.json']);
  assert(chunks[1].includes(`${DELETED_HEADING}\n\n`), `heading is not followed by a blank line: ${JSON.stringify(chunks[1])}`);
});

test('splitCommentIntoChunks - a split after the blank line below a heading moves both to the next chunk', () => {
  const comment = buildComment();
  const lines = comment.split('\n');
  const heading = lineIndex(lines, DELETED_HEADING);

  // Split before the first text line below the heading
  const chunks = splitCommentIntoChunks(comment, COMMENT_IDENTIFIER, limitSplittingBefore(lines, heading + 2));

  assert(!chunks[0].includes(DELETED_HEADING));
  assert.deepStrictEqual(contentLines(commentContent(chunks[1])).slice(0, 2), [DELETED_HEADING, '- rules/old_rule.json']);
  assert(chunks[1].includes(`${DELETED_HEADING}\n\n`), `heading is not followed by a blank line: ${JSON.stringify(chunks[1])}`);
});

test('splitCommentIntoChunks - a comment below the limit is returned unsplit', () => {
  const comment = buildComment();

  const chunks = splitCommentIntoChunks(comment, COMMENT_IDENTIFIER, commentByteSize(comment) * 2);

  assert.strictEqual(chunks.length, 1);
  // The identifier line sits flush above the comment, which is otherwise unchanged
  assert(chunks[0].startsWith(`${IDENTIFIER_MARKER}\n${TITLE}\n`), `chunk starts with: ${JSON.stringify(chunks[0].slice(0, 60))}`);
  assert.deepStrictEqual(contentLines(commentContent(chunks[0])), contentLines(comment));
});

test('splitCommentIntoChunks - a comment of exactly the limit is not split, one byte more is', () => {
  const comment = buildComment();
  // The limit has to cover the comment plus the room reserved for a part label
  const size = commentByteSize(comment + '\n') + SAFETY_MARGIN;

  assert.strictEqual(splitCommentIntoChunks(comment, COMMENT_IDENTIFIER, size).length, 1);
  assert.strictEqual(splitCommentIntoChunks(comment, COMMENT_IDENTIFIER, size - 1).length, 2);
});

test('splitCommentIntoChunks - the limit counts bytes, not characters', () => {
  const comment = ['- ééééé', '- ééééé', '- ééééé'].join('\n');
  assert.strictEqual(commentByteSize(comment + '\n'), 39);
  assert.strictEqual((comment + '\n').length, 24);

  // 30 characters would fit the whole comment, 30 bytes do not
  assert(splitCommentIntoChunks(comment, COMMENT_IDENTIFIER, 30 + SAFETY_MARGIN).length > 1);
});

test('splitCommentIntoChunks - an empty comment yields just the identifier line', () => {
  assert.deepStrictEqual(splitCommentIntoChunks('', COMMENT_IDENTIFIER, 100), [`${IDENTIFIER_MARKER}\n`]);
});

test('splitCommentIntoChunks - a table spanning several chunks repeats the header in every one of them', () => {
  const comment = buildComment();

  const chunks = splitCommentIntoChunks(comment, COMMENT_IDENTIFIER, 400);

  const chunksWithRows = chunks.filter(chunk => chunk.includes('| rule_'));
  assert(chunksWithRows.length > 2, `expected the table to span more than two chunks, got ${chunksWithRows.length}`);
  for (const chunk of chunksWithRows) {
    assert(
      chunk.includes(`${RESULTS_TABLE_HEADER}\n${RESULTS_TABLE_SEPARATOR}`),
      `chunk without a table header: ${JSON.stringify(chunk)}`,
    );
  }
});

test('splitCommentIntoChunks - no content is lost when splitting', () => {
  const comment = buildComment();

  const chunks = splitCommentIntoChunks(comment, COMMENT_IDENTIFIER, 400);

  const rejoined = contentLines(chunks.join(''));
  for (const line of contentLines(comment)) {
    assert(rejoined.includes(line), `line missing after splitting: ${line}`);
  }
});

test('splitCommentIntoChunks - no chunk exceeds the limit, part label included', () => {
  const comment = buildComment();

  for (const chunk of splitCommentIntoChunks(comment, COMMENT_IDENTIFIER, 400)) {
    assert(commentByteSize(chunk) <= 400, `chunk of ${commentByteSize(chunk)} bytes exceeds the limit`);
  }
});

test('splitCommentIntoChunks - text below a table does not inherit the table header', () => {
  const comment = [
    RESULTS_HEADING,
    '',
    RESULTS_TABLE_HEADER,
    RESULTS_TABLE_SEPARATOR,
    resultsRow(0),
    '',
    DELETED_HEADING,
    '',
    ...Array.from({ length: 6 }, (_, i) => `- rules/old_rule_${i}.json`),
  ].join('\n');

  const chunks = splitCommentIntoChunks(comment, COMMENT_IDENTIFIER, 300);

  assert(chunks.length > 1);
  for (const chunk of chunks.filter(chunk => !chunk.includes('| rule_'))) {
    assert(!chunk.includes(RESULTS_TABLE_SEPARATOR), `table header carried over into plain text: ${JSON.stringify(chunk)}`);
  }
});

test('splitCommentIntoChunks - a single line above the limit ends up alone in its chunk', () => {
  const longRow = resultsRow(0);
  const comment = ['- rules/old_rule.json', longRow, '- rules/another_rule.json'].join('\n');
  assert(commentByteSize(longRow + '\n') > 40);

  const chunks = splitCommentIntoChunks(comment, COMMENT_IDENTIFIER, 40);

  // The line cannot be split any further, so it is passed through instead of being dropped
  const chunkWithLongRow = chunks.find(chunk => chunk.includes(longRow));
  assert.deepStrictEqual(contentLines(commentContent(chunkWithLongRow)), [longRow]);
  assert(chunks.join('').includes('- rules/another_rule.json'));
});

test('splitCommentIntoChunks - a comment that is not split carries no part label', () => {
  const comment = buildComment();

  const chunks = splitCommentIntoChunks(comment, COMMENT_IDENTIFIER, commentByteSize(comment) * 2);

  assert.strictEqual(chunks.length, 1);
  assert(!chunks[0].includes(':open_book: Part '), 'a single chunk should not be numbered');
});

test('splitCommentIntoChunks - every chunk is numbered with its part and the total', () => {
  const comment = buildComment();

  const chunks = splitCommentIntoChunks(comment, COMMENT_IDENTIFIER, 400);
  const total = chunks.length;

  assert(total > 1);
  // The first chunk is numbered at the end, every following one at the top
  assert(chunks[0].endsWith(partLabel(1, total)), `first chunk ends with: ${JSON.stringify(chunks[0].slice(-40))}`);
  for (let i = 1; i < total; i++) {
    assert(
      chunks[i].startsWith(`${IDENTIFIER_MARKER}\n${partLabel(i + 1, total)}\n\n`),
      `chunk ${i} starts with: ${JSON.stringify(chunks[i].slice(0, 40))}`,
    );
  }
  // Exactly one label per chunk
  for (const chunk of chunks) {
    assert.strictEqual(chunk.split('\n').filter(line => line.startsWith(':open_book: Part ')).length, 1);
  }
});

test('splitCommentIntoChunks - no false positive table header separators are detected', () => {

  const dangerousLine = '| Looking for credentials | [See in Explore](https://example.grafana.example/explore?something=---BEGIN RSA PRIVATE KEY---) | 0 | 5.230158 s | 342,526,137 decbytes | 0 |'; // Has the triple dash which might trip up table header separator detection

  const comment = buildCommentWithInjectedLine(
    dangerousLine,
    5,
  );
  const chunks = splitCommentIntoChunks(comment, COMMENT_IDENTIFIER, 400);
  assert(chunks.length > 1);

  const chunksWithDangerousLine = chunks.filter(chunk => chunk.includes(dangerousLine));
  assert.strictEqual(chunksWithDangerousLine.length, 1, 'the dangerous line should appear in exactly one chunk');

  const chunksWithRows = chunks.filter(chunk => chunk.includes('| rule_') || chunk.includes(dangerousLine));
  assert(chunksWithRows.length > 1, `expected the table to span more than one chunk, got ${chunksWithRows.length}`);
  for (const chunk of chunksWithRows) {
    assert(
      chunk.includes(`${RESULTS_TABLE_HEADER}\n${RESULTS_TABLE_SEPARATOR}`),
      `chunk without a table header: ${JSON.stringify(chunk)}`,
    );
  }
});
