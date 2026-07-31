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

import { buildCommentBody } from './lib/comment-body.js';
import { extractTitle } from './lib/extract-title.js';
import { getContext, getInputs } from './lib/inputs.js';
import { addCommentMutation, minimizeCommentMutation, oldCommentQuery } from './lib/queries.js';
import { buildErrorsTableAndAnnotate, buildTestResultsTable } from './lib/tables.js';

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

    const comment = buildCommentBody({
      commentTitle: inputs.commentTitle,
      changedFiles,
      deletedFiles,
      testResults: inputs.testResults,
      conversionErrors: inputs.conversionErrors,
      repoUrl: `https://github.com/${context.repo.owner}/${context.repo.repo}`,
      headRef: prData.data.head.ref,
    });

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

export { main, extractTitle, buildTestResultsTable, buildErrorsTableAndAnnotate };
