const test = require('node:test');
const assert = require('node:assert/strict');

const model = require('../web/page-setup-model.js');

const workspace = {
  revision: 'plan', source_revision: 'source',
  ast: {root: {members: [{node: {kind: 'page', id: '@sheet', members: [
    {property: {name: 'width', value: {unit_value: {number: 210, unit: 'mm'}}}},
    {property: {name: 'height', value: {unit_value: {number: 297, unit: 'mm'}}}},
  ]}}]}},
};

test('recognizes page presets and resolves orientation', () => {
  const current = model.dimensions(workspace);
  assert.equal(current.preset, 'A4');
  assert.equal(current.orientation, 'portrait');
  assert.deepEqual(model.resolvedPoints({preset: 'A3', orientation: 'landscape'}), {width: model.presets.A3[1], height: model.presets.A3[0]});
  for (const preset of ['A5','A6','B5','Executive','Tabloid','Ledger','DL Envelope','C5 Envelope','4×6 Label']) {
    const resolved = model.resolvedPoints({preset, orientation: 'portrait'});
    assert.ok(resolved.width > 0 && resolved.height >= resolved.width, preset);
  }
});

test('builds exact preset and custom page-size payloads', () => {
  const preset = model.buildPayload(workspace, {preset: 'Letter', orientation: 'portrait'});
  assert.equal(preset.target, '@sheet');
  assert.equal(preset.width_points, 612);
  assert.equal(preset.height_points, 792);
  const custom = model.buildPayload(workspace, {preset: 'Custom', orientation: 'landscape', width: 100, height: 200, unit: 'mm'});
  assert.ok(custom.width_points > custom.height_points);
  assert.throws(() => model.resolvedPoints({preset: 'Custom', width: 0, height: 20, unit: 'mm'}), /between/);
});
