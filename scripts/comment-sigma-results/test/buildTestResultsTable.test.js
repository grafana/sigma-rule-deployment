import { test } from 'node:test';
import assert from 'node:assert';
import * as commentModule from '../comment.js';

test('buildTestResultsTable - empty object returns empty string', () => {
  const result = commentModule.buildTestResultsTable({});
  assert.strictEqual(result, '');
});

test('buildTestResultsTable - null returns empty string', () => {
  const result = commentModule.buildTestResultsTable(null);
  assert.strictEqual(result, '');
});

test('buildTestResultsTable - undefined returns empty string', () => {
  const result = commentModule.buildTestResultsTable(undefined);
  assert.strictEqual(result, '');
});

test('buildTestResultsTable - single file with single result', () => {
  const testResults = {
    '/path/to/file1.json': [
      {
        datasource: 'loki',
        link: 'https://grafana.com/explore/123',
        stats: {
          count: 42,
          errors: [],
          fields: {}
        }
      }
    ]
  };

  // extractTitle will fall back to filename since file doesn't exist
  const result = commentModule.buildTestResultsTable(testResults);
  
  assert(result.includes('### Test Results'));
  assert(result.includes('| File name | Link | Result count | Errors |'));
  assert(result.includes('file1.json'));
  assert(result.includes('https://grafana.com/explore/123'));
  assert(result.includes('42'));
  assert(result.includes('0')); // error count
});

test('buildTestResultsTable - single file with multiple results', () => {
  const testResults = {
    '/path/to/file1.json': [
      {
        datasource: 'loki',
        link: 'https://grafana.com/explore/123',
        stats: {
          count: 42,
          errors: ['error1'],
          fields: {}
        }
      },
      {
        datasource: 'loki',
        link: 'https://grafana.com/explore/456',
        stats: {
          count: 15,
          errors: [],
          fields: {}
        }
      }
    ]
  };

  const result = commentModule.buildTestResultsTable(testResults);
  
  assert(result.includes('file1.json'));
  assert(result.includes('42'));
  assert(result.includes('1')); // error count
  assert(result.includes('15'));
  assert(result.includes('0')); // error count for second result
});

test('buildTestResultsTable - multiple files with results', () => {
  const testResults = {
    '/path/to/file1.json': [
      {
        datasource: 'loki',
        link: 'https://grafana.com/explore/123',
        stats: {
          count: 42,
          errors: [],
          fields: {}
        }
      }
    ],
    '/path/to/file2.json': [
      {
        datasource: 'elasticsearch',
        link: 'https://grafana.com/explore/456',
        stats: {
          count: 100,
          errors: ['error1', 'error2'],
          fields: {}
        }
      }
    ]
  };

  const result = commentModule.buildTestResultsTable(testResults);
  
  assert(result.includes('file1.json'));
  assert(result.includes('file2.json'));
  assert(result.includes('42'));
  assert(result.includes('100'));
  assert(result.includes('2')); // error count for file2
});

test('buildTestResultsTable - handles errors array correctly', () => {
  const testResults = {
    '/path/to/file1.json': [
      {
        datasource: 'loki',
        link: 'https://grafana.com/explore/123',
        stats: {
          count: 0,
          errors: ['error1', 'error2', 'error3'],
          fields: {}
        }
      }
    ]
  };

  const result = commentModule.buildTestResultsTable(testResults);
  
  assert(result.includes('file1.json'));
  assert(result.includes('https://grafana.com/explore/123'));
  assert(result.includes('0'));
  assert(result.includes('3')); // error count
});

