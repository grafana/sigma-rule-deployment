import { test } from 'node:test';
import assert from 'node:assert';
import { parseCommand, ParseError } from '../parse-sigma-comment.js';

test('parseCommand - sigma convert all', () => {
  const command = parseCommand('sigma convert all');
  assert.strictEqual(command.action, 'convert');
  assert.strictEqual(command.target, 'all');
  assert.strictEqual(command.allRules, true);
  assert.strictEqual(command.noTest, false);
  assert.strictEqual(command.debug, false);
  assert.strictEqual(command.queryTimeWindow, '');
});

test('parseCommand - sigma integrate all', () => {
  const command = parseCommand('sigma integrate all');
  assert.strictEqual(command.action, 'integrate');
  assert.strictEqual(command.allRules, true);
});

test('parseCommand - convert with name target', () => {
  const command = parseCommand('sigma convert aws_cloudtrail');
  assert.strictEqual(command.action, 'convert');
  assert.strictEqual(command.target, 'aws_cloudtrail');
  assert.strictEqual(command.allRules, false);
});

test('parseCommand - integrate with name target', () => {
  const command = parseCommand('sigma integrate gcp_k8s');
  assert.strictEqual(command.action, 'integrate');
  assert.strictEqual(command.target, 'gcp_k8s');
  assert.strictEqual(command.allRules, false);
});

test('parseCommand - --no-test flag', () => {
  const command = parseCommand('sigma convert aws_cloudtrail --no-test');
  assert.strictEqual(command.noTest, true);
  assert.strictEqual(command.debug, false);
});

test('parseCommand - --debug flag', () => {
  const command = parseCommand('sigma convert aws_cloudtrail --debug');
  assert.strictEqual(command.debug, true);
  assert.strictEqual(command.noTest, false);
});

test('parseCommand - --query-time-window flag', () => {
  const command = parseCommand('sigma convert aws_cloudtrail --query-time-window 1d12h');
  assert.strictEqual(command.queryTimeWindow, '1d12h');
});

test('parseCommand - combined flags', () => {
  const command = parseCommand('sigma integrate aws_cloudtrail --no-test --debug --query-time-window 30m');
  assert.strictEqual(command.action, 'integrate');
  assert.strictEqual(command.noTest, true);
  assert.strictEqual(command.debug, true);
  assert.strictEqual(command.queryTimeWindow, '30m');
});

test('parseCommand - keywords are case-insensitive', () => {
  const command = parseCommand('SIGMA CONVERT ALL --NO-TEST --DEBUG --QUERY-TIME-WINDOW 6h');
  assert.strictEqual(command.action, 'convert');
  assert.strictEqual(command.allRules, true);
  assert.strictEqual(command.noTest, true);
  assert.strictEqual(command.debug, true);
  assert.strictEqual(command.queryTimeWindow, '6h');
});

test('parseCommand - target case is preserved', () => {
  const command = parseCommand('SIGMA CONVERT AWS_CLOUDTRAIL');
  assert.strictEqual(command.target, 'AWS_CLOUDTRAIL');
  assert.strictEqual(command.allRules, false);
});

test('parseCommand - tolerates extra whitespace', () => {
  const command = parseCommand('  sigma   convert   aws_cloudtrail   --debug  ');
  assert.strictEqual(command.action, 'convert');
  assert.strictEqual(command.target, 'aws_cloudtrail');
  assert.strictEqual(command.debug, true);
});

test('parseCommand - valid durations', () => {
  for (const duration of ['30s', '30m', '6h', '1d', '1d12h', '2d4h30m10s']) {
    const command = parseCommand(`sigma convert all --query-time-window ${duration}`);
    assert.strictEqual(command.queryTimeWindow, duration);
  }
});

test('parseCommand - missing action', () => {
  assert.throws(() => parseCommand('sigma'), (err) => {
    assert(err instanceof ParseError);
    assert.match(err.message, /Missing action/);
    return true;
  });
});

test('parseCommand - unknown action', () => {
  assert.throws(() => parseCommand('sigma delete all'), (err) => {
    assert(err instanceof ParseError);
    assert.match(err.message, /Unknown action `delete`/);
    return true;
  });
});

test('parseCommand - missing target', () => {
  assert.throws(() => parseCommand('sigma convert'), (err) => {
    assert(err instanceof ParseError);
    assert.match(err.message, /Missing target/);
    return true;
  });
});

test('parseCommand - flag in place of target is a missing target', () => {
  assert.throws(() => parseCommand('sigma convert --no-test'), (err) => {
    assert(err instanceof ParseError);
    assert.match(err.message, /Missing target/);
    return true;
  });
});

test('parseCommand - unknown flag', () => {
  assert.throws(() => parseCommand('sigma convert all --verbose'), (err) => {
    assert(err instanceof ParseError);
    assert.match(err.message, /Unknown flag `--verbose`/);
    return true;
  });
});

test('parseCommand - unexpected extra argument', () => {
  assert.throws(() => parseCommand('sigma convert all extra'), (err) => {
    assert(err instanceof ParseError);
    assert.match(err.message, /Unexpected argument `extra`/);
    return true;
  });
});

test('parseCommand - invalid durations', () => {
  for (const duration of ['5x', 'm', '30', '1.5h', '30meters', '-30m']) {
    assert.throws(
      () => parseCommand(`sigma convert all --query-time-window ${duration}`),
      (err) => {
        assert(err instanceof ParseError);
        assert.match(err.message, /Invalid duration/);
        return true;
      },
      `expected duration '${duration}' to be rejected`
    );
  }
});

test('parseCommand - --query-time-window without value', () => {
  assert.throws(() => parseCommand('sigma convert all --query-time-window'), (err) => {
    assert(err instanceof ParseError);
    assert.match(err.message, /requires a duration value/);
    return true;
  });
});

test('parseCommand - ignores everything after the first line', () => {
  const command = parseCommand('sigma convert all\n\nTriggered by grafana/sigma-rule-deployment#419, commit grafana/sigma-rule-deployment@761f9e0');
  assert.strictEqual(command.action, 'convert');
  assert.strictEqual(command.allRules, true);
});

test('parseCommand - flags on the first line with a trailer after', () => {
  const command = parseCommand('sigma convert aws_cloudtrail --no-test --query-time-window 6h\nsome explanation\nacross lines');
  assert.strictEqual(command.target, 'aws_cloudtrail');
  assert.strictEqual(command.noTest, true);
  assert.strictEqual(command.queryTimeWindow, '6h');
});

test('parseCommand - CRLF line endings in a multi-line comment', () => {
  const command = parseCommand('sigma integrate all\r\n\r\nTriggered by automation');
  assert.strictEqual(command.action, 'integrate');
  assert.strictEqual(command.allRules, true);
});
