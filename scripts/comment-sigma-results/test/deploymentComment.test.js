import { test } from 'node:test';
import assert from 'node:assert';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const {
  buildDeploymentComment,
  normalizeDeploymentResults,
  parseDeploymentResults,
} = require('../../../actions/deploy/deployment-comment.cjs');

test('normalizeDeploymentResults sorts titles alphabetically and falls back to UID', () => {
  const results = normalizeDeploymentResults([
    { uid: 'uid-z', title: 'Zulu rule' },
    { uid: 'uid-b', title: 'beta rule' },
    { uid: 'uid-a', title: 'Alpha rule' },
    { uid: 'uid-fallback', title: '  ' },
    { uid: '' },
  ]);

  assert.deepStrictEqual(results, [
    { uid: 'uid-a', title: 'Alpha rule' },
    { uid: 'uid-b', title: 'beta rule' },
    { uid: 'uid-fallback', title: 'uid-fallback' },
    { uid: 'uid-z', title: 'Zulu rule' },
  ]);
});

test('parseDeploymentResults prefers JSON details and falls back to UID output', () => {
  assert.deepStrictEqual(
    parseDeploymentResults('[{"uid":"uid-b","title":"Beta"},{"uid":"uid-a","title":"Alpha"}]', 'ignored'),
    [
      { uid: 'uid-a', title: 'Alpha' },
      { uid: 'uid-b', title: 'Beta' },
    ]
  );

  assert.deepStrictEqual(parseDeploymentResults('invalid-json', 'uid-z uid-a'), [
    { uid: 'uid-a', title: 'uid-a' },
    { uid: 'uid-z', title: 'uid-z' },
  ]);
});

test('buildDeploymentComment uses sorted rule titles while preserving UID links', () => {
  const comment = buildDeploymentComment({
    created: [
      { uid: 'uid-z', title: 'Zulu rule' },
      { uid: 'uid-a', title: 'Alpha rule' },
    ],
    updated: [{ uid: 'uid-u', title: 'Updated rule' }],
    deleted: [
      { uid: 'uid-d2', title: 'Gamma rule' },
      { uid: 'uid-d1', title: 'Beta rule' },
    ],
    grafanaInstance: '"https://grafana.example/"',
  });

  assert(comment.includes('| 2 | 1 | 2 |'));
  assert(comment.includes('- [Alpha rule](https://grafana.example/alerting/grafana/uid-a/view)'));
  assert(comment.includes('- [Zulu rule](https://grafana.example/alerting/grafana/uid-z/view)'));
  assert(comment.indexOf('Alpha rule') < comment.indexOf('Zulu rule'));
  assert(comment.indexOf('Beta rule') < comment.indexOf('Gamma rule'));
  assert(!comment.includes('[uid-a]'));
  assert(!comment.includes('/uid-d1/view'));
});

test('buildDeploymentComment renders titles without links when the Grafana URL is absent', () => {
  const comment = buildDeploymentComment({
    created: [{ uid: 'uid-a', title: 'Alpha rule' }],
  });

  assert(comment.includes('- Alpha rule'));
  assert(!comment.includes('alerting/grafana'));
});
