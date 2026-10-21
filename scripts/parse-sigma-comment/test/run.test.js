import { test } from 'node:test';
import assert from 'node:assert';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { run } from '../parse-sigma-comment.js';

const CONFIG_YAML = `version: 2
folders:
  conversion_path: "./conversions"
  deployment_path: "./deployments"
configurations:
  - name: aws_cloudtrail
    conversion:
      input: "rules/cloud/aws/cloudtrail/*"
  - name: gcp_k8s
    conversion:
      input: "rules/cloud/kubernetes/audit/*"
  - name: azure_audit
    conversion:
      input: "rules/cloud/azure/audit/*"
  - name: multi_input
    conversion:
      input:
        - "rules/cloud/okta/*"
        - "rules/cloud/auth0/*"
  - name: aws_cloudtrail_correlations
    conversion:
      input: "rules/correlation/cloud/aws/cloudtrail/*"
`;

function writeTempConfig(t) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'parse-sigma-comment-'));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const configPath = path.join(dir, 'config.yml');
  fs.writeFileSync(configPath, CONFIG_YAML);
  return configPath;
}

function createMockOctokit() {
  const calls = { reactions: [], comments: [] };
  return {
    calls,
    rest: {
      reactions: {
        createForIssueComment: async (params) => { calls.reactions.push(params); },
      },
      issues: {
        createComment: async (params) => { calls.comments.push(params); },
      },
    },
  };
}

const context = { repo: { owner: 'test-owner', repo: 'test-repo' } };

function makeInputs(configPath, overrides = {}) {
  return {
    commentBody: '',
    commentAuthorAssociation: 'MEMBER',
    commentId: '123',
    issueNumber: '456',
    allRules: 'false',
    configPath,
    githubToken: 'fake-token',
    ...overrides,
  };
}

test('run - all_rules input path (non-comment trigger)', async (t) => {
  const configPath = writeTempConfig(t);
  const octokit = createMockOctokit();

  const outputs = await run(makeInputs(configPath, { allRules: 'true' }), context, octokit);

  assert.strictEqual(outputs.triggered, 'true');
  assert.strictEqual(outputs.action, 'convert');
  assert.strictEqual(outputs.all_rules, 'true');
  assert.strictEqual(outputs.configurations, '');
  assert.strictEqual(outputs.no_test, 'false');
  assert.strictEqual(outputs.debug, 'false');
  assert.strictEqual(outputs.query_time_window, '');
  assert.strictEqual(outputs.error, '');
  // No reactions for the non-comment path
  assert.strictEqual(octokit.calls.reactions.length, 0);
});

test('run - untrusted author is ignored', async (t) => {
  const configPath = writeTempConfig(t);
  const octokit = createMockOctokit();

  for (const association of ['CONTRIBUTOR', 'NONE', 'FIRST_TIME_CONTRIBUTOR']) {
    const outputs = await run(
      makeInputs(configPath, { commentBody: 'sigma convert all', commentAuthorAssociation: association }),
      context,
      octokit
    );
    assert.strictEqual(outputs.triggered, 'false', `association ${association} should not trigger`);
    assert.strictEqual(outputs.error, '');
  }
  assert.strictEqual(octokit.calls.reactions.length, 0);
});

test('run - trusted associations are accepted', async (t) => {
  const configPath = writeTempConfig(t);

  for (const association of ['OWNER', 'MEMBER', 'COLLABORATOR']) {
    const octokit = createMockOctokit();
    const outputs = await run(
      makeInputs(configPath, { commentBody: 'sigma convert all', commentAuthorAssociation: association }),
      context,
      octokit
    );
    assert.strictEqual(outputs.triggered, 'true', `association ${association} should trigger`);
  }
});

test('run - non-sigma comment is ignored', async (t) => {
  const configPath = writeTempConfig(t);
  const octokit = createMockOctokit();

  const outputs = await run(makeInputs(configPath, { commentBody: 'looks good to me!' }), context, octokit);

  assert.strictEqual(outputs.triggered, 'false');
  assert.strictEqual(outputs.error, '');
  assert.strictEqual(octokit.calls.reactions.length, 0);
  assert.strictEqual(octokit.calls.comments.length, 0);
});

