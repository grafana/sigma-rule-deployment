import { test } from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const script = fileURLToPath(new URL('../determine-image-ref.sh', import.meta.url));

function runScript(args = [], env = {}) {
  return execFileSync('bash', [script, ...args], {
    encoding: 'utf8',
    stdio: 'pipe',
    env: { ...process.env, ACTION_REF: '', GITHUB_OUTPUT: '', ...env },
  });
}

const validVersions = [
  // Normal versions, including zero and components beyond integer precision.
  '0.0.0',
  '0.0.1',
  '0.1.0',
  '1.0.0',
  '1.2.3',
  '10.20.30',
  '999999999999999999999999.999999999999999999999999.999999999999999999999999',

  // Prereleases, including the examples from https://semver.org/.
  '1.0.0-alpha',
  '1.0.0-alpha.1',
  '1.0.0-alpha.beta',
  '1.0.0-beta',
  '1.0.0-beta.2',
  '1.0.0-beta.11',
  '1.0.0-rc.1',
  '1.0.0-0.3.7',
  '1.0.0-x.7.z.92',
  '1.0.0-x-y-z.--',
  '1.2.3-0',
  '1.2.3-123',
  '1.2.3-01alpha',
  '1.2.3-01-',
  '1.2.3-01-2',
  '1.2.3-ALPHA',
  '1.2.3--',
  '1.2.3---.-',
  '1.2.3-a-b-c.00a.0.9',

  // Build metadata allows numeric identifiers with leading zeroes.
  '1.0.0+0',
  '1.0.0+001',
  '1.0.0+20130313144700',
  '1.0.0+21AF26D3----117B344092BD',
  '1.2.3+build.001',
  '1.2.3+000.001.0002',
  '1.2.3+-',
  '1.2.3+BUILD.Metadata-1',

  // Prereleases combined with build metadata.
  '1.0.0-alpha+001',
  '1.0.0-beta+exp.sha.5114f85',
  '1.2.3-rc.1+build.123',
  '1.2.3-0+000',
  '1.2.3--+-',
];

for (const version of validVersions) {
  const ref = `v${version}`;
  test(`preserves valid Semantic Version tag ${ref}`, () => {
    assert.equal(runScript([ref]), `Using tag: ${ref}\nimage_ref=${ref}\n`);
  });
}

for (const ref of [
  // The original pattern accepts any number of numeric components and leading zeroes.
  'v0',
  'v1',
  'v10',
  'v00',
  'v01',
  'v0.0',
  'v0.1',
  'v1.0',
  'v1.2',
  'v1.3',
  'v10.20',
  'v01.2',
  'v1.02',
  'v00.001',
  'v01.2.3',
  'v1.02.3',
  'v1.2.03',
  'v1.2.3.4',
  'v1.2.3.4.5',
  'v00.001.0002.00003',
  'v999999999999999999999999',
  'v999999999999999999999999.999999999999999999999999',
]) {
  test(`preserves previously accepted numeric version tag ${ref}`, () => {
    assert.equal(runScript([ref]), `Using tag: ${ref}\nimage_ref=${ref}\n`);
  });
}

const unsupportedVersionRefs = [
  // Numeric components must be nonempty and separated by single dots.
  'v',
  'v.',
  'v1.',
  'v.2',
  'v1..2',
  'v1.2.',
  'v1.2.3.',

  // Tags with prerelease or build metadata must follow all SemVer rules.
  'v01.2.3-alpha',
  'v1.02.3+build.001',
  'v1.2.03-rc.1',
  'v1.2.3.4-alpha',
  'v1.2.3.4+build.001',
  'v1.2.3-01',
  'v1.2.3-alpha.01',
  'v1.2.3-01.alpha',

  // Prerelease and build metadata require a complete Semantic Version.
  'v1.2-rc.1',
  'v1.2+build.001',

  // Empty identifiers and repeated separators.
  'v1.2.3-',
  'v1.2.3-alpha.',
  'v1.2.3-.alpha',
  'v1.2.3-alpha..beta',
  'v1.2.3+',
  'v1.2.3+build.',
  'v1.2.3+.build',
  'v1.2.3+build..meta',
  'v1.2.3+build+extra',

  // Identifiers must contain only ASCII letters, digits, and hyphens.
  'v1.2.3-alpha_beta',
  'v1.2.3+build_meta',
  'v1.2.3-alpha!',
  'v1.2.3+build!',
  'v1.2.3-é',
  'v1.2.3+é',
  'v١.2.3',
  'v1.٢.3',
  'v1.2.٣',
  'v1.2.3-١',
  'v1.2.3+١',

  // The entire ref must match, with the existing lowercase v prefix.
  '1.2',
  '1.2.3',
  '1.2.3-alpha+001',
  'V1.2',
  'V1.2.3',
  'vv1.2',
  'vv1.2.3',
  ' v1.2',
  ' v1.2.3',
  'v1.2 ',
  'v1.2.3 ',
  'v1.2\n',
  'v1.2.3\n',
  'v1.2.3\njunk',
];

for (const ref of unsupportedVersionRefs) {
  test(`uses SHA fallback for unsupported version ref ${JSON.stringify(ref)}`, () => {
    assert.equal(runScript([ref]), `Using SHA: sha-${ref}\nimage_ref=sha-${ref}\n`);
  });
}

test('maps latest to main', () => {
  assert.equal(runScript(['latest']), 'Using main tag\nimage_ref=main\n');
});

for (const ref of ['abc123def456', 'main']) {
  test(`uses SHA fallback for ${ref}`, () => {
    assert.equal(runScript([ref]), `Using SHA: sha-${ref}\nimage_ref=sha-${ref}\n`);
  });
}

for (const ref of ['v1', 'v1.2', 'v1.3', 'v01.2.3', 'v1.2.3.4', 'v1.2.3-rc.1+build.001']) {
  test(`reads ${ref} from ACTION_REF and appends to GITHUB_OUTPUT`, (t) => {
    const directory = mkdtempSync(join(tmpdir(), 'determine-image-ref-'));
    t.after(() => rmSync(directory, { recursive: true, force: true }));
    const outputFile = join(directory, 'output');
    writeFileSync(outputFile, 'existing=output\n');

    assert.equal(
      runScript([], { ACTION_REF: ref, GITHUB_OUTPUT: outputFile }),
      `Using tag: ${ref}\n`,
    );
    assert.equal(readFileSync(outputFile, 'utf8'), `existing=output\nimage_ref=${ref}\n`);
  });
}

test('prefers the argument over ACTION_REF from the environment', () => {
  assert.equal(
    runScript(['v1.2.3'], { ACTION_REF: 'latest' }),
    'Using tag: v1.2.3\nimage_ref=v1.2.3\n',
  );
});

test('requires an action reference', () => {
  assert.throws(() => runScript(), (error) => {
    assert.equal(error.status, 1);
    assert.equal(
      error.stderr,
      'Error: ACTION_REF must be provided as argument or environment variable\n',
    );
    return true;
  });
});
