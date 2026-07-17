const assert = require('node:assert/strict');
const test = require('node:test');
const model = require('../web/typed-experiment-model.js');

test('normalizes typed experiment outcomes and causal breaks', () => {
  const result = model.normalize({
    revision: 'rev',
    source_revision: 'src',
    projection: {inventory_hash: 'inventory', fixtures: [
      {name: 'page-break', outcome: 'planned', pages: 2, raster_status: 'captured', break_ledger: [{from_page: 1, to_page: 2, reason: 'explicit_page_break'}]},
      {name: 'limit', outcome: 'resource-limit'},
    ]},
  });
  assert.equal(result.revision, 'rev');
  assert.equal(result.fixtures[0].breaks[0].label, '1 → 2 · explicit page break');
  assert.deepEqual(model.summary(result), {total: 2, planned: 1, rejected: 1});
});
