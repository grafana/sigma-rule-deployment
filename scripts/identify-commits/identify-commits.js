#!/usr/bin/env node

/**
 * Identify Relevant Commits Script
 * 
 * Identifies relevant commits in a pull request for change detection.
 * Finds base commit, first commit, automation commits, and calculates previous refs.
 * 
 * Usage:
 *   node identify-commits.js
 * 
 * Environment variables (for GitHub Actions):
 *   PULL_REQUEST_NUMBER, ACTIONS_USERNAME, GITHUB_TOKEN
 * 
 * CLI arguments (for local testing):
 *   --pr-number, --actions-username, --token
 */

import * as core from '@actions/core';
import * as github from '@actions/github';

/**
 * Get inputs from environment variables or CLI arguments
 */
function getInputs() {
  // Check if running in GitHub Actions
  const isGitHubActions = !!process.env.GITHUB_ACTIONS;
  
  if (isGitHubActions) {
    // Use @actions/core for GitHub Actions
    return {
      pullRequestNumber: core.getInput('pull_request_number') || process.env.PULL_REQUEST_NUMBER || github.context.issue?.number,
      actionsUsername: core.getInput('actions_username') || process.env.ACTIONS_USERNAME || 'github-actions[bot]',
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
    
    // Fallback to environment variables
    return {
      pullRequestNumber: inputs.pull_request_number || process.env.PULL_REQUEST_NUMBER,
      actionsUsername: inputs.actions_username || process.env.ACTIONS_USERNAME || 'github-actions[bot]',
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
    // Parse from GITHUB_REPOSITORY env var
    const repo = process.env.GITHUB_REPOSITORY || '';
    const [owner, repoName] = repo.split('/');
    return {
      repo: {
        owner: owner || process.env.GITHUB_REPO_OWNER || '',
        repo: repoName || process.env.GITHUB_REPO_NAME || '',
      },
      issue: {
        number: parseInt(process.env.PULL_REQUEST_NUMBER || '0', 10)
      }
    };
  }
}

/**
 * Identify relevant commits in a pull request
 */
async function identifyCommits(octokit, context, prNumber, actionsUsername) {
  const iterator = await octokit.paginate(octokit.rest.pulls.listCommits.endpoint.merge({
    owner: context.repo.owner,
    repo: context.repo.repo,
    pull_number: prNumber,
    per_page: 1
  }));

  let previousCommit = "";
  let firstCommit = "";
  let lastCommit = "";
  let baseCommit = "";

  for (const remoteCommit of iterator) {
    if (previousCommit === "" && remoteCommit.parents.length > 0) {
      previousCommit = remoteCommit.parents[0].sha;
      baseCommit = remoteCommit.parents[0].sha;
      firstCommit = remoteCommit.sha;
    }
    if (remoteCommit.commit.author.name === actionsUsername) {
      previousCommit = remoteCommit.sha;
      lastCommit = remoteCommit.sha;
    }
  }

  const previousRef = lastCommit || previousCommit;

  return {
    baseCommit,
    previousCommit,
    firstCommit,
    lastCommit,
    previousRef
  };
}

/**
 * Set outputs (to GitHub Actions or stdout)
 */
function setOutputs(results) {
  const outputs = {
    'base-commit': results.baseCommit,
    'previous-commit': results.previousCommit,
    'first-commit': results.firstCommit,
    'last-commit': results.lastCommit,
    'previous-ref': results.previousRef
  };

  if (process.env.GITHUB_ACTIONS) {
    // Write to GitHub Actions outputs
    for (const [key, value] of Object.entries(outputs)) {
      core.setOutput(key, value);
    }
  } else {
    // Write to stdout for local testing
    for (const [key, value] of Object.entries(outputs)) {
      console.log(`${key}=${value}`);
    }
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
      throw new Error('pull_request_number is required (or must be available from GitHub context)');
    }
    if (!inputs.githubToken) {
      throw new Error('github_token is required');
    }

    const context = getContext();
    
    if (!context.repo.owner || !context.repo.repo) {
      throw new Error('Repository owner and name must be provided via GITHUB_REPOSITORY or GITHUB_REPO_OWNER/GITHUB_REPO_NAME');
    }

    // Initialize GitHub client
    const octokit = github.getOctokit(inputs.githubToken);

    // Identify commits
    const results = await identifyCommits(
      octokit,
      context,
      parseInt(inputs.pullRequestNumber, 10),
      inputs.actionsUsername
    );

    // Log results
    console.log(`Base commit or base ref: ${results.baseCommit}`);
    console.log(`Last commit or base ref: ${results.previousCommit}`);
    console.log(`PR First Commit: ${results.firstCommit}`);
    console.log(`PR Last Commit by automation: ${results.lastCommit}`);
    console.log(`PR Previous Ref: ${results.previousRef}`);

    // Set outputs
    setOutputs(results);

    console.log('Commit identification completed successfully');
  } catch (error) {
    if (process.env.GITHUB_ACTIONS) {
      core.setFailed(error.message);
    } else {
      console.error('Error:', error.message);
      process.exit(1);
    }
  }
}

// Run if executed directly
const isMainModule = import.meta.url === `file://${process.argv[1]}`.replace(/\\/g, '/') ||
  process.argv[1]?.endsWith('identify-commits.js') ||
  process.argv[1]?.endsWith('identify-commits');

if (isMainModule) {
  main();
}

export { main, identifyCommits, getInputs, getContext, setOutputs };

