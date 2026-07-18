const test = require('node:test');
const assert = require('node:assert/strict');

const model = require('../web/viewport-model.js');

test('fits physical pages into the available logical viewport', () => {
  const page = {pageWidth: 600, pageHeight: 800, viewportWidth: 900, viewportHeight: 700, paddingInline: 80, paddingBlock: 100};
  assert.equal(model.fitZoom({...page, mode: 'page'}), 0.75);
  assert.equal(model.fitZoom({...page, mode: 'width'}), 1.36);
});

test('keeps fit zoom bounded on very small and very large screens', () => {
  assert.equal(model.fitZoom({pageWidth: 600, pageHeight: 800, viewportWidth: 10, viewportHeight: 10}), model.minZoom);
  assert.equal(model.fitZoom({pageWidth: 100, pageHeight: 100, viewportWidth: 2000, viewportHeight: 2000}), model.maxZoom);
  assert.equal(model.fitZoom({}), 1);
});

test('derives raster DPI from logical zoom and actual device pixel ratio', () => {
  assert.equal(model.renderDPI(1, 1), 72);
  assert.equal(model.renderDPI(1, 2), 144);
  assert.equal(model.renderDPI(0.5, 3), 108);
  assert.equal(model.renderDPI(4, 4), model.maxDPI);
  assert.equal(model.previewDPI(1, 2), 96);
});
