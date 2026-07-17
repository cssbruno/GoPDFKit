(function (root, factory) {
  const model = factory();
  if (typeof module === 'object' && module.exports) module.exports = model;
  else root.PaperStudioTagModel = model;
})(typeof globalThis !== 'undefined' ? globalThis : this, function () {
  'use strict';

  function normalize(payload, workspace) {
    if (!payload || payload.format_version !== 1 || payload.evidence !== 'final_serialized_pdf' ||
        payload.plan_revision !== workspace?.revision || payload.source_revision !== workspace?.source_revision) {
      throw new Error('Final PDF tag evidence is stale or unsupported');
    }
    const report = payload.report;
    if (!report || report.version !== 1 || !/^[0-9a-f]{64}$/.test(report.pdf_sha256 || '') || !Array.isArray(report.nodes) || report.nodes.length > 100000) {
      throw new Error('Final PDF tag report is malformed');
    }
    const seen = new Set();
    const rows = [];
    for (let index = 0; index < report.nodes.length; index += 1) {
      const node = report.nodes[index];
      if (!Number.isInteger(node.object) || node.object < 1 || seen.has(node.object) || !Number.isInteger(node.depth) || node.depth < 0 || node.depth > 256 ||
          !/^[A-Za-z0-9]{1,64}$/.test(node.role || '') || (index === 0 && (node.role !== 'Document' || node.depth !== 0)) ||
          (index > 0 && node.depth > rows[index - 1].depth + 1) || (node.depth > 0 && !seen.has(node.parent))) {
        throw new Error('Final PDF tag tree is malformed');
      }
      seen.add(node.object);
      rows.push(Object.freeze({
        object: node.object, parent: node.parent, role: node.role, depth: node.depth,
        pageObject: node.page_object || 0, markedContent: node.marked_content || 0, children: node.children || 0,
        hasAlt: Boolean(node.has_alt), hasActualText: Boolean(node.has_actual_text), hasLanguage: Boolean(node.has_language),
      }));
    }
    if (report.passed && (!report.marked || !report.structure_root || !report.parent_tree || !report.document_element || !rows.length ||
        report.structure_elements !== rows.length || report.content_marked !== report.marked_content || report.content_ends !== report.content_marked + report.artifact_content)) {
      throw new Error('Passing tag report lacks required final-PDF evidence');
    }
    return Object.freeze({
      passed: Boolean(report.passed), hash: report.pdf_sha256, marked: Boolean(report.marked),
      markedContent: report.marked_content || 0, contentMarked: report.content_marked || 0,
      structureElements: report.structure_elements || 0, failures: Object.freeze([...(report.failures || [])]), rows: Object.freeze(rows),
    });
  }

  return Object.freeze({normalize});
});
