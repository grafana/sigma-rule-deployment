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
  assert(result.includes('| File name | Link | Result count | Execution time | Bytes processed | Errors |'));
  assert(result.includes('file1.json'));
  assert(result.includes('https://grafana.com/explore/123'));
  assert(result.includes('42'));
  assert(result.includes('0')); // error count
  assert(result.includes('-')); // execution time and bytes processed should be '-' when not provided
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

test('buildTestResultsTable - displays execution time and bytes processed when provided', () => {
  const testResults = {
    '/path/to/file1.json': [
      {
        datasource: 'loki',
        link: 'https://grafana.com/explore/123',
        stats: {
          count: 42,
          errors: [],
          fields: {},
          executionTime: {
            value: 2.534394,
            unit: 's'
          },
          bytesProcessed: {
            value: 1234567,
            unit: 'decbytes'
          }
        }
      }
    ]
  };

  const result = commentModule.buildTestResultsTable(testResults);

  assert(result.includes('file1.json'));
  assert(result.includes('2.534394 s'));
  assert(result.includes('1,234,567 decbytes'));
  assert(result.includes('42'));
});

test('buildTestResultsTable - displays zero values when present, shows dash when missing', () => {
  // Test with zero values - should display them
  const testResultsZero = {
    '/path/to/file1.json': [
      {
        datasource: 'loki',
        link: 'https://grafana.com/explore/123',
        stats: {
          count: 42,
          errors: [],
          fields: {},
          executionTime: {
            value: 0,
            unit: 's'
          },
          bytesProcessed: {
            value: 0,
            unit: 'decbytes'
          }
        }
      }
    ]
  };

  const resultZero = commentModule.buildTestResultsTable(testResultsZero);
  assert(resultZero.includes('file1.json'));
  assert(resultZero.includes('0 s'), 'Should display zero execution time with unit');
  assert(resultZero.includes('0 decbytes'), 'Should display zero bytes processed with unit');

  // Test with missing fields - should show dash
  const testResultsMissing = {
    '/path/to/file2.json': [
      {
        datasource: 'loki',
        link: 'https://grafana.com/explore/456',
        stats: {
          count: 42,
          errors: [],
          fields: {}
          // executionTime and bytesProcessed are missing
        }
      }
    ]
  };

  const resultMissing = commentModule.buildTestResultsTable(testResultsMissing);
  assert(resultMissing.includes('file2.json'));
  const linesMissing = resultMissing.split('\n');
  const dataLineMissing = linesMissing.find(line => line.includes('file2.json'));
  assert(dataLineMissing.includes('-'), 'Should show dash for missing fields');
});

