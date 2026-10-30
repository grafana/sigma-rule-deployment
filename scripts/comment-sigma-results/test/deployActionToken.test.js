import { test } from 'node:test';
import assert from 'node:assert';
import { readFileSync } from 'node:fs';

const actionPath = new URL('../../../actions/deploy/action.yml', import.meta.url);

/**
 * Splits the composite steps of an action.yml into one block of lines per step.
 * The steps are the "    - " entries under "runs.steps", which is enough for
 * this file without pulling in a YAML parser.
 */
function getSteps(actionYaml) {
  const lines = actionYaml.split('\n');
  const start = lines.findIndex((line) => line === '  steps:');
  assert(start !== -1, 'expected a runs.steps section in actions/deploy/action.yml');

  const steps = [];
  for (const line of lines.slice(start + 1)) {
    if (/^\S/.test(line)) {
      break;
    }
    if (line.startsWith('    - ')) {
      steps.push([]);
    }
    steps.at(-1)?.push(line);
  }
  return steps.map((step) => step.join('\n'));
}

test('the deploy Action comments on pull requests with the github_token input', () => {
  const steps = getSteps(readFileSync(actionPath, 'utf8'));
  const commentSteps = steps.filter((step) => /^\s+uses: actions\/github-script@/m.test(step));

  // One step comments the deployed alerts, the other comments a failed deployment.
  assert.strictEqual(commentSteps.length, 2, 'expected two actions/github-script steps');
  for (const step of commentSteps) {
    const name = step.match(/^\s+- name: (.*)$/m)?.[1];
    // Without github-token, actions/github-script falls back to github.token, and the
    // reusable workflow's custom_github_token secret is silently ignored.
    assert.match(
      step,
      /^ +github-token: \$\{\{ inputs\.github_token \}\}$/m,
      `step "${name}" must pass github-token: \${{ inputs.github_token }} to actions/github-script`,
    );
  }
});
