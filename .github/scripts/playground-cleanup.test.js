'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const {cleanupPlaygrounds, playgroundCapacity} = require('./playground-cleanup.js');

const repository = 'reeveops/reeve-test';
const expired = '2026-09-17T00:00:00.000Z';
const future = '2026-09-19T00:00:00.000Z';

function playground(number, {expires = expired, run = number, owned = true} = {}) {
  return {
    number,
    created_at: '2026-09-16T00:00:00.000Z',
    body: `<!-- reeve:playground-session:v1 -->\n<!-- reeve:playground-expires-at:${expires} -->\n<!-- reeve:playground-run:${run} -->`,
    labels: [{name: 'reeve-playground'}],
    base: {ref: 'master'},
    head: {ref: `reeve-playground/${number}`, repo: {full_name: repository}},
    user: {login: owned ? 'reeve-e2e-author[bot]' : 'someone-else'}
  };
}

function fixture() {
  const closed = [];
  const deleted = [];
  const runs = new Map([[1, 'in_progress'], [2, 'queued'], [3, 'completed']]);
  const pulls = [playground(1), playground(2), playground(3), playground(4, {expires: future}), playground(5, {owned: false})];
  const github = {
    paginate: async (method, args) => method(args),
    rest: {
      pulls: {
        list: async () => pulls,
        listFiles: async () => [{filename: 'playground/opentofu.yaml'}],
        update: async ({pull_number, headers}) => {
          assert.equal(headers.authorization, 'Bearer author-token');
          closed.push(pull_number);
        }
      },
      actions: {
        getWorkflowRun: async ({run_id}) => ({data: {id: run_id, status: runs.get(run_id)}}),
        listWorkflowRuns: async () => []
      },
      git: {deleteRef: async ({ref, headers}) => {
        assert.equal(headers.authorization, 'Bearer author-token');
        deleted.push(ref);
      }}
    }
  };
  const summary = {addHeading() { return this; }, addTable() { return this; }, async write() {}};
  const core = {warning() {}, error() {}, setFailed(message) { throw new Error(message); }, summary};
  return {github, core, closed, deleted};
}

test('cleanup preserves active, queued, fresh, and unrelated sessions', async () => {
  const f = fixture();
  const result = await cleanupPlaygrounds({
    github: f.github,
    authorToken: 'author-token',
    context: {repo: {owner: 'reeveops', repo: 'reeve-test'}},
    core: f.core,
    now: Date.parse('2026-09-18T00:00:00.000Z')
  });

  assert.deepEqual(f.closed, [3]);
  assert.deepEqual(f.deleted, ['heads/reeve-playground/3']);
  assert.deepEqual(result, {discovered: 5, closed: 1, deleted: 1, active: 2, fresh: 1, skipped: 1, failed: 0});
});

test('playground workflow queues more than one pending session', () => {
  const workflow = fs.readFileSync(path.join(__dirname, '..', 'workflows', 'playground.yml'), 'utf8');
  assert.match(workflow, /concurrency:\n  group: reeve-playground-session\n  queue: max\n  cancel-in-progress: false/);
  assert.equal(playgroundCapacity([{status: 'in_progress'}, {status: 'queued'}]), true);
  assert.equal(playgroundCapacity(Array.from({length: 100}, () => ({status: 'queued'}))), false);
});

test('playground launch requests queue without displacing one another', () => {
  const workflow = fs.readFileSync(path.join(__dirname, '..', 'workflows', 'playground-start.yml'), 'utf8');
  assert.match(workflow, /concurrency:\n  group: playground-start\n  queue: max\n  cancel-in-progress: false/);
});
