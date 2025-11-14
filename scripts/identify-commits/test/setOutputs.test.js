import { test } from 'node:test';
import assert from 'node:assert';
import * as identifyModule from '../identify-commits.js';

test('setOutputs - GitHub Actions mode', () => {
  const originalEnv = process.env.GITHUB_ACTIONS;
  process.env.GITHUB_ACTIONS = 'true';

  const results = {
    baseCommit: 'abc123',
    previousCommit: 'def456',
    firstCommit: 'ghi789',
    lastCommit: 'jkl012',
    previousRef: 'jkl012'
  };

  // Verify it doesn't throw (actual output verification is done in stdout mode test)
  // In GitHub Actions, it calls core.setOutput which we can't easily mock
  assert.doesNotThrow(() => {
    identifyModule.setOutputs(results);
  });

  process.env.GITHUB_ACTIONS = originalEnv;
});

test('setOutputs - non-GitHub Actions mode (stdout)', () => {
  const originalEnv = process.env.GITHUB_ACTIONS;
  delete process.env.GITHUB_ACTIONS;

  const results = {
    baseCommit: 'abc123',
    previousCommit: 'def456',
    firstCommit: 'ghi789',
    lastCommit: 'jkl012',
    previousRef: 'jkl012'
  };

  // Capture console.log output
  const originalLog = console.log;
  const logCalls = [];
  console.log = (...args) => {
    logCalls.push(args.join(' '));
  };

  identifyModule.setOutputs(results);

  // Verify outputs were written to stdout
  assert.strictEqual(logCalls.length, 5);
  assert(logCalls.some(line => line === 'base-commit=abc123'));
  assert(logCalls.some(line => line === 'previous-commit=def456'));
  assert(logCalls.some(line => line === 'first-commit=ghi789'));
  assert(logCalls.some(line => line === 'last-commit=jkl012'));
  assert(logCalls.some(line => line === 'previous-ref=jkl012'));

  console.log = originalLog;
  process.env.GITHUB_ACTIONS = originalEnv;
});

test('setOutputs - handles empty values', () => {
  const originalEnv = process.env.GITHUB_ACTIONS;
  delete process.env.GITHUB_ACTIONS;

  const results = {
    baseCommit: '',
    previousCommit: '',
    firstCommit: '',
    lastCommit: '',
    previousRef: ''
  };

  const originalLog = console.log;
  const logCalls = [];
  console.log = (...args) => {
    logCalls.push(args.join(' '));
  };

  identifyModule.setOutputs(results);

  // Verify empty values are still output
  assert.strictEqual(logCalls.length, 5);
  assert(logCalls.some(line => line === 'base-commit='));
  assert(logCalls.some(line => line === 'last-commit='));

  console.log = originalLog;
  process.env.GITHUB_ACTIONS = originalEnv;
});

