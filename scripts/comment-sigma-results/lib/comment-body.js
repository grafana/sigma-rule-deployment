import { extractTitle } from './extract-title.js';

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
 * Assemble the full comment body from its pre-rendered sections.
 */
export function buildCommentBody({
  commentTitle,
  changedFiles,
  deletedFiles,
  changedFilesList,
  testResultsTable,
  errorsTable,
}) {
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
