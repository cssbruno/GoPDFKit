const test = require('node:test');
const assert = require('node:assert/strict');

const model = require('../web/provenance-model.js');

test('filters exact binding and token provenance by retained fragment source key', () => {
  const provenance = {
    bindings: [
      {node: '@message', path: '@invoice.total', kind: 'paragraph'},
      {node: '@other', path: '@invoice.note', kind: 'paragraph'},
    ],
    style_tokens: [
      {node: '@message', property: 'size-token', theme: 'print', token: 'body-size', value: '11pt', token_chain: [{theme: 'print', token: 'body-size'}]},
      {node: '@other', property: 'color-token', theme: 'print', token: 'ink', value: '#000000'},
    ],
  };
  assert.deepEqual(model.forFragments(provenance, [{source_identity: {key: '@message'}}]), {
    bindings: [provenance.bindings[0]],
    styleTokens: [provenance.style_tokens[0]],
  });
});

test('keeps anonymous provenance visible for a page-wide inspection', () => {
  const normalized = model.forFragments({bindings: [{path: '@invoice.total'}], style_tokens: [{property: 'size-token'}]}, []);
  assert.equal(normalized.bindings.length, 1);
  assert.equal(normalized.styleTokens.length, 1);
  assert.match(model.bindingLabel(normalized.bindings[0]), /@invoice\.total/);
  assert.match(model.tokenLabel({node: '@message', property: 'size-token', theme: 'print', token: 'size', value: '11pt', token_chain: [{theme: 'print', token: 'size'}, {theme: 'base', token: 'size'}]}), /print:size → base:size/);
});
