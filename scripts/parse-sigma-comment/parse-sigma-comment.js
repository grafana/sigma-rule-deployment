#!/usr/bin/env node

/**
 * Parse Sigma Comment Script
 *
 * Parses `sigma convert` / `sigma integrate` commands from PR issue comments
 * and resolves targets against the v2 configuration file.
 *
 * Supported grammar (keywords are case-insensitive):
 *   sigma convert all
 *   sigma convert <target> [--no-test] [--debug] [--query-time-window <duration>]
 *   sigma integrate all
 *   sigma integrate <target> [--no-test] [--debug] [--query-time-window <duration>]
 *
 * <target> is either a configuration name from the v2 config (case-sensitive)
 * or a name prefix: `ops` matches every configuration whose name starts with
 * `ops`. Exact matches win over prefix matches.
 *
 * Usage:
 *   node parse-sigma-comment.js
 *
 * Environment variables (for GitHub Actions):
 *   COMMENT_BODY, COMMENT_AUTHOR_ASSOCIATION, COMMENT_AUTHOR_TYPE, COMMENT_ID,
 *   ISSUE_NUMBER, CONFIG_PATH, GITHUB_TOKEN
 *
 * CLI arguments (for local testing):
 *   --comment-body, --comment-author-association, --comment-author-type,
 *   --comment-id, --issue-number, --config-path, --github-token
 */

import * as core from '@actions/core';
import * as github from '@actions/github';
import fs from 'node:fs';
import yaml from 'js-yaml';

const TRUSTED_ASSOCIATIONS = ['OWNER', 'MEMBER', 'COLLABORATOR'];

const USAGE = [
  'Usage:',
  '  sigma convert all',
  '  sigma convert <target> [--no-test] [--debug] [--query-time-window <duration>]',
  '  sigma integrate all',
  '  sigma integrate <target> [--no-test] [--debug] [--query-time-window <duration>]',
].join('\n');

// Digits followed by s/m/h/d units, possibly repeated (e.g. 30m, 6h, 1d, 1d12h)
const DURATION_PATTERN = /^(?:\d+[smhd])+$/;

/**
 * Error type for command parse/resolution problems. These are reported back to
 * the commenter and never fail the workflow; any other error is an execution
 * error and fails the step.
 */
class ParseError extends Error {}

/**
 * Get inputs from environment variables or CLI arguments
 */
function getInputs() {
  // Check if running in GitHub Actions
  const isGitHubActions = !!process.env.GITHUB_ACTIONS;

  if (isGitHubActions) {
    // Use @actions/core for GitHub Actions
    return {
      commentBody: core.getInput('comment_body') || process.env.COMMENT_BODY || github.context.payload?.comment?.body || '',
      commentAuthorAssociation: core.getInput('comment_author_association') || process.env.COMMENT_AUTHOR_ASSOCIATION || github.context.payload?.comment?.author_association || '',
      commentAuthorLogin: core.getInput('comment_author_login') || process.env.COMMENT_AUTHOR_LOGIN || github.context.payload?.comment?.user?.login || '',
      commentAuthorType: core.getInput('comment_author_type') || process.env.COMMENT_AUTHOR_TYPE || github.context.payload?.comment?.user?.type || '',
      commentId: core.getInput('comment_id') || process.env.COMMENT_ID || github.context.payload?.comment?.id || '',
      issueNumber: core.getInput('issue_number') || process.env.ISSUE_NUMBER || github.context.issue?.number || '',
      configPath: core.getInput('config_path') || process.env.CONFIG_PATH || 'config/config.yml',
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
      commentBody: inputs.comment_body || process.env.COMMENT_BODY || '',
      commentAuthorAssociation: inputs.comment_author_association || process.env.COMMENT_AUTHOR_ASSOCIATION || '',
      commentAuthorLogin: inputs.comment_author_login || process.env.COMMENT_AUTHOR_LOGIN || '',
      commentAuthorType: inputs.comment_author_type || process.env.COMMENT_AUTHOR_TYPE || '',
      commentId: inputs.comment_id || process.env.COMMENT_ID || '',
      issueNumber: inputs.issue_number || process.env.ISSUE_NUMBER || '',
      configPath: inputs.config_path || process.env.CONFIG_PATH || 'config/config.yml',
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
        number: parseInt(process.env.ISSUE_NUMBER || '0', 10)
      }
    };
  }
}