test('run - sigma convert all', async (t) => {
  const configPath = writeTempConfig(t);
  const octokit = createMockOctokit();

  const outputs = await run(makeInputs(configPath, { commentBody: 'sigma convert all' }), context, octokit);

  assert.strictEqual(outputs.triggered, 'true');
  assert.strictEqual(outputs.action, 'convert');
  assert.strictEqual(outputs.all_rules, 'true');
  assert.strictEqual(outputs.configurations, '');
  assert.strictEqual(outputs.error, '');
  assert.strictEqual(octokit.calls.reactions.length, 1);
  assert.strictEqual(octokit.calls.reactions[0].content, 'eyes');
  assert.strictEqual(octokit.calls.reactions[0].comment_id, 123);
});

test('run - convert with name target', async (t) => {
  const configPath = writeTempConfig(t);
  const octokit = createMockOctokit();

  const outputs = await run(makeInputs(configPath, { commentBody: 'sigma convert aws_cloudtrail' }), context, octokit);

  assert.strictEqual(outputs.triggered, 'true');
  assert.strictEqual(outputs.action, 'convert');
  assert.strictEqual(outputs.all_rules, 'false');
  assert.strictEqual(outputs.configurations, 'aws_cloudtrail');
});

test('run - integrate with name target', async (t) => {
  const configPath = writeTempConfig(t);
  const octokit = createMockOctokit();

  const outputs = await run(makeInputs(configPath, { commentBody: 'sigma integrate gcp_k8s' }), context, octokit);

  assert.strictEqual(outputs.triggered, 'true');
  assert.strictEqual(outputs.action, 'integrate');
  assert.strictEqual(outputs.configurations, 'gcp_k8s');
});

test('run - prefix target matching multiple configs', async (t) => {
  const configPath = writeTempConfig(t);
  const octokit = createMockOctokit();

  const outputs = await run(
    makeInputs(configPath, { commentBody: 'sigma convert aws' }),
    context,
    octokit
  );

  assert.strictEqual(outputs.triggered, 'true');
  assert.strictEqual(outputs.configurations, 'aws_cloudtrail aws_cloudtrail_correlations');
});

test('run - each flag', async (t) => {
  const configPath = writeTempConfig(t);

  let outputs = await run(
    makeInputs(configPath, { commentBody: 'sigma convert aws_cloudtrail --no-test' }),
    context,
    createMockOctokit()
  );
  assert.strictEqual(outputs.no_test, 'true');
  assert.strictEqual(outputs.debug, 'false');

  outputs = await run(
    makeInputs(configPath, { commentBody: 'sigma convert aws_cloudtrail --debug' }),
    context,
    createMockOctokit()
  );
  assert.strictEqual(outputs.debug, 'true');
  assert.strictEqual(outputs.no_test, 'false');

  outputs = await run(
    makeInputs(configPath, { commentBody: 'sigma convert aws_cloudtrail --query-time-window 6h' }),
    context,
    createMockOctokit()
  );
  assert.strictEqual(outputs.query_time_window, '6h');
});

test('run - combined flags', async (t) => {
  const configPath = writeTempConfig(t);
  const octokit = createMockOctokit();

  const outputs = await run(
    makeInputs(configPath, { commentBody: 'sigma integrate aws_cloudtrail --no-test --debug --query-time-window 1d12h' }),
    context,
    octokit
  );

  assert.strictEqual(outputs.triggered, 'true');
  assert.strictEqual(outputs.action, 'integrate');
  assert.strictEqual(outputs.no_test, 'true');
  assert.strictEqual(outputs.debug, 'true');
  assert.strictEqual(outputs.query_time_window, '1d12h');
});

test('run - keywords are case-insensitive', async (t) => {
  const configPath = writeTempConfig(t);
  const octokit = createMockOctokit();

  const outputs = await run(
    makeInputs(configPath, { commentBody: '  SIGMA CONVERT aws_cloudtrail --NO-TEST' }),
    context,
    octokit
  );

  assert.strictEqual(outputs.triggered, 'true');
  assert.strictEqual(outputs.action, 'convert');
  assert.strictEqual(outputs.configurations, 'aws_cloudtrail');
  assert.strictEqual(outputs.no_test, 'true');
});

test('run - configuration names are case-sensitive', async (t) => {
  const configPath = writeTempConfig(t);
  const octokit = createMockOctokit();

  const outputs = await run(
    makeInputs(configPath, { commentBody: 'sigma convert AWS_CLOUDTRAIL' }),
    context,
    octokit
  );

  assert.strictEqual(outputs.triggered, 'false');
  assert.match(outputs.error, /Could not resolve target `AWS_CLOUDTRAIL`/);
});

