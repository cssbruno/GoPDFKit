const test = require('node:test');
const assert = require('node:assert/strict');

const gate = require('../web/mutation-gate.js');

const workspace = {revision: 'plan-2', source_revision: 'source-2'};

test('revision gate fails closed while preview metadata is stale or incomplete', () => {
  assert.equal(gate.revisionsLocked(workspace, 'plan-2', 'source-2', false), false);
  assert.equal(gate.revisionsLocked(workspace, 'plan-1', 'source-2', false), true);
  assert.equal(gate.revisionsLocked(workspace, 'plan-2', 'source-1', false), true);
  assert.equal(gate.revisionsLocked(workspace, 'plan-2', 'source-2', true), true);
  assert.equal(gate.revisionsLocked(null, 'plan-2', 'source-2', false), true);
});

test('visual mutation gate also blocks while a commit is in flight', () => {
  assert.equal(gate.visualMutationsLocked(workspace, 'plan-2', 'source-2', false, false), false);
  assert.equal(gate.visualMutationsLocked(workspace, 'plan-2', 'source-2', false, true), true);
  assert.equal(gate.visualMutationsLocked(workspace, 'plan-1', 'source-2', false, false), true);
});
