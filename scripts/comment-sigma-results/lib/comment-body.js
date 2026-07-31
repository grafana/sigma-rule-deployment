import { extractTitle } from './extract-title.js';
import { buildErrorsTableAndAnnotate, buildTestResultsTable } from './tables.js';

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

  return `
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
}
