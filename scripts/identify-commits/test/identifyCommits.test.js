import { test } from 'node:test';
import assert from 'node:assert';
import * as identifyModule from '../identify-commits.js';

test('identifyCommits - PR with automation commits', async () => {
  const mockOctokit = {
    paginate: async (endpoint) => {
      // Simulate PR commits: base -> user1 -> automation -> user2 -> automation
      return [
        {
          sha: 'commit1',
          parents: [{ sha: 'base_commit' }],
          commit: { author: { name: 'user1' } }
        },
        {
          sha: 'commit2',
          parents: [{ sha: 'commit1' }],
          commit: { author: { name: 'github-actions[bot]' } }
        },
        {
          sha: 'commit3',
          parents: [{ sha: 'commit2' }],
          commit: { author: { name: 'user2' } }
        },
        {
          sha: 'commit4',
          parents: [{ sha: 'commit3' }],
          commit: { author: { name: 'github-actions[bot]' } }
        }
      ];
    },
    rest: {
      pulls: {
        listCommits: {
          endpoint: {
            merge: (params) => params
          }
        }
      }
    }
  };

  const context = {
    repo: { owner: 'test-owner', repo: 'test-repo' }
  };

  const results = await identifyModule.identifyCommits(
    mockOctokit,
    context,
    123,
    'github-actions[bot]'
  );

  assert.strictEqual(results.baseCommit, 'base_commit');
  assert.strictEqual(results.firstCommit, 'commit1');
  assert.strictEqual(results.lastCommit, 'commit4');
  assert.strictEqual(results.previousCommit, 'commit4');
  assert.strictEqual(results.previousRef, 'commit4');
  assert.strictEqual(results.diffRef, 'commit4');
});

test('identifyCommits - PR without automation commits', async () => {
  const mockOctokit = {
    paginate: async (endpoint) => {
      return [
        {
          sha: 'commit1',
          parents: [{ sha: 'base_commit' }],
          commit: { author: { name: 'user1' } }
        },
        {
          sha: 'commit2',
          parents: [{ sha: 'commit1' }],
          commit: { author: { name: 'user2' } }
        }
      ];
    },
    rest: {
      pulls: {
        listCommits: {
          endpoint: {
            merge: (params) => params
          }
        }
      }
    }
  };

  const context = {
    repo: { owner: 'test-owner', repo: 'test-repo' }
  };

  const results = await identifyModule.identifyCommits(
    mockOctokit,
    context,
    123,
    'github-actions[bot]'
  );

  assert.strictEqual(results.baseCommit, 'base_commit');
  assert.strictEqual(results.firstCommit, 'commit1');
  assert.strictEqual(results.lastCommit, '');
  assert.strictEqual(results.previousCommit, 'base_commit');
  assert.strictEqual(results.previousRef, 'base_commit');
});

test('identifyCommits - PR with single commit', async () => {
  const mockOctokit = {
    paginate: async (endpoint) => {
      return [
        {
          sha: 'commit1',
          parents: [{ sha: 'base_commit' }],
          commit: { author: { name: 'user1' } }
        }
      ];
    },
    rest: {
      pulls: {
        listCommits: {
          endpoint: {
            merge: (params) => params
          }
        }
      }
    }
  };

  const context = {
    repo: { owner: 'test-owner', repo: 'test-repo' }
  };

  const results = await identifyModule.identifyCommits(
    mockOctokit,
    context,
    123,
    'github-actions[bot]'
  );

  assert.strictEqual(results.baseCommit, 'base_commit');
  assert.strictEqual(results.firstCommit, 'commit1');
  assert.strictEqual(results.lastCommit, '');
  assert.strictEqual(results.previousCommit, 'base_commit');
  assert.strictEqual(results.previousRef, 'base_commit');
});

test('identifyCommits - PR with only automation commits', async () => {
  const mockOctokit = {
    paginate: async (endpoint) => {
      return [
        {
          sha: 'commit1',
          parents: [{ sha: 'base_commit' }],
          commit: { author: { name: 'github-actions[bot]' } }
        },
        {
          sha: 'commit2',
          parents: [{ sha: 'commit1' }],
          commit: { author: { name: 'github-actions[bot]' } }
        }
      ];
    },
    rest: {
      pulls: {
        listCommits: {
          endpoint: {
            merge: (params) => params
          }
        }
      }
    }
  };

  const context = {
    repo: { owner: 'test-owner', repo: 'test-repo' }
  };

  const results = await identifyModule.identifyCommits(
    mockOctokit,
    context,
    123,
    'github-actions[bot]'
  );

  assert.strictEqual(results.baseCommit, 'base_commit');
  assert.strictEqual(results.firstCommit, 'commit1');
  assert.strictEqual(results.lastCommit, 'commit2');
  assert.strictEqual(results.previousCommit, 'commit2');
  assert.strictEqual(results.previousRef, 'commit2');
});

