(function (root, factory) {
  const model = factory();
  if (typeof module === 'object' && module.exports) module.exports = model;
  else root.PaperStudioReviewModel = model;
})(typeof globalThis !== 'undefined' ? globalThis : this, function () {
  'use strict';

  const palette = Object.freeze([
    {kind: 'paragraph', label: 'Paragraph', parents: ['body', 'row', 'column'], template: 'paragraph'},
    {kind: 'heading', label: 'Heading', parents: ['body', 'row', 'column'], template: 'heading'},
    {kind: 'list', label: 'List', parents: ['body', 'row', 'column'], template: 'list'},
    {kind: 'row', label: 'Row', parents: ['body', 'row', 'column'], template: 'row'},
    {kind: 'column', label: 'Column', parents: ['body', 'row', 'column'], template: 'column'},
    {kind: 'page-break', label: 'Page break', parents: ['body', 'row', 'column'], template: 'page-break'},
    {kind: 'component', label: 'Component instance', parents: ['body', 'row', 'column'], template: 'component'},
  ]);

  function value(member) {
    const scalar = member?.property?.value;
    if (!scalar) return null;
    if (scalar.string_value !== undefined) return String(scalar.string_value);
    if (scalar.bool_value !== undefined) return Boolean(scalar.bool_value);
    if (scalar.number_value !== undefined) return Number(scalar.number_value);
    return scalar.raw ?? null;
  }

  function property(node, name) {
    for (const member of node?.members || []) {
      if (member.property?.name === name) return value(member);
    }
    return null;
  }

  function walk(node, parent = null, output = []) {
    if (!node) return output;
    output.push({node, parent});
    for (const member of node.members || []) if (member.node) walk(member.node, node, output);
    return output;
  }

  function describe(node, root) {
    if (!node) return null;
    const bind = property(node, 'bind');
    let mode = 'authored';
    let title = 'Authored value';
    let detail = 'Edit the source node directly.';
    if (bind) {
      mode = 'binding';
      title = 'Dynamic binding';
      detail = `Fixture values come from ${bind}; edit the selected scenario value, not the rendered text.`;
    } else if (['value', 'object', 'keyed-list', 'scenario'].includes(node.kind)) {
      mode = 'fixture';
      title = 'Scenario fixture';
      detail = 'This is data input. Visual edits do not rewrite the fixture implicitly.';
    } else if (['repeat', 'loop'].includes(node.kind)) {
      mode = 'template';
      title = 'Repeated template';
      detail = 'Edit the template or choose a scenario fixture; expanded rows are not independent authored targets.';
    } else if (node.kind === 'use') {
      mode = 'invocation';
      title = 'Component invocation';
      detail = 'Edit invocation arguments or fill an explicitly declared slot.';
    } else if (node.kind === 'fill') {
      mode = 'slot';
      title = 'Slot fill';
      detail = 'This content belongs to one component slot and its declared cardinality/type contract.';
    } else if (node.kind === 'component') {
      mode = 'definition';
      title = 'Component definition';
      detail = 'Changes may affect every invocation; review the blast radius before committing.';
    }
    const sameBinding = bind ? walk(root).filter(({node: candidate}) => property(candidate, 'bind') === bind).length : 0;
    return Object.freeze({mode, title, detail, binding: bind || '', sharedCount: sameBinding});
  }

  function acceptedPalette(parentKind, items = palette) {
    return items.filter(item => item.parents.includes(parentKind));
  }

  function dropTargets(root, items = palette) {
    const result = walk(root).filter(({node}) => ['body', 'row', 'column'].includes(node.kind) && node.id).map(({node}) => ({
      target: node.id,
      kind: node.kind,
      accepts: acceptedPalette(node.kind, items).map(item => item.kind),
    }));
    const definitions = new Map(walk(root).filter(({node}) => node.kind === 'component' && node.id).map(({node}) => [node.id, node]));
    for (const {node: use} of walk(root).filter(({node}) => node.kind === 'use' && node.id)) {
      const definition = definitions.get(property(use, 'component'));
      const filled = new Set((use.members || []).filter(member => member.node?.kind === 'fill').map(member => member.node.id));
      for (const {node: slot} of walk(definition).filter(({node}) => node.kind === 'slot' && node.id && !filled.has(node.id))) {
        const type = property(slot, 'type') || 'blocks';
        const accepts = type === 'text' ? ['paragraph', 'heading'] : type === 'list' ? ['list'] : type === 'row-column' ? ['row', 'column'] : acceptedPalette('body', items).map(item => item.kind);
        result.push({target: use.id, slot: slot.id, kind: 'slot', accepts});
      }
    }
    return result;
  }

  function blastRadius(node, root) {
    if (!node) return Object.freeze({scope: 'none', count: 0, targets: []});
    const all = walk(root);
    let targets = [];
    if (node.kind === 'component' && node.id) {
      targets = all.filter(({node: candidate}) => candidate.kind === 'use' && property(candidate, 'component') === node.id).map(({node: candidate}) => candidate.id).filter(Boolean);
    } else if (property(node, 'style')) {
      const style = property(node, 'style');
      targets = all.filter(({node: candidate}) => property(candidate, 'style') === style).map(({node: candidate}) => candidate.id).filter(Boolean);
    } else if (node.kind === 'use') {
      targets = [node.id].filter(Boolean);
    }
    const scope = targets.length > 1 ? 'shared' : targets.length === 1 ? 'local' : 'none';
    return Object.freeze({scope, count: targets.length, targets: Object.freeze(targets)});
  }

  function pageBreakPolicies() {
    return Object.freeze([
      {value: 'hard', label: 'Hard break', detail: 'Always start the following content on a new page.'},
      {value: 'keep-with-next', label: 'Keep with next', detail: 'Move the break decision with the following authored block.'},
      {value: 'avoid-orphan', label: 'Avoid orphan', detail: 'Prefer a legal break that preserves the next block as a unit.'},
    ]);
  }

  function optimisticFeedback(intent) {
    return Object.freeze({
      tone: 'speculative',
      authoritative: false,
      text: `Speculative preview · ${intent || 'exact patch'} pending server confirmation`,
    });
  }

  function normalizeReview(payload, workspace) {
    if (!payload || payload.format_version !== 1 || !workspace || payload.revision !== workspace.revision ||
        payload.source_revision !== workspace.source_revision || payload.plan_hash !== workspace.plan_hash) {
      throw new Error('Review metadata belongs to a stale or unsupported plan');
    }
    return Object.freeze({
      revision: payload.revision,
      sourceRevision: payload.source_revision,
      scenario: String(payload.scenario || ''),
      accessibility: payload.accessibility ? Object.freeze({...payload.accessibility, failures: Object.freeze([...(payload.accessibility.failures || [])])}) : null,
      annotations: Object.freeze((payload.annotations || []).map((item) => Object.freeze({...item, transform: Object.freeze([...(item.transform || [])])}))),
      comments: Object.freeze((payload.comments || []).map((item) => Object.freeze({...item}))),
      reference: payload.reference ? Object.freeze({...payload.reference, diffDigest: String(payload.reference.diff_digest || ''), changedPixels: Number(payload.reference.changed_pixels || 0), transform: Object.freeze([...(payload.reference.transform || [])])}) : null,
    });
  }

  function reviewAnchor(workspace, target, page, rect = {}) {
    if (!workspace?.revision || !workspace?.source_revision || !target || !Number.isInteger(page) || page < 1) throw new Error('Exact review anchor is unavailable');
    return {source_revision: workspace.source_revision, plan_revision: workspace.revision, scenario: workspace.scenario || '', target, page,
      x: Number(rect.x || 0), y: Number(rect.y || 0), width: Number(rect.width || 0), height: Number(rect.height || 0), transform: [1, 0, 0, 1, 0, 0]};
  }

  function commentPayload(workspace, target, page, body, author = '') {
    if (!String(body || '').trim()) throw new Error('Comment body cannot be empty');
    return {...reviewAnchor(workspace, target, page, {x: 8, y: 8}), kind: 'comment', body: String(body).trim(), author: String(author || '').trim()};
  }

  function annotationPayload(workspace, target, page, note = '', rect = {}) {
    return {...reviewAnchor(workspace, target, page, {x: rect.x ?? 8, y: rect.y ?? 8, width: rect.width ?? 28, height: rect.height ?? 12}), kind: 'annotation', label: 'selection', note: String(note || '').trim()};
  }

  return Object.freeze({annotationPayload, palette, acceptedPalette, blastRadius, commentPayload, describe, dropTargets, normalizeReview, pageBreakPolicies, optimisticFeedback});
});
