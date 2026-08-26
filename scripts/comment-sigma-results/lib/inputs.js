import fs from 'fs';
import path from 'path';

import * as core from '@actions/core';
import * as github from '@actions/github';

/**
 * Read and parse the JSON file referenced by TEST_RESULTS_FILE.
 */
function readTestResultsFile(filePath) {
  if (!filePath) return null;
  const resolved = path.isAbsolute(filePath)
    ? filePath
    : path.join(process.env.RULE_DIRECTORY_PATH || process.cwd(), filePath);
  try {
    return JSON.parse(fs.readFileSync(resolved, 'utf8'));
  } catch (e) {
    console.log(`Failed to read/parse test results file (${resolved}):`, e.message);
    return null;
  }
}

/**
 * Get inputs from environment variables or CLI arguments
 */
export function getInputs() {
  // Check if running in GitHub Actions
  const isGitHubActions = !!process.env.GITHUB_ACTIONS;

  if (isGitHubActions) {
    // Use @actions/core for GitHub Actions
    const testResultsFile = core.getInput('test_results_file') || process.env.TEST_RESULTS_FILE;
    const testResults = readTestResultsFile(testResultsFile);

    let errorResults = null;
    const errorResultsStr = core.getInput('conversion_errors') || process.env.CONVERSION_ERRORS;
    if (errorResultsStr) {
      try {
        errorResults = JSON.parse(errorResultsStr);
      } catch (e) {
        console.log('Failed to parse CONVERSION_ERRORS JSON:', e.message);
      }
    }

    return {
      pullRequestNumber: core.getInput('pull_request_number') || process.env.PULL_REQUEST_NUMBER,
      changedFiles: core.getInput('changed_files') || process.env.CHANGED_FILES || '',
      deletedFiles: core.getInput('deleted_files') || process.env.DELETED_FILES || '',
      commentTitle: core.getInput('comment_title') || process.env.COMMENT_TITLE,
      commentIdentifier: core.getInput('comment_identifier') || process.env.COMMENT_IDENTIFIER,
      testResults: testResults,
      conversionErrors: errorResults,
      githubToken: core.getInput('github_token') || process.env.GITHUB_TOKEN,
    };
  } else {
    // Parse CLI arguments for local testing
    const args = process.argv.slice(2);
    const inputs = {};

    for (let i = 0; i < args.length; i += 2) {
      const key = args[i]?.replace(/^--/, '');
      const value = args[i + 1];
      if (key && value) {
        inputs[key.replace(/-/g, '_')] = value;
      }
    }

    const testResultsFile = inputs.test_results_file || process.env.TEST_RESULTS_FILE;
    const testResults = readTestResultsFile(testResultsFile);

    let errorResults = null;
    const errorResultsStr = inputs.conversion_errors || process.env.CONVERSION_ERRORS;
    if (errorResultsStr) {
      try {
        errorResults = JSON.parse(errorResultsStr);
      } catch (e) {
        console.log('Failed to parse CONVERSION_ERRORS JSON:', e.message);
      }
    }

    // Fallback to environment variables
    return {
      pullRequestNumber: inputs.pull_request_number || process.env.PULL_REQUEST_NUMBER,
      changedFiles: inputs.changed_files || process.env.CHANGED_FILES || '',
      deletedFiles: inputs.deleted_files || process.env.DELETED_FILES || '',
      commentTitle: inputs.comment_title || process.env.COMMENT_TITLE,
      commentIdentifier: inputs.comment_identifier || process.env.COMMENT_IDENTIFIER,
      testResults: testResults,
      conversionErrors: errorResults,
      githubToken: inputs.github_token || process.env.GITHUB_TOKEN,
    };
  }
}

/**
 * Get GitHub context (repo owner/name)
 */
export function getContext() {
  if (process.env.GITHUB_ACTIONS) {
    // Use GitHub Actions context
    return github.context;
  } else {
    // Parse from GITHUB_REPOSITORY env var or CLI args
    const repo = process.env.GITHUB_REPOSITORY || '';
    const [owner, repoName] = repo.split('/');
    return {
      repo: {
        owner: owner || process.env.GITHUB_REPO_OWNER || '',
        repo: repoName || process.env.GITHUB_REPO_NAME || '',
      }
    };
  }
}
