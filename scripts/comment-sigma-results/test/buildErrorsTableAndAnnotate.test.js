import { test } from 'node:test';
import assert from 'node:assert';
import * as commentModule from '../comment.js';

// These tests run inside GitHub Actions, where GITHUB_ACTIONS is set. Unset it so
// the annotations don't show up on the job running the tests.
delete process.env.GITHUB_ACTIONS;

const REPO_URL = 'https://github.com/grafana/sigma-internal';
const HEAD_REF = 'my-branch';

test('buildErrorsTableAndAnnotate - empty array returns empty string', () => {
  const result = commentModule.buildErrorsTableAndAnnotate([], REPO_URL, HEAD_REF);
  assert.strictEqual(result, '');
});

test('buildErrorsTableAndAnnotate - null returns empty string', () => {
  const result = commentModule.buildErrorsTableAndAnnotate(null, REPO_URL, HEAD_REF);
  assert.strictEqual(result, '');
});

test('buildErrorsTableAndAnnotate - undefined returns empty string', () => {
  const result = commentModule.buildErrorsTableAndAnnotate(undefined, REPO_URL, HEAD_REF);
  assert.strictEqual(result, '');
});

test('buildErrorsTableAndAnnotate - single error renders header and row', () => {
  const conversionErrors = [
    {
      conversion_name: 'github_audit',
      input_file: 'rules/okta_user_created.yml',
      output: 'Unknown modifier "bogus"',
    }
  ];

  const result = commentModule.buildErrorsTableAndAnnotate(conversionErrors, REPO_URL, HEAD_REF);

  assert(result.includes('### Conversion Errors'));
  assert(result.includes('| File name | Link | Error message |'));
  assert(result.includes('github_audit'));
  assert(result.includes('Unknown modifier "bogus"'));
});

test('buildErrorsTableAndAnnotate - links to the rule file at the head ref', () => {
  const conversionErrors = [
    {
      conversion_name: 'github_audit',
      input_file: 'rules/cloud/okta_user_created.yml',
      output: 'boom',
    }
  ];

  const result = commentModule.buildErrorsTableAndAnnotate(conversionErrors, REPO_URL, HEAD_REF);

  // Link text is the basename, the href is the full path relative to the repo root
  assert(result.includes(
    '[okta_user_created.yml](https://github.com/grafana/sigma-internal/blob/my-branch/rules/cloud/okta_user_created.yml)'
  ));
});

test('buildErrorsTableAndAnnotate - renders dash when input_file is missing', () => {
  const conversionErrors = [
    { conversion_name: 'github_audit', output: 'boom' }
  ];

  const result = commentModule.buildErrorsTableAndAnnotate(conversionErrors, REPO_URL, HEAD_REF);

  assert(!result.includes(']('), 'should not render an empty markdown link');
  const dataLine = result.split('\n').find(line => line.includes('github_audit'));
  assert(dataLine.includes('| - |'), 'should render dash in link cell');
});

test('buildErrorsTableAndAnnotate - replaces newlines so the row stays on one line', () => {
  const conversionErrors = [
    {
      conversion_name: 'github_audit',
      input_file: 'rules/test.yml',
      output: 'Errors found in Sigma rules:\n* Unknown modifier "bogus"',
    }
  ];

  const result = commentModule.buildErrorsTableAndAnnotate(conversionErrors, REPO_URL, HEAD_REF);

  assert(result.includes('Errors found in Sigma rules:<br>* Unknown modifier "bogus"'));
  const dataLines = result.split('\n').filter(line => line.includes('github_audit'));
  assert.strictEqual(dataLines.length, 1, 'a raw newline would split this into two rows');
});

test('buildErrorsTableAndAnnotate - escapes pipes in the error message', () => {
  const conversionErrors = [
    {
      conversion_name: 'github_audit',
      input_file: 'rules/test.yml',
      // Loki queries are full of pipes, so this is the common case
      output: '{job="x"} | json | line_format broke',
    }
  ];

  const result = commentModule.buildErrorsTableAndAnnotate(conversionErrors, REPO_URL, HEAD_REF);

  assert(result.includes('\\| json \\| line_format broke'));
});

test('buildErrorsTableAndAnnotate - falls back to Unknown error when output is empty', () => {
  const conversionErrors = [
    { conversion_name: 'github_audit', input_file: 'rules/test.yml', output: '' }
  ];

  const result = commentModule.buildErrorsTableAndAnnotate(conversionErrors, REPO_URL, HEAD_REF);

  assert(result.includes('Unknown error'));
});

test('buildErrorsTableAndAnnotate - renders one row per error', () => {
  const conversionErrors = [
    { conversion_name: 'conversion_a', input_file: 'rules/a.yml', output: 'boom a' },
    { conversion_name: 'conversion_b', input_file: 'rules/b.yml', output: 'boom b' },
  ];

  const result = commentModule.buildErrorsTableAndAnnotate(conversionErrors, REPO_URL, HEAD_REF);

  assert(result.includes('conversion_a'));
  assert(result.includes('boom a'));
  assert(result.includes('conversion_b'));
  assert(result.includes('boom b'));
});
