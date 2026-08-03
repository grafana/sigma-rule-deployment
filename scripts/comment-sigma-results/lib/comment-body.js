import { extractTitle } from './extract-title.js';
import { buildErrorsTableAndAnnotate, buildTestResultsTable } from './tables.js';

export const MAX_COMMENT_SIZE = 262144; // GitHub's maximum comment size in bytes according to https://github.com/dead-claudia/github-limits#issue-comments

/**
 * Build the markdown bullet list of changed files, linking each one to its
 * blob on the PR's head ref.
 */
export function buildChangedFilesList(changedFiles, repoUrl, headRef) {
  return changedFiles.map(file => {
    const title = extractTitle(file);
    return `- [${title}](${repoUrl}/blob/${headRef}/${file})`;
  }).join("\n");
}

export function commentByteSize(text) {
  // Calculate the byte size of the comment text in UTF-8 encoding
  return Buffer.byteLength(text, 'utf8');
}

export function splitCommentIntoChunks(comment, maxCommentSize = MAX_COMMENT_SIZE) {
  const safetyMargin = commentByteSize(":open_book: Part 100 of 100\n\n")

  const chunks = [];
  let currentChunk = '';
  let lastTableHeader = ''; // Keep track of the last table header to avoid splitting in the middle of a table

  for (const line of comment.split('\n')) {
    const lineWithNewline = line + '\n';

    // If the line is a table header, store it
    if (line.startsWith('|') && line.includes('---')) {
      lastTableHeader = currentChunk.split('\n').slice(0,-1).pop() + '\n' + lineWithNewline;
    } else if (!line.startsWith('|') && lastTableHeader) {
      lastTableHeader = ''; // Reset if we are no longer in a table
    }

    if (commentByteSize(currentChunk + lineWithNewline) > maxCommentSize - safetyMargin) {

      let preparedNewChunk = '';

      // If the current chunk ends with a headline (e.g. ### Test Results) (and a table header), move it to the next chunk
      const chunkLines = currentChunk.split('\n');
      if (chunkLines.at(-1) === '') {
        chunkLines.pop(); // currentChunk always ends in a newline, drop the empty trailing entry
      }
      for (let i = chunkLines.length - 1; i >= 0; i--) {
        const line = chunkLines[i];

        if(line.trim() === '') {
          preparedNewChunk = line + '\n' + preparedNewChunk;
          chunkLines.splice(i, 1);
        } else if (line.startsWith('### ')) {
          preparedNewChunk = line + '\n' + preparedNewChunk;
          chunkLines.splice(i, 1);
        } else if( line.startsWith('|') && line.includes('---')) {
          preparedNewChunk = chunkLines.splice(i-1, 2).join('\n') + '\n' + preparedNewChunk;
          i--; // Skip the next line since we already spliced it
        }
        else {
          break;
        }

      }

      // If preparedNewChunk is still empty and we are in the middle of a table, copy the last table header to the next chunk
      if (preparedNewChunk === '' && lastTableHeader) {
        preparedNewChunk = lastTableHeader;
      }
      
      currentChunk = chunkLines.join('\n') + '\n';

      chunks.push(currentChunk);
      currentChunk = preparedNewChunk;
    }
    currentChunk += lineWithNewline;
  }

  if (currentChunk) {
    chunks.push(currentChunk);
  }

  if(chunks.length > 1) {
    chunks[0] += `\n\n:open_book: Part 1 of ${chunks.length}`;
    for(let i = 1; i < chunks.length; i++) {
      chunks[i] = `:open_book: Part ${i + 1} of ${chunks.length}\n\n` + chunks[i];
    }
  }
  return chunks;
}

/**
 * Render the full comment body: the changed/deleted file lists, the test
 * results table (when TEST_RESULTS was provided) and the conversion errors
 * table (when CONVERSION_ERRORS was provided).
 */
export function buildCommentBody({
  commentTitle,
  changedFiles,
  deletedFiles,
  testResults,
  conversionErrors,
  repoUrl,
  headRef,
}) {
  // Build file list with titles
  const changedFilesList = buildChangedFilesList(changedFiles, repoUrl, headRef);

  // Build test results table if TEST_RESULTS is provided
  const testResultsTable = testResults ? buildTestResultsTable(testResults) : '';

  // Build errors table if CONVERSION_ERRORS is provided
  const errorsTable = buildErrorsTableAndAnnotate(conversionErrors, repoUrl, headRef);

  const comment = `
### ${commentTitle}

| Changed | Deleted |
| --- | --- |
| ${changedFiles.length} | ${deletedFiles.length} |

### Changed Files

${changedFiles.length ? changedFilesList : "No files changed"}

### Deleted Files

${deletedFiles.length ? deletedFiles.map(file => `- ${file}`).join("\n") : "No files deleted"}

${testResultsTable ? '\n' + testResultsTable : ''}

${errorsTable ? '\n' + errorsTable : ''}
`;

  return comment;
}
