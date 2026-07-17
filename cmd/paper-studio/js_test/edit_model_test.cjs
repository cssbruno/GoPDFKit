const test = require('node:test');
const assert = require('node:assert/strict');

const model = require('../web/edit-model.js');

  const root = {
  kind: 'document', id: '@report', members: [{node: {
    kind: 'page', id: '@page', members: [
      {node: {kind: 'header', id: '@head', members: []}},
      {node: {kind: 'footer', id: '@foot', members: []}},
      {node: {
      kind: 'body', id: '@body', members: [{node: {
        kind: 'row', id: '@grid', members: [
          {node: {kind: 'paragraph', id: '@left', members: []}},
          {node: {kind: 'image', id: '@art', members: []}},
          {node: {kind: 'table', id: '@ledger', members: [
            {node: {kind: 'table-track', id: '@track', members: []}},
            {node: {kind: 'cell', id: '@cell', members: []}},
          ]}},
          {node: {kind: 'canvas', id: '@diagram', members: [
            {node: {kind: 'anchor', id: '@badge', members: []}},
          ]}},
        ],
      }}],
    }}],
  }}],
};

test('selection exposes only handles supported by exact source structure', () => {
  const left = model.findSelection(root, '@left');
  assert.deepEqual(model.operations(left), ['text', 'box', 'grid', 'flow']);
  assert.equal(left.parent.id, '@grid');
  assert.deepEqual(model.operations(model.findSelection(root, '@art')), ['image', 'flow']);
  assert.deepEqual(model.operations(model.findSelection(root, '@page')), ['page']);
  assert.deepEqual(model.operations(model.findSelection(root, '@badge')), ['canvas']);
  assert.deepEqual(model.operations(model.findSelection(root, '@head')), ['region']);
  assert.deepEqual(model.properties(model.findSelection(root, '@track'), 'table'), ['width', 'min-width', 'max-width']);
  assert.equal(model.findSelection(root, '@missing'), null);
});

test('image and table payloads stay typed and selection-specific', () => {
  const workspace = {source_revision: 'source-digest', revision: 'plan-hash'};
  assert.deepEqual(model.buildPayload(workspace, model.findSelection(root, '@art'), 'image', 'focus-x', '0.75'), {
    source_revision: 'source-digest', plan_revision: 'plan-hash', scenario: '', operation: 'image', target: '@art', property: 'focus-x', number: 0.75,
  });
  assert.equal(model.buildPayload(workspace, model.findSelection(root, '@art'), 'image', 'decorative', 'false').bool, false);
  assert.equal(model.buildPayload(workspace, model.findSelection(root, '@ledger'), 'table', 'split', 'avoid').split, 'avoid');
  assert.equal(model.buildPayload(workspace, model.findSelection(root, '@cell'), 'table', 'header', 'true').bool, true);
  assert.throws(() => model.buildPayload(workspace, model.findSelection(root, '@track'), 'table', 'header', 'true'), /unavailable/);
  assert.equal(model.buildPayload(workspace, model.findSelection(root, '@page'), 'page', 'margin-left', '18').points, 18);
  assert.deepEqual(model.buildPayload(workspace, model.findSelection(root, '@badge'), 'canvas', 'left', '@left.right + 8pt'), {
    source_revision: 'source-digest', plan_revision: 'plan-hash', scenario: '', operation: 'canvas', target: '@badge', property: 'left',
    text: '@left', kind: 'right', points: 8,
  });
});

test('payload contains review facts and semantic intent but no capabilities', () => {
  const workspace = {source_revision: 'source-digest', revision: 'plan-hash', scenario: '@print'};
  const selection = model.findSelection(root, '@left');
  assert.deepEqual(model.buildPayload(workspace, selection, 'grid', 'track-size', '48.25'), {
    source_revision: 'source-digest', plan_revision: 'plan-hash', scenario: '@print',
    operation: 'grid', target: '@left', property: 'track-size', points: 48.25,
  });
  const encoded = JSON.stringify(model.buildPayload(workspace, selection, 'box', 'background', '#AABBCC'));
  assert.equal(encoded.includes('capability'), false);
  assert.equal(encoded.includes('handle'), false);
  assert.equal(JSON.parse(encoded).color, '#aabbcc');
});

test('font replacement is explicit, supported-only, and locates the authored text owner', () => {
  const textRoot = {kind: 'document', id: '@report', span: {start: {line: 1}, end: {line: 8}}, members: [{node: {
    kind: 'paragraph', id: '@copy', span: {start: {line: 4}, end: {line: 7}}, members: [],
  }}]};
  const selection = model.findTextSelectionAtLine(textRoot, 5);
  assert.equal(selection.target, '@copy');
  assert.deepEqual(model.coreFonts, ['Courier', 'Helvetica', 'Times', 'Symbol', 'ZapfDingbats']);
  assert.deepEqual(model.buildPayload({source_revision: 's', revision: 'source-bad'}, selection, 'text', 'font', 'Helvetica'), {
    source_revision: 's', plan_revision: 'source-bad', scenario: '', operation: 'text', target: '@copy', property: 'font', text: 'Helvetica',
  });
  assert.throws(() => model.buildPayload({source_revision: 's', revision: 'source-bad'}, selection, 'text', 'font', 'Unavailable Sans'), /Choose one of/);
});

test('invalid values and structurally unavailable handles fail closed', () => {
  const workspace = {source_revision: 'source-digest', revision: 'plan-hash'};
  const selection = model.findSelection(root, '@left');
  assert.throws(() => model.buildPayload(workspace, selection, 'grid', 'track-weight', '1.5'), /whole number/);
  assert.throws(() => model.buildPayload(workspace, selection, 'box', 'background', 'red'), /six-digit/);
  assert.throws(() => model.buildPayload(workspace, model.findSelection(root, '@art'), 'box', 'padding', '1'), /unavailable/);
  assert.throws(() => model.buildPayload({}, selection, 'box', 'padding', '1'), /revisions/);
});

test('flow handle exposes only exact semantic destinations and emits a parent target', () => {
  const selection = model.findSelection(root, '@left');
  assert.ok(model.operations(selection).includes('flow'));
  assert.deepEqual(model.flowDestinations(selection).map((node) => node.id), ['@body', '@grid']);
  assert.deepEqual(model.buildPayload({source_revision: 's', revision: 'p'}, selection, 'flow', 'destination', '@grid'), {
    source_revision: 's', plan_revision: 'p', scenario: '', operation: 'flow', target: '@left', property: 'destination', new_parent: '@grid',
  });
  assert.throws(() => model.buildPayload({source_revision: 's', revision: 'p'}, selection, 'flow', 'destination', '@art'), /destination/);
});
