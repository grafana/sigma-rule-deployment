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