test('run - unknown action reports error, reaction and reply', async (t) => {
  const configPath = writeTempConfig(t);
  const octokit = createMockOctokit();

  const outputs = await run(makeInputs(configPath, { commentBody: 'sigma delete all' }), context, octokit);

  assert.strictEqual(outputs.triggered, 'false');
  assert.match(outputs.error, /Unknown action `delete`/);
  assert.strictEqual(octokit.calls.reactions.length, 1);
  assert.strictEqual(octokit.calls.reactions[0].content, 'confused');
  assert.strictEqual(octokit.calls.comments.length, 1);
  assert.strictEqual(octokit.calls.comments[0].issue_number, 456);
  assert.match(octokit.calls.comments[0].body, /Unknown action `delete`/);
  assert.match(octokit.calls.comments[0].body, /Usage:/);
  assert.match(octokit.calls.comments[0].body, /sigma convert all/);
});

test('run - missing target reports error', async (t) => {
  const configPath = writeTempConfig(t);
  const octokit = createMockOctokit();

  const outputs = await run(makeInputs(configPath, { commentBody: 'sigma convert' }), context, octokit);

  assert.strictEqual(outputs.triggered, 'false');
  assert.match(outputs.error, /Missing target/);
  assert.strictEqual(octokit.calls.reactions[0].content, 'confused');
});

test('run - unresolvable target reply lists valid configuration names', async (t) => {
  const configPath = writeTempConfig(t);
  const octokit = createMockOctokit();

  const outputs = await run(
    makeInputs(configPath, { commentBody: 'sigma convert does_not_exist' }),
    context,
    octokit
  );

  assert.strictEqual(outputs.triggered, 'false');
  assert.match(outputs.error, /Could not resolve target `does_not_exist`/);
  assert.strictEqual(octokit.calls.reactions[0].content, 'confused');
  assert.match(octokit.calls.comments[0].body, /Valid configuration names: aws_cloudtrail, gcp_k8s, azure_audit, multi_input, aws_cloudtrail_correlations/);
});

test('run - unknown flag reports error', async (t) => {
  const configPath = writeTempConfig(t);
  const octokit = createMockOctokit();

  const outputs = await run(
    makeInputs(configPath, { commentBody: 'sigma convert all --verbose' }),
    context,
    octokit
  );

  assert.strictEqual(outputs.triggered, 'false');
  assert.match(outputs.error, /Unknown flag `--verbose`/);
});

test('run - invalid duration reports error', async (t) => {
  const configPath = writeTempConfig(t);
  const octokit = createMockOctokit();

  const outputs = await run(
    makeInputs(configPath, { commentBody: 'sigma convert all --query-time-window 5x' }),
    context,
    octokit
  );

  assert.strictEqual(outputs.triggered, 'false');
  assert.match(outputs.error, /Invalid duration `5x`/);
  assert.match(octokit.calls.comments[0].body, /Usage:/);
});

test('run - reaction API failure is tolerated', async (t) => {
  const configPath = writeTempConfig(t);
  const octokit = createMockOctokit();
  octokit.rest.reactions.createForIssueComment = async () => { throw new Error('API down'); };

  const outputs = await run(makeInputs(configPath, { commentBody: 'sigma convert all' }), context, octokit);

  assert.strictEqual(outputs.triggered, 'true');
  assert.strictEqual(outputs.all_rules, 'true');
});

test('run - reply API failure is tolerated', async (t) => {
  const configPath = writeTempConfig(t);
  const octokit = createMockOctokit();
  octokit.rest.issues.createComment = async () => { throw new Error('API down'); };

  const outputs = await run(makeInputs(configPath, { commentBody: 'sigma delete all' }), context, octokit);

  assert.strictEqual(outputs.triggered, 'false');
  assert.match(outputs.error, /Unknown action/);
});

test('run - missing config file is an execution error', async (t) => {
  const octokit = createMockOctokit();

  await assert.rejects(
    () => run(makeInputs('/does/not/exist.yml', { commentBody: 'sigma convert all' }), context, octokit),
    /Config file not found/
  );
});

test('run - works without octokit (no token)', async (t) => {
  const configPath = writeTempConfig(t);

  const outputs = await run(makeInputs(configPath, { commentBody: 'sigma convert all' }), context, null);

  assert.strictEqual(outputs.triggered, 'true');
});