test('identifyCommits - PR with commit without parent (edge case)', async () => {
  const mockOctokit = {
    paginate: async (endpoint) => {
      // First commit has no parent (initial commit)
      return [
        {
          sha: 'commit1',
          parents: [],
          commit: { author: { name: 'user1' } }
        },
        {
          sha: 'commit2',
          parents: [{ sha: 'commit1' }],
          commit: { author: { name: 'user2' } }
        }
      ];
    },
    rest: {
      pulls: {
        listCommits: {
          endpoint: {
            merge: (params) => params
          }
        }
      }
    }
  };

  const context = {
    repo: { owner: 'test-owner', repo: 'test-repo' }
  };

  const results = await identifyModule.identifyCommits(
    mockOctokit,
    context,
    123,
    'github-actions[bot]'
  );

  // When first commit has no parent, baseCommit is empty
  // When we process commit2 (which has commit1 as parent), firstCommit gets set to commit2
  assert.strictEqual(results.baseCommit, 'commit1');
  assert.strictEqual(results.firstCommit, 'commit2');
  assert.strictEqual(results.lastCommit, '');
  assert.strictEqual(results.previousCommit, 'commit1');
  assert.strictEqual(results.previousRef, 'commit1');
});

test('identifyCommits - custom actions username', async () => {
  const mockOctokit = {
    paginate: async (endpoint) => {
      return [
        {
          sha: 'commit1',
          parents: [{ sha: 'base_commit' }],
          commit: { author: { name: 'user1' } }
        },
        {
          sha: 'commit2',
          parents: [{ sha: 'commit1' }],
          commit: { author: { name: 'custom-bot' } }
        }
      ];
    },
    rest: {
      pulls: {
        listCommits: {
          endpoint: {
            merge: (params) => params
          }
        }
      }
    }
  };

  const context = {
    repo: { owner: 'test-owner', repo: 'test-repo' }
  };

  const results = await identifyModule.identifyCommits(
    mockOctokit,
    context,
    123,
    'custom-bot'
  );

  assert.strictEqual(results.baseCommit, 'base_commit');
  assert.strictEqual(results.firstCommit, 'commit1');
  assert.strictEqual(results.lastCommit, 'commit2');
  assert.strictEqual(results.previousCommit, 'commit2');
  assert.strictEqual(results.previousRef, 'commit2');
});

test('identifyCommits - throws when the API returns no commits', async () => {
  const mockOctokit = {
    paginate: async (endpoint) => {
      return [];
    },
    rest: {
      pulls: {
        listCommits: {
          endpoint: {
            merge: (params) => params
          }
        }
      }
    }
  };

  const context = {
    repo: { owner: 'test-owner', repo: 'test-repo' }
  };

  await assert.rejects(
    () => identifyModule.identifyCommits(mockOctokit, context, 123, 'github-actions[bot]'),
    /No commits were returned for pull request #123/
  );
});


test('identifyCommits - changedFilesFromBase diffs against the PR base', async () => {
  // Regression test for #433: with changed_files_from_base the actions diff against
  // diffRef, which must be the PR base rather than the last automation commit.
  const mockOctokit = {
    paginate: async (endpoint) => {
      return [
        {
          sha: 'commit1',
          parents: [{ sha: 'base_commit' }],
          commit: { author: { name: 'user1' } }
        },
        {
          sha: 'commit2',
          parents: [{ sha: 'commit1' }],
          commit: { author: { name: 'github-actions[bot]' } }
        },
        {
          sha: 'commit3',
          parents: [{ sha: 'commit2' }],
          commit: { author: { name: 'user2' } }
        }
      ];
    },
    rest: {
      pulls: {
        listCommits: {
          endpoint: {
            merge: (params) => params
          }
        }
      }
    }
  };

  const context = {
    repo: { owner: 'test-owner', repo: 'test-repo' }
  };

  const fromBase = await identifyModule.identifyCommits(
    mockOctokit,
    context,
    123,
    'github-actions[bot]',
    true
  );

  assert.strictEqual(fromBase.baseCommit, 'base_commit');
  assert.strictEqual(fromBase.diffRef, 'base_commit');
  // previousRef still points at the last automation commit, for manual-edit detection
  assert.strictEqual(fromBase.previousRef, 'commit2');

  const fromPrevious = await identifyModule.identifyCommits(
    mockOctokit,
    context,
    123,
    'github-actions[bot]',
    false
  );

  assert.strictEqual(fromPrevious.diffRef, 'commit2');
  assert.strictEqual(fromPrevious.previousRef, 'commit2');
});

test('getInputs - reads changed_files_from_base from the environment', () => {
  const saved = {};
  for (const name of ['GITHUB_ACTIONS', 'PULL_REQUEST_NUMBER', 'CHANGED_FILES_FROM_BASE']) {
    saved[name] = process.env[name];
  }
  process.env.GITHUB_ACTIONS = 'true';
  process.env.PULL_REQUEST_NUMBER = '123';

  try {
    process.env.CHANGED_FILES_FROM_BASE = 'true';
    assert.strictEqual(identifyModule.getInputs().changedFilesFromBase, true);

    process.env.CHANGED_FILES_FROM_BASE = 'false';
    assert.strictEqual(identifyModule.getInputs().changedFilesFromBase, false);

    delete process.env.CHANGED_FILES_FROM_BASE;
    assert.strictEqual(identifyModule.getInputs().changedFilesFromBase, false);
  } finally {
    for (const [name, value] of Object.entries(saved)) {
      if (value === undefined) {
        delete process.env[name];
      } else {
        process.env[name] = value;
      }
    }
  }
});
