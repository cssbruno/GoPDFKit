const test = require('node:test');
const assert = require('node:assert/strict');

const model = require('../web/issue-model.js');

test('formats an issue as readable clipboard text', () => {
  assert.equal(model.format({code: 'PAPER_COMPILE_FONT', message: 'Font is unavailable', start_line: 9, start_column: 5}),
    '[PAPER_COMPILE_FONT] Font is unavailable\nLine 9:5');
  assert.equal(model.format({stage: 'compile', message: 'Failed'}), '[compile] Failed\ncompile');
  assert.throws(() => model.format(null), /required/);
});

test('groups compiler issues into exact source-line annotations', () => {
  const issues = [
    {code: 'PAPER_PARSE_VALUE', message: 'Expected a value', severity: 'error', start_line: 4, start_column: 7},
    {code: 'PAPER_PARSE_TOKEN', message: 'Unexpected token', severity: 'warning', start_line: 4, start_column: 9},
    {code: 'NO_LOCATION', message: 'Global failure'},
    {code: 'OUTSIDE', message: 'Outside source', start_line: 12},
  ];
  const annotations = model.sourceAnnotations(issues, 8);
  assert.equal(annotations.length, 1);
  assert.equal(annotations[0].line, 4);
  assert.equal(annotations[0].severity, 'error');
  assert.equal(annotations[0].label, 'PAPER_PARSE_VALUE · Expected a value · +1');
  assert.match(annotations[0].title, /Unexpected token/);
  assert.equal(annotations[0].issues.length, 2);
  assert.throws(() => model.sourceAnnotations([], -1), /line count/);
});
