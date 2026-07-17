const test = require('node:test');
const assert = require('node:assert/strict');

const model = require('../web/inspection-model.js');

test('projects exact page-local baselines without changing geometry', () => {
  const marks = model.baselineMarks([
    {page: 2, line: {fragment: 7, index: 0, bounds: {x: 10, y: 20, width: 80, height: 12}, baseline: 29}},
    {page: 1, line: {fragment: 3, index: 1, bounds: {x: 4, y: 5, width: 10, height: 8}, baseline: 11}},
  ], 2);
  assert.deepEqual(marks, [{
    index: 0,
    fragment: 7,
    rect: {x: 10, y: 29, width: 80, height: 0},
    label: 'baseline 1',
  }]);
});

test('rejects malformed or out-of-bounds baseline evidence', () => {
  assert.deepEqual(model.baselineMarks([
    {page: 1, line: {index: 0, bounds: {x: 0, y: 10, width: 10, height: 4}, baseline: 20}},
    {page: 1, line: {index: 1, bounds: {x: 0, y: 10, width: 10, height: 4}}},
  ], 1), []);
});

test('projects and deduplicates exact semantic table-cell ownership', () => {
  const rect = {x: 10, y: 20, width: 80, height: 12};
  const marks = model.tableCellMarks([
    {id: 1, page: 2, border_box: rect, semantic_ownership: {owner: 8, cell: 4, table_header: true}},
    {id: 2, page: 2, border_box: rect, semantic_ownership: {owner: 9, cell: 4, table_header: true}},
    {id: 3, page: 1, border_box: rect, semantic_ownership: {owner: 10, cell: 5}},
  ], 2);
  assert.deepEqual(marks, [{cell: 4, fragment: 1, rect, tableHeader: true, label: 'table header cell'}]);
});

test('projects exact page-local retained grid tracks', () => {
  const marks = model.gridTrackMarks([
    {index: 4, track: {group: 2, page: 1, axis: 'column', index: 0, bounds: {x: 10, y: 20, width: 30, height: 12}, gap_after: 8}},
    {index: 5, track: {group: 2, page: 1, axis: 'row', index: 0, bounds: {x: 10, y: 20, width: 68, height: 12}}},
    {index: 6, track: {group: 3, page: 2, axis: 'column', index: 0, bounds: {x: 10, y: 20, width: 30, height: 12}}},
  ], 1);
  assert.deepEqual(marks, [
    {index: 4, group: 2, axis: 'column', trackIndex: 0, gapAfter: 8, rect: {x: 10, y: 20, width: 30, height: 12}, label: 'column track 1 · grid 2'},
    {index: 5, group: 2, axis: 'row', trackIndex: 0, gapAfter: 0, rect: {x: 10, y: 20, width: 68, height: 12}, label: 'row track 1 · grid 2'},
  ]);
});

test('rejects malformed retained grid tracks', () => {
  assert.deepEqual(model.gridTrackMarks([
    {track: {group: 1, page: 1, axis: 'column', index: 0, bounds: {x: 0, y: 0, width: 0, height: 10}}},
    {track: {group: 1, page: 1, axis: 'diagonal', index: 0, bounds: {x: 0, y: 0, width: 10, height: 10}}},
  ], 1), []);
});

test('projects exact retained page-master regions', () => {
  const marks = model.pageRegionMarks([
    {index: 0, region: {page: 1, region: 'header', master: 'first', bounds: {x: 6, y: 4, width: 168, height: 10}}},
    {index: 1, region: {page: 1, region: 'body', bounds: {x: 6, y: 14, width: 168, height: 70}}},
    {index: 2, region: {page: 2, region: 'body', bounds: {x: 6, y: 14, width: 168, height: 70}}},
  ], 1);
  assert.deepEqual(marks, [
    {index: 0, region: 'header', master: 'first', rect: {x: 6, y: 4, width: 168, height: 10}, label: 'header region · first'},
    {index: 1, region: 'body', master: '', rect: {x: 6, y: 14, width: 168, height: 70}, label: 'body region'},
  ]);
});

test('projects exact nested fragment box-model layers', () => {
  const margin = {x: 10, y: 20, width: 100, height: 60};
  const border = {x: 14, y: 23, width: 92, height: 54};
  const padding = {x: 16, y: 25, width: 88, height: 50};
  const content = {x: 22, y: 31, width: 76, height: 38};
  assert.deepEqual(model.boxModelMarks([
    {id: 4, page: 2, margin_box: margin, border_box: border, padding_box: padding, content_box: content},
    {id: 5, page: 1, margin_box: margin, border_box: border, padding_box: padding, content_box: content},
  ], 2), [{fragment: 4, margin, border, padding, content}]);
});

test('rejects malformed or non-nested fragment box-model layers', () => {
  assert.deepEqual(model.boxModelMarks([
    {id: 1, page: 1, margin_box: {x: 0, y: 0, width: 10, height: 10}, border_box: {x: -1, y: 0, width: 10, height: 10}, padding_box: {x: 0, y: 0, width: 8, height: 8}, content_box: {x: 1, y: 1, width: 6, height: 6}},
    {id: 2, page: 1, margin_box: {x: 0, y: 0, width: NaN, height: 10}},
  ], 1), []);
});

test('projects exact overflow, clip, and collision evidence', () => {
  const marks = model.issueMarks({
    fragments: [
      {id: 1, page: 1, border_box: {x: 0, y: 0, width: 20, height: 20}},
      {id: 2, page: 1, border_box: {x: 15, y: 10, width: 20, height: 20}},
    ],
    commands: [{index: 7, page: 1, command: {kind: 'clip', bounds: {x: 2, y: 3, width: 8, height: 9}}}],
    images: [{command_index: 7, image: {crop: {clip: {x: 4, y: 5, width: 6, height: 7}}}}],
    diagnostics: [
      {diagnostic: {code: 'CANVAS_NODE_OVERFLOW', location: {page: 1, has_bounds: true, bounds: {x: 18, y: 18, width: 8, height: 8}}}},
    ],
  }, 1);
  assert.equal(marks.overflow.length, 1);
  assert.deepEqual(marks.clips.map((mark) => mark.label), ['clip', 'image clip']);
  assert.deepEqual(marks.collisions, [{rect: {x: 15, y: 10, width: 5, height: 10}, label: 'collision · 1/2'}]);
});

test('issue projection rejects containment, page mismatches, and unpositioned diagnostics', () => {
  const marks = model.issueMarks({
    fragments: [
      {id: 1, page: 1, border_box: {x: 0, y: 0, width: 20, height: 20}},
      {id: 2, page: 1, border_box: {x: 2, y: 2, width: 4, height: 4}},
    ],
    diagnostics: [{diagnostic: {code: 'FONT_MISSING', location: {page: 2, has_bounds: false}}}],
  }, 1);
  assert.deepEqual(marks, {overflow: [], clips: [], collisions: []});
});
