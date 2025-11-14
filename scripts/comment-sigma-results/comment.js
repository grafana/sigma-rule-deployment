#!/usr/bin/env node

/**
 * Comment Sigma Results Script
 * 
 * Posts a comment to a PR with Sigma rule conversion/integration results.
 * Can be run standalone or as part of a GitHub Action.
 * 
 * Usage:
 *   node comment.js [options]
 * 
 * Environment variables (for GitHub Actions):
 *   PULL_REQUEST_NUMBER, CHANGED_FILES, DELETED_FILES, COMMENT_TITLE,
 *   COMMENT_IDENTIFIER, TEST_RESULTS, GITHUB_TOKEN
 * 
 * CLI arguments (for local testing):
 *   --pr-number, --changed-files, --deleted-files, --title, --identifier,
 *   --test-results, --token
 */

import * as core from '@actions/core';
import * as github from '@actions/github';
import fs from 'fs';
import path from 'path';

/**
 * Extract title from JSON file
 */
function extractTitle(filePath) {
  try {
    if (!fs.existsSync(filePath)) {
      console.log(`File does not exist: ${filePath}`);
      return path.basename(filePath);
    }

    const content = fs.readFileSync(filePath, 'utf8');
    
    // Try JSON parsing first
    try {
      const jsonData = JSON.parse(content);
      
      // Check for title at top level (for alert rule files)
      if (jsonData.title && typeof jsonData.title === 'string') {
        return jsonData.title.trim();
      }
      
      // Check for title in rules array (for conversion output files)
      if (jsonData.rules && Array.isArray(jsonData.rules) && jsonData.rules.length > 0) {
        const firstRule = jsonData.rules[0];
        if (firstRule && firstRule.title && typeof firstRule.title === 'string') {
          return firstRule.title.trim();
        }
      }
    } catch (jsonError) {
      // JSON parsing failed, will try regex fallback
      console.log(`JSON parsing failed for ${filePath}: ${jsonError.message}, trying regex fallback`);
    }

    // Fallback to regex if JSON parsing didn't find title
    const titleMatch = content.match(/"title":\s*"([^"]+)"/);
    if (titleMatch && titleMatch[1]) {
      return titleMatch[1].trim();
    }

    // Final fallback to filename if no title found
    console.log(`No title found in ${filePath} using JSON or regex`);
    return path.basename(filePath);
  } catch (error) {
    console.log(`Error reading file ${filePath}: ${error.message}`);
    // Fallback to filename if file can't be read
    return path.basename(filePath);
  }
}

/**
 * Build test results table from TEST_RESULTS JSON
 */
function buildTestResultsTable(testResults) {
  if (!testResults || Object.keys(testResults).length === 0) {
    return '';
  }

  let resultTable = `### Test Results\n\n| File name | Link | Result count | Errors |\n| --- | --- | --- | --- |\n`;

  for (const [filePath, results] of Object.entries(testResults)) {
    const title = extractTitle(filePath);
    for (const result of results) {
      resultTable += `| ${title} | [See in Explore](${result.link}) | ${result.stats.count} | ${result.stats.errors.length} |\n`;
    }
  }

  return resultTable;
}

/**
 * Get inputs from environment variables or CLI arguments
 */
