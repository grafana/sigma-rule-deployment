import { test } from 'node:test';
import assert from 'node:assert';
import fs from 'fs';
import { getInputs } from '../lib/inputs.js';

function withEnv(overrides, fn) {
  const original = {};
  for (const key of Object.keys(overrides)) {
    original[key] = process.env[key];
    if (overrides[key] === undefined) {
      delete process.env[key];
    } else {
      process.env[key] = overrides[key];
    }
  }
  try {
    fn();
  } finally {
    for (const key of Object.keys(original)) {
      if (original[key] === undefined) {
        delete process.env[key];
      } else {
        process.env[key] = original[key];
      }
    }
  }
}

test('getInputs - GitHub Actions branch reads and parses TEST_RESULTS_FILE', (t) => {
  const results = { 'rules/a.json': [{ datasource: 'loki', link: '', stats: { count: 1 } }] };

  withEnv({ GITHUB_ACTIONS: 'true', TEST_RESULTS_FILE: 'test-query-results.json' }, () => {
    t.mock.method(fs, 'readFileSync', () => JSON.stringify(results));
    const inputs = getInputs();
    assert.deepStrictEqual(inputs.testResults, results);
  });
});

test('getInputs - GitHub Actions branch returns null when TEST_RESULTS_FILE unset', (t) => {
  withEnv({ GITHUB_ACTIONS: 'true', TEST_RESULTS_FILE: undefined }, () => {
    const inputs = getInputs();
    assert.strictEqual(inputs.testResults, null);
  });
});

test('getInputs - GitHub Actions branch returns null when the file is missing', (t) => {
  withEnv({ GITHUB_ACTIONS: 'true', TEST_RESULTS_FILE: 'does-not-exist.json' }, () => {
    t.mock.method(fs, 'readFileSync', () => {
      throw new Error('ENOENT: no such file or directory');
    });
    const inputs = getInputs();
    assert.strictEqual(inputs.testResults, null);
  });
});

test('getInputs - GitHub Actions branch returns null on malformed JSON', (t) => {
  withEnv({ GITHUB_ACTIONS: 'true', TEST_RESULTS_FILE: 'test-query-results.json' }, () => {
    t.mock.method(fs, 'readFileSync', () => '{not valid json');
    const inputs = getInputs();
    assert.strictEqual(inputs.testResults, null);
  });
});

test('getInputs - CLI fallback branch reads and parses TEST_RESULTS_FILE', (t) => {
  const results = { 'rules/b.json': [{ datasource: 'elasticsearch', link: '', stats: { count: 2 } }] };

  withEnv({ GITHUB_ACTIONS: undefined, TEST_RESULTS_FILE: 'test-query-results.json' }, () => {
    t.mock.method(fs, 'readFileSync', () => JSON.stringify(results));
    const inputs = getInputs();
    assert.deepStrictEqual(inputs.testResults, results);
  });
});