/**
 * Parse a sigma command comment into a structured command.
 * Throws ParseError on any grammar violation.
 */
function parseCommand(commentBody) {
  // Commands are single-line; anything after the first line (automation
  // trailers, explanations) is ignored.
  const firstLine = commentBody.split(/\r?\n/, 1)[0];
  const tokens = firstLine.trim().split(/\s+/);

  // tokens[0] is 'sigma' (validated by the caller)
  const actionToken = tokens[1]?.toLowerCase();
  if (!actionToken) {
    throw new ParseError('Missing action. Expected `convert` or `integrate`.');
  }
  if (actionToken !== 'convert' && actionToken !== 'integrate') {
    throw new ParseError(`Unknown action \`${tokens[1]}\`. Expected \`convert\` or \`integrate\`.`);
  }

  const target = tokens[2];
  if (!target || target.startsWith('--')) {
    throw new ParseError('Missing target. Expected `all` or a configuration name/glob.');
  }

  const command = {
    action: actionToken,
    target,
    allRules: target.toLowerCase() === 'all',
    noTest: false,
    debug: false,
    queryTimeWindow: '',
  };

  for (let i = 3; i < tokens.length; i++) {
    const flag = tokens[i].toLowerCase();
    switch (flag) {
      case '--no-test':
        command.noTest = true;
        break;
      case '--debug':
        command.debug = true;
        break;
      case '--query-time-window': {
        const value = tokens[++i];
        if (!value) {
          throw new ParseError('Flag `--query-time-window` requires a duration value (e.g. `30m`, `6h`, `1d12h`).');
        }
        if (!DURATION_PATTERN.test(value)) {
          throw new ParseError(
            `Invalid duration \`${value}\` for \`--query-time-window\`. ` +
            'Expected digits followed by s/m/h/d units, e.g. `30m`, `6h`, `1d`, `1d12h`.'
          );
        }
        command.queryTimeWindow = value;
        break;
      }
      default:
        if (flag.startsWith('--')) {
          throw new ParseError(`Unknown flag \`${tokens[i]}\`. Expected \`--no-test\`, \`--debug\` or \`--query-time-window\`.`);
        }
        throw new ParseError(`Unexpected argument \`${tokens[i]}\`.`);
    }
  }

  return command;
}

/**
 * Load the v2 config file and return its configurations list.
 * Throws a plain Error (execution error) when the file is missing or malformed.
 */
function loadConfig(configPath) {
  if (!fs.existsSync(configPath)) {
    throw new Error(`Config file not found: ${configPath}`);
  }
  const config = yaml.load(fs.readFileSync(configPath, 'utf8'));
  if (!Array.isArray(config?.configurations)) {
    throw new Error(`Config file ${configPath} does not contain a 'configurations' list`);
  }
  return config.configurations;
}

/**
 * Resolve a command target to a list of configuration names:
 *   1. Exact (case-sensitive) match against configurations[].name
 *   2. Prefix match: `ops` selects every configuration whose name starts with
 *      `ops`, with no separator boundary — any naming style groups by prefix,
 *      at the cost of over-matches such as `dev` also selecting `devops_*`.
 * Throws ParseError (with validNames attached) when nothing matches.
 */
function resolveTarget(target, configurations) {
  const exact = configurations.find((c) => c.name === target);
  if (exact) {
    return [exact.name];
  }

  const matches = configurations
    .filter((c) => typeof c.name === 'string' && c.name.startsWith(target))
    .map((c) => c.name);

  if (matches.length === 0) {
    const error = new ParseError(`Could not resolve target \`${target}\` to any configuration name or name prefix.`);
    error.validNames = configurations.map((c) => c.name).join(', ');
    throw error;
  }

  return matches;
}

/**
 * Add a reaction to the comment, tolerating API failures.
 */
async function addReaction(octokit, context, commentId, content) {
  if (!octokit || !commentId) {
    return;
  }
  try {
    await octokit.rest.reactions.createForIssueComment({
      owner: context.repo.owner,
      repo: context.repo.repo,
      comment_id: Number(commentId),
      content,
    });
    console.log(`Reacted to comment ${commentId} with ${content} emoji`);
  } catch (error) {
    console.log(`Could not react to comment: ${error.message}`);
  }
}