function getInputs() {
  // Check if running in GitHub Actions
  const isGitHubActions = !!process.env.GITHUB_ACTIONS;
  
  if (isGitHubActions) {
    // Use @actions/core for GitHub Actions
    let testResults = null;
    const testResultsStr = core.getInput('test_results') || process.env.TEST_RESULTS;
    if (testResultsStr) {
      try {
        testResults = JSON.parse(testResultsStr);
      } catch (e) {
        console.log('Failed to parse TEST_RESULTS JSON:', e.message);
      }
    }

    return {
      pullRequestNumber: core.getInput('pull_request_number') || process.env.PULL_REQUEST_NUMBER,
      changedFiles: core.getInput('changed_files') || process.env.CHANGED_FILES || '',
      deletedFiles: core.getInput('deleted_files') || process.env.DELETED_FILES || '',
      commentTitle: core.getInput('comment_title') || process.env.COMMENT_TITLE,
      commentIdentifier: core.getInput('comment_identifier') || process.env.COMMENT_IDENTIFIER,
      testResults: testResults,
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
    
    let testResults = null;
    const testResultsStr = inputs.test_results || process.env.TEST_RESULTS;
    if (testResultsStr) {
      try {
        testResults = JSON.parse(testResultsStr);
      } catch (e) {
        console.log('Failed to parse TEST_RESULTS JSON:', e.message);
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
      githubToken: inputs.github_token || process.env.GITHUB_TOKEN,
    };
  }
}

/**
 * Get GitHub context (repo owner/name)
 */
function getContext() {
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

/**
 * Main function
 */
async function main() {
  try {
    const inputs = getInputs();
    
    // Validate required inputs
    if (!inputs.pullRequestNumber) {
      throw new Error('pull_request_number is required');
    }
    if (!inputs.commentTitle) {
      throw new Error('comment_title is required');
    }
    if (!inputs.commentIdentifier) {
      throw new Error('comment_identifier is required');
    }
    if (!inputs.githubToken) {
      throw new Error('github_token is required');
    }

    const changedFiles = inputs.changedFiles.split(' ').filter(file => file.trim() !== '');
    const deletedFiles = inputs.deletedFiles.split(' ').filter(file => file.trim() !== '');
    
    // Initialize GitHub client
    const octokit = github.getOctokit(inputs.githubToken);
    const context = getContext();
    
    if (!context.repo.owner || !context.repo.repo) {
      throw new Error('Repository owner and name must be provided via GITHUB_REPOSITORY or GITHUB_REPO_OWNER/GITHUB_REPO_NAME');
    }
    
    // Get PR data
    const prData = await octokit.rest.pulls.get({
      owner: context.repo.owner,
      repo: context.repo.repo,
      pull_number: parseInt(inputs.pullRequestNumber, 10),
    });
    
    if (!prData.data) {
      console.log(`No pull request found for ${context.repo.owner}/${context.repo.repo}#${inputs.pullRequestNumber}`);
      return;
    }

    const nodeId = prData.data.node_id;

    // Build file list with titles
    const changedFilesList = changedFiles.map(file => {
      const title = extractTitle(file);
      return `- [${title}](https://github.com/${context.repo.owner}/${context.repo.repo}/blob/${prData.data.head.ref}/${file})`;
    }).join("\n");

    // Build test results table if TEST_RESULTS is provided
    const testResultsTable = inputs.testResults ? buildTestResultsTable(inputs.testResults) : '';

    const comment = `
### ${inputs.commentTitle}

| Changed | Deleted |
| --- | --- |
| ${changedFiles.length} | ${deletedFiles.length} |

### Changed Files

${changedFiles.length ? changedFilesList : "No files changed"}

### Deleted Files

${deletedFiles.length ? deletedFiles.map(file => `- ${file}`).join("\n") : "No files deleted"}

${testResultsTable ? '\n' + testResultsTable : ''}
`;

    // GraphQL queries
    const oldCommentQuery = `query GetPRComments($owner: String!, $name: String!, $number: Int!) {
        repository(owner: $owner, name: $name) {
          id
          pullRequest(number: $number) {
            title
            comments(last: 100) {
              nodes {
                id
                bodyText
                isMinimized
                author {
                  login
                }
              }
            }
          }
        }
    }`;

    const minimizeCommentMutation = `mutation MinimizeComment($subjectId: ID!) {
      minimizeComment(input: {
        subjectId: $subjectId,
        classifier: OUTDATED
      }) {
        clientMutationId
      }
    }`;

    const addCommentMutation = `mutation AddComment($body: String!, $subjectId: ID!) {
      addComment(input: {
        body: $body,
        subjectId: $subjectId,
      }) {
        subject {
          id
          ... on PullRequest {
            number
          }
        }
      }
    }`;

    // Find and minimize old comments
    const comments = await octokit.graphql(oldCommentQuery, {
      owner: context.repo.owner,
      name: context.repo.repo,
      number: parseInt(inputs.pullRequestNumber, 10)
    });

    for (const comment of comments?.repository?.pullRequest?.comments?.nodes ?? []) {
      if (!comment.isMinimized && comment.bodyText.startsWith(inputs.commentIdentifier)) {
        await octokit.graphql(minimizeCommentMutation, {
          subjectId: comment.id
        });
      }
    }

    // Post new comment
    await octokit.graphql(addCommentMutation, {
      body: comment,
      subjectId: nodeId
    });

    console.log('Comment posted successfully');
  } catch (error) {
    if (process.env.GITHUB_ACTIONS) {
      core.setFailed(error.message);
    } else {
      console.error('Error:', error.message);
      process.exit(1);
    }
  }
}

// Run if executed directly (check if this is the main module)
const isMainModule = import.meta.url === `file://${process.argv[1]}`.replace(/\\/g, '/') ||
  process.argv[1]?.endsWith('comment.js') ||
  process.argv[1]?.endsWith('comment');

if (isMainModule) {
  main();
}

export { main, extractTitle, buildTestResultsTable };

