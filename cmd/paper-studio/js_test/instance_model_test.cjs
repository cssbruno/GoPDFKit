const test = require('node:test');
const assert = require('node:assert/strict');

const model = require('../web/instance-model.js');

test('classifies authored, expanded, and repeated fragments from retained identity', () => {
  const classified = model.classifyFragments([
    {id: 1, region: 'body', source_identity: {key: '@title', instance: '@title'}},
    {id: 2, region: 'body', source_identity: {key: '@row', instance: '@row/item-a'}},
    {id: 3, region: 'body', source_identity: {key: '@row', instance: '@row/item-b'}},
    {id: 4, region: 'header', repeated: true, source_identity: {key: '@head', instance: '@head'}},
  ]);

  assert.deepEqual(classified.map(({kind}) => kind), ['authored', 'expanded', 'expanded', 'repeated']);
  assert.equal(classified[1].label, 'instance · @row/item-a');
  assert.equal(classified[3].label, 'repeated header · @head');
  assert.equal(classified[3].className, 'is-instance is-instance-repeated');
});

test('fails closed to authored when optional identity evidence is absent', () => {
  const [classified] = model.classifyFragments([{id: 9, region: 'footer'}]);
  assert.equal(classified.kind, 'authored');
  assert.equal(classified.label, 'authored · fragment-9');
});
