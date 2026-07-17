const test = require('node:test');
const assert = require('node:assert/strict');

const model = require('../web/tag-model.js');

const workspace = {revision: 'plan', source_revision: 'source'};
const payload = {
  format_version: 1, evidence: 'final_serialized_pdf', plan_revision: 'plan', source_revision: 'source',
  report: {
    version: 1, pdf_sha256: 'a'.repeat(64), passed: true, marked: true,
    structure_root: 9, parent_tree: 8, document_element: 1,
    marked_content: 1, content_marked: 1, artifact_content: 0, content_ends: 1, structure_elements: 2,
    nodes: [
      {object: 1, parent: 9, role: 'Document', depth: 0, children: 1, marked_content: 0},
      {object: 2, parent: 1, role: 'P', depth: 1, children: 0, marked_content: 1},
    ],
  },
};

test('normalizes exact final-PDF tag evidence without plan-role inference', () => {
  const result = model.normalize(payload, workspace);
  assert.equal(result.passed, true);
  assert.deepEqual(result.rows.map(row => [row.role, row.depth]), [['Document', 0], ['P', 1]]);
  assert.equal(result.markedContent, 1);
});

test('rejects stale, malformed, and internally inconsistent tag evidence', () => {
  assert.throws(() => model.normalize({...payload, plan_revision: 'stale'}, workspace), /stale/);
  assert.throws(() => model.normalize({...payload, report: {...payload.report, pdf_sha256: 'bad'}}, workspace), /malformed/);
  assert.throws(() => model.normalize({...payload, report: {...payload.report, nodes: [payload.report.nodes[0], {...payload.report.nodes[1], parent: 99}]}}, workspace), /malformed/);
  assert.throws(() => model.normalize({...payload, report: {...payload.report, content_marked: 0}}, workspace), /lacks required/);
});
