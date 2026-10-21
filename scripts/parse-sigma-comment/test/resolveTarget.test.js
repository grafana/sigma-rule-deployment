import { test } from 'node:test';
import assert from 'node:assert';
import { resolveTarget, ParseError } from '../parse-sigma-comment.js';

const configurations = [
  { name: 'dev_github_audit' },
  { name: 'dev_aws_cloudtrail' },
  { name: 'ops_okta_rules_default' },
  { name: 'ops_okta_rules_target' },
  { name: 'ops_aws_cloudtrail_rules_5m' },
];

test('resolveTarget - exact name match', () => {
  assert.deepStrictEqual(resolveTarget('ops_okta_rules_default', configurations), ['ops_okta_rules_default']);
});

test('resolveTarget - name matching is case-sensitive', () => {
  assert.throws(() => resolveTarget('OPS_OKTA_RULES_DEFAULT', configurations), (err) => {
    assert(err instanceof ParseError);
    assert.match(err.message, /Could not resolve target `OPS_OKTA_RULES_DEFAULT`/);
    return true;
  });
});

test('resolveTarget - prefix matching multiple configs', () => {
  assert.deepStrictEqual(
    resolveTarget('ops', configurations),
    ['ops_okta_rules_default', 'ops_okta_rules_target', 'ops_aws_cloudtrail_rules_5m']
  );
});

test('resolveTarget - prefix matching a subset of an environment', () => {
  assert.deepStrictEqual(
    resolveTarget('ops_okta', configurations),
    ['ops_okta_rules_default', 'ops_okta_rules_target']
  );
});

test('resolveTarget - prefix matching a single config', () => {
  assert.deepStrictEqual(resolveTarget('dev_aws', configurations), ['dev_aws_cloudtrail']);
});

test('resolveTarget - prefix matching has no separator boundary', () => {
  // Deliberate: any naming style groups by prefix, at the cost of over-matches
  // like `dev` also selecting `devops_*`. Exact matches always win, and the
  // resolved names are logged, so over-selection is visible.
  const configs = [{ name: 'dev_web' }, { name: 'devops_runners' }, { name: 'devAWS' }];
  assert.deepStrictEqual(resolveTarget('dev', configs), ['dev_web', 'devops_runners', 'devAWS']);
});

test('resolveTarget - exact match wins over prefix expansion', () => {
  const configs = [{ name: 'aws' }, { name: 'aws_cloudtrail' }];
  assert.deepStrictEqual(resolveTarget('aws', configs), ['aws']);
});

test('resolveTarget - no matches throws ParseError with valid names', () => {
  assert.throws(() => resolveTarget('does_not_exist', configurations), (err) => {
    assert(err instanceof ParseError);
    assert.match(err.message, /Could not resolve target `does_not_exist`/);
    assert.strictEqual(
      err.validNames,
      'dev_github_audit, dev_aws_cloudtrail, ops_okta_rules_default, ops_okta_rules_target, ops_aws_cloudtrail_rules_5m'
    );
    return true;
  });
});