/**
 * Post a reply comment on the issue/PR, tolerating API failures.
 */
async function postReply(octokit, context, issueNumber, body) {
  if (!octokit || !issueNumber) {
    return;
  }
  try {
    await octokit.rest.issues.createComment({
      owner: context.repo.owner,
      repo: context.repo.repo,
      issue_number: Number(issueNumber),
      body,
    });
    console.log(`Posted reply to issue #${issueNumber}`);
  } catch (error) {
    console.log(`Could not post reply: ${error.message}`);
  }
}

/**
 * Build the full outputs object with defaults, overridden by the given values.
 */
function buildOutputs(overrides = {}) {
  return {
    triggered: 'false',
    action: '',
    all_rules: 'false',
    configurations: '',
    no_test: 'false',
    debug: 'false',
    query_time_window: '',
    error: '',
    ...overrides,
  };
}

/**
 * Core logic: evaluate the inputs and return the outputs object.
 * Reactions and reply comments are best-effort side effects.
 * ParseErrors are reported, never thrown; anything else is an execution error.
 */
async function run(inputs, context, octokit) {
  // Security-critical: never parse commands from untrusted commenters.
  // - Bots: only installed GitHub Apps and the repo's own
  //   workflows can comment as a bot, and both are controlled by repo admins.
  // - Humans need OWNER, MEMBER or COLLABORATOR association; MEMBER covers org
  //   members whose repo role comes from any team.
  const isBot = (inputs.commentAuthorType || '').toLowerCase() === 'bot';
  const association = (inputs.commentAuthorAssociation || '').toUpperCase();
  if (!isBot && !TRUSTED_ASSOCIATIONS.includes(association)) {
    console.log(`Ignoring comment from untrusted author ${inputs.commentAuthorLogin || 'unknown'} (type: ${inputs.commentAuthorType || 'unknown'}, association: ${inputs.commentAuthorAssociation || 'unknown'})`);
    return buildOutputs();
  }

  const body = inputs.commentBody || '';
  if (!/^\s*sigma\b/i.test(body)) {
    console.log('Comment does not start with `sigma`; ignoring');
    return buildOutputs();
  }

  // The config is needed to resolve targets; a missing/malformed config is a
  // real execution error and should fail the step.
  const configurations = loadConfig(inputs.configPath);

  try {
    const command = parseCommand(body);
    const resolvedNames = command.allRules ? [] : resolveTarget(command.target, configurations);

    const outputs = buildOutputs({
      triggered: 'true',
      action: command.action,
      all_rules: command.allRules ? 'true' : 'false',
      configurations: resolvedNames.join(' '),
      no_test: command.noTest ? 'true' : 'false',
      debug: command.debug ? 'true' : 'false',
      query_time_window: command.queryTimeWindow,
    });

    console.log(`Parsed command: action=${command.action} target=${command.target}`);
    await addReaction(octokit, context, inputs.commentId, 'eyes');
    return outputs;
  } catch (error) {
    if (!(error instanceof ParseError)) {
      throw error;
    }
    console.log(`Parse error: ${error.message}`);
    await addReaction(octokit, context, inputs.commentId, 'confused');
    let reply = `${error.message}\n\n${USAGE}`;
    if (error.validNames) {
      reply += `\n\nValid configuration names: ${error.validNames}`;
    }
    await postReply(octokit, context, inputs.issueNumber, reply);
    return buildOutputs({ error: error.message });
  }
}

/**
 * Set outputs (to GitHub Actions or stdout)
 */
function setOutputs(outputs) {
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
    const context = getContext();

    // The token is only needed for reactions/replies; their failures are tolerated
    const octokit = inputs.githubToken ? github.getOctokit(inputs.githubToken) : null;

    const outputs = await run(inputs, context, octokit);
    setOutputs(outputs);

    console.log('Comment parsing completed successfully');
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
  process.argv[1]?.endsWith('parse-sigma-comment.js') ||
  process.argv[1]?.endsWith('parse-sigma-comment');

if (isMainModule) {
  main();
}

export {
  main,
  run,
  getInputs,
  getContext,
  setOutputs,
  parseCommand,
  resolveTarget,
  loadConfig,
  buildOutputs,
  addReaction,
  postReply,
  ParseError,
  USAGE,
};
