import { test } from 'node:test';
import assert from 'node:assert';
import fs from 'fs';
import { extractTitle } from '../comment.js';

test('extractTitle - top-level title in JSON', (t) => {
  const filePath = '/tmp/test1.json';
  const content = JSON.stringify({ title: 'Test Rule Title', other: 'data' });
  
  t.mock.method(fs, 'existsSync', () => true);
  t.mock.method(fs, 'readFileSync', () => content);
  
  const result = extractTitle(filePath);
  assert.strictEqual(result, 'Test Rule Title');
});

test('extractTitle - title in rules array', (t) => {
  const filePath = '/tmp/test2.json';
  const content = JSON.stringify({
    rules: [
      { title: 'First Rule Title', id: '1' },
      { title: 'Second Rule', id: '2' }
    ]
  });
  
  t.mock.method(fs, 'existsSync', () => true);
  t.mock.method(fs, 'readFileSync', () => content);
  
  const result = extractTitle(filePath);
  assert.strictEqual(result, 'First Rule Title');
});

test('extractTitle - title with whitespace trimmed', (t) => {
  const filePath = '/tmp/test3.json';
  const content = JSON.stringify({ title: '  Padded Title  ' });
  
  t.mock.method(fs, 'existsSync', () => true);
  t.mock.method(fs, 'readFileSync', () => content);
  
  const result = extractTitle(filePath);
  assert.strictEqual(result, 'Padded Title');
});

test('extractTitle - fallback to regex when JSON parsing fails', (t) => {
  const filePath = '/tmp/test4.json';
  const content = '{"invalid": json, "title": "Regex Title"}';
  
  t.mock.method(fs, 'existsSync', () => true);
  t.mock.method(fs, 'readFileSync', () => content);
  
  const result = extractTitle(filePath);
  assert.strictEqual(result, 'Regex Title');
});

test('extractTitle - fallback to filename when no title found', (t) => {
  const filePath = '/tmp/test5.json';
  const content = JSON.stringify({ other: 'data', noTitle: true });
  
  t.mock.method(fs, 'existsSync', () => true);
  t.mock.method(fs, 'readFileSync', () => content);
  
  const result = extractTitle(filePath);
  assert.strictEqual(result, 'test5.json');
});

test('extractTitle - fallback to filename when file does not exist', (t) => {
  const filePath = '/tmp/nonexistent.json';
  
  t.mock.method(fs, 'existsSync', () => false);
  
  const result = extractTitle(filePath);
  assert.strictEqual(result, 'nonexistent.json');
});

test('extractTitle - fallback to filename on read error', (t) => {
  const filePath = '/tmp/error.json';
  
  t.mock.method(fs, 'existsSync', () => true);
  t.mock.method(fs, 'readFileSync', () => {
    throw new Error('Permission denied');
  });
  
  const result = extractTitle(filePath);
  assert.strictEqual(result, 'error.json');
});

test('extractTitle - empty rules array falls back to filename', (t) => {
  const filePath = '/tmp/test6.json';
  const content = JSON.stringify({ rules: [] });
  
  t.mock.method(fs, 'existsSync', () => true);
  t.mock.method(fs, 'readFileSync', () => content);
  
  const result = extractTitle(filePath);
  assert.strictEqual(result, 'test6.json');
});

test('extractTitle - rules array with no title field falls back to filename', (t) => {
  const filePath = '/tmp/test7.json';
  const content = JSON.stringify({
    rules: [
      { id: '1', name: 'Rule 1' }
    ]
  });
  
  t.mock.method(fs, 'existsSync', () => true);
  t.mock.method(fs, 'readFileSync', () => content);
  
  const result = extractTitle(filePath);
  assert.strictEqual(result, 'test7.json');
});

