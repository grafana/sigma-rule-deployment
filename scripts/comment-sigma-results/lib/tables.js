import * as core from '@actions/core';
import path from 'path';

import { extractTitle } from './extract-title.js';

/**
 * Build test results table from TEST_RESULTS JSON
 */
export function buildTestResultsTable(testResults) {
  if (!testResults || Object.keys(testResults).length === 0) {
    return '';
  }

  let resultTable = `### Test Results\n\n| File name | Link | Result count | Execution time | Bytes processed | Errors |\n| --- | --- | --- | --- | --- | --- |\n`;

  for (const [filePath, results] of Object.entries(testResults)) {
    const title = extractTitle(filePath);
    for (const result of results) {
      const executionTime = result.stats.executionTime?.unit
        ? `${result.stats.executionTime.value} ${result.stats.executionTime.unit}`.trim()
        : '-';
      const bytesProcessed = result.stats.bytesProcessed?.unit
        ? `${result.stats.bytesProcessed.value.toLocaleString()} ${result.stats.bytesProcessed.unit}`.trim()
        : '-';
      const linkCell = result.link ? `[See in Explore](${result.link})` : '-';
      const errors = result.stats.errors || [];
      const errorCell = errors.length === 0
        ? '0'
        : `${errors.length}<br>${errors
          .map(error => String(error).replace(/\|/g, '\\|').replace(/\r?\n/g, '<br>'))
          .join('<br>')}`;

      if (process.env.GITHUB_ACTIONS) {
        for (const error of errors) {
          core.error(String(error), {
            title: `Integration Error in ${title}`,
            file: filePath,
          });
        }
      }

      resultTable += `| ${title} | ${linkCell} | ${result.stats.count} | ${executionTime} | ${bytesProcessed} | ${errorCell} |\n`;
    }
  }

  return resultTable;
}

export function buildErrorsTableAndAnnotate(conversionErrors, repoUrl, headRef) {
  if (!conversionErrors || conversionErrors.length === 0) {
    return '';
  }

  let errorsTable = `### Conversion Errors\n\n| File name | Link | Error message |\n| --- | --- | --- |\n`;

  for (const error of conversionErrors) {
    const title = error.conversion_name;
    const errorMessage = error.output || 'Unknown error';
    const errorCell = errorMessage.replace(/\|/g, '\\|').replace(/\r?\n/g, '<br>');
    const linkCell = error.input_file ? `[${path.basename(error.input_file)}](${repoUrl}/blob/${headRef}/${error.input_file})` : '-';
    errorsTable += `| ${title} | ${linkCell} | ${errorCell} |\n`;

    // Set GitHub Actions error annotation for the error.
    if (process.env.GITHUB_ACTIONS) {
      core.error(errorMessage, {
        title: `Conversion Error in ${title}`,
        file: error.input_file || ''
      })
    }
  }

  return errorsTable;
}
