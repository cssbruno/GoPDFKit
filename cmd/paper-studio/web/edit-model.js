(function (root, factory) {
  const model = factory();
  if (typeof module === 'object' && module.exports) module.exports = model;
  else root.PaperStudioEditModel = model;
})(typeof globalThis !== 'undefined' ? globalThis : this, function () {
  'use strict';

  const boxProperties = Object.freeze([
    'padding', 'padding-top', 'padding-right', 'padding-bottom', 'padding-left',
    'border-width', 'border-top-width', 'border-right-width', 'border-bottom-width', 'border-left-width',
    'border-radius', 'border-color', 'background',
  ]);
  const gridProperties = Object.freeze([
    'track', 'track-size', 'track-min', 'track-max', 'track-weight', 'track-grow', 'track-shrink',
    'cross-size', 'cross-min', 'cross-max', 'cross-align',
  ]);
  const boxKinds = new Set(['paragraph', 'heading', 'list']);
  const coreFonts = Object.freeze(['Courier', 'Helvetica', 'Times', 'Symbol', 'ZapfDingbats']);
  const gridKinds = new Set(['paragraph', 'heading', 'image', 'table']);
  const gridParents = new Set(['row', 'column']);
  const imageProperties = Object.freeze(['fit', 'focus-x', 'focus-y', 'width', 'height', 'max-width', 'max-height', 'alt', 'decorative']);
  const canvasProperties = Object.freeze(['left', 'right', 'center-x', 'top', 'bottom', 'center-y']);
  const flowKinds = new Set(['heading', 'paragraph', 'list', 'page-break', 'text', 'row', 'column', 'image', 'table', 'canvas', 'use', 'repeat', 'loop']);
  const flowParents = new Set(['body', 'row', 'column']);

  function findSelection(root, target) {
    let result = null;
    let matches = 0;
    function walk(node, parent) {
      if (!node) return;
      if (node.id === target) {
        matches += 1;
        result = {target, node, parent, root};
      }
      for (const member of node.members || []) walk(member.node, node);
    }
    walk(root, null);
    return matches === 1 ? result : null;
  }

  function findTextSelectionAtLine(root, line) {
    let result = null;
    const wanted = Number(line);
    function walk(node) {
      if (!node) return;
      const start = Number(node.span?.start?.line || 0);
      const end = Number(node.span?.end?.line || start);
      if (boxKinds.has(node.kind) && node.id && start > 0 && wanted >= start && wanted <= end) {
        result = findSelection(root, node.id);
      }
      for (const member of node.members || []) walk(member.node);
    }
    walk(root);
    return result;
  }

  function operations(selection) {
    if (!selection) return [];
    const result = [];
    if (boxKinds.has(selection.node.kind)) result.push('text');
    if (boxKinds.has(selection.node.kind)) result.push('box');
    if (gridKinds.has(selection.node.kind) && gridParents.has(selection.parent?.kind) && selection.parent?.id) result.push('grid');
    if (selection.node.kind === 'image') result.push('image');
    if (['table', 'table-track', 'cell'].includes(selection.node.kind)) result.push('table');
    if (selection.node.kind === 'page') result.push('page');
    if (selection.node.kind === 'anchor' && selection.parent?.kind === 'canvas' && selection.parent?.id) result.push('canvas');
    if (['header', 'footer'].includes(selection.node.kind) && selection.parent?.kind === 'page' && selection.parent?.id) result.push('region');
    if (flowKinds.has(selection.node.kind) && flowDestinations(selection).length) result.push('flow');
    return result;
  }

  function properties(operation) {
    if (operation === 'text') return ['font'];
    if (operation === 'box') return boxProperties;
    if (operation === 'grid') return gridProperties;
    if (operation === 'image') return imageProperties;
    if (operation === 'table') {
      return [];
    }
    if (operation === 'page') return ['margin', 'margin-top', 'margin-right', 'margin-bottom', 'margin-left'];
    if (operation === 'canvas') return canvasProperties;
    if (operation === 'region') return ['padding', 'border-width', 'border-radius', 'border-color', 'background'];
    if (operation === 'flow') return ['destination'];
    return [];
  }

  function valueSpec(operation, property) {
    if (operation === 'text' && property === 'font') return {kind: 'choice', label: 'Replacement font', choices: coreFonts, field: 'text'};
    if ((operation === 'box' || operation === 'region') && ['border-color', 'background'].includes(property)) return {kind: 'color', label: 'Color'};
    if (operation === 'grid' && property === 'track') return {kind: 'choice', label: 'Track', choices: ['fixed', 'auto', 'fraction', 'flex']};
    if (operation === 'grid' && property === 'track-weight') return {kind: 'integer', label: 'Weight', min: 1, max: 4294967295};
    if (operation === 'grid' && ['track-grow', 'track-shrink'].includes(property)) return {kind: 'number', label: 'Flex factor', min: 0, max: 4294.967295, step: 0.1, field: 'number'};
    if (operation === 'grid' && ['track-size', 'cross-size'].includes(property)) return {kind: 'length', label: 'Size (auto, %, or pt)'};
    if (operation === 'grid' && ['track-min', 'track-max', 'cross-min', 'cross-max'].includes(property)) return {kind: 'length', label: 'Constraint (auto, %, or pt)', positive: false};
    if (operation === 'grid' && property === 'cross-align') return {kind: 'choice', label: 'Cross alignment', choices: ['start', 'center', 'end', 'stretch']};
    if (operation === 'image' && property === 'fit') return {kind: 'choice', label: 'Fit', choices: ['auto', 'contain', 'cover']};
    if (operation === 'image' && ['focus-x', 'focus-y'].includes(property)) return {kind: 'number', label: 'Ratio', min: 0, max: 1, step: 0.05, field: 'number'};
    if (operation === 'image' && ['width', 'max-width'].includes(property)) return {kind: 'length', label: 'Size (auto, %, or pt)', allowPercent: true};
    if (operation === 'image' && ['height', 'max-height'].includes(property)) return {kind: 'length', label: 'Size (auto or pt)', allowPercent: false};
    if (operation === 'image' && property === 'alt') return {kind: 'text', label: 'Alt text'};
    if ((operation === 'image' && property === 'decorative') || (operation === 'table' && ['repeat-header', 'header'].includes(property))) return {kind: 'boolean', label: 'Value', choices: ['true', 'false']};
    if (operation === 'table' && property === 'split') return {kind: 'choice', label: 'Split', choices: ['rows', 'avoid'], field: 'split'};
    if (operation === 'table' && ['width', 'min-width', 'max-width'].includes(property)) return {kind: 'length', label: 'Size (auto, %, or pt)'};
    if (operation === 'canvas') return {kind: 'constraint', label: 'Target anchor'};
    if (operation === 'flow') return {kind: 'text', label: 'Destination @id'};
    return {kind: 'number', label: 'Points', min: 0, max: 1000000, step: 0.25};
  }

  function buildPayload(workspace, selection, operation, property, rawValue) {
    if (!workspace?.source_revision || !workspace?.revision) throw new Error('Exact source and plan revisions are unavailable');
    if (!operations(selection).includes(operation) || !propertiesForSelection(selection, operation).includes(property)) throw new Error('Handle is unavailable for this selection');
    const payload = {
      source_revision: workspace.source_revision,
      plan_revision: workspace.revision,
      scenario: workspace.scenario || '',
      operation,
      target: selection.target,
      property,
    };
    const spec = valueSpec(operation, property);
    if (operation === 'flow') {
      const destination = String(rawValue || '').trim();
      if (!/^@[A-Za-z][A-Za-z0-9_-]*$/.test(destination) || !flowDestinations(selection).some((node) => node.id === destination)) {
        throw new Error('Choose an existing body, row, or column destination');
      }
      payload.new_parent = destination;
    } else if (spec.kind === 'constraint') {
      const match = String(rawValue || '').trim().match(/^(canvas|@[A-Za-z][A-Za-z0-9_-]*)\.(left|right|center-x|top|bottom|center-y)(?:\s*([+-])\s*(\d+(?:\.\d+)?)pt)?$/);
      if (!match) throw new Error('Use canvas.left or @sibling.right + 8pt');
      payload.text = match[1];
      payload.kind = match[2];
      if (match[3]) payload.points = Number(match[4]) * (match[3] === '-' ? -1 : 1);
    } else if (spec.kind === 'length') {
      const length = String(rawValue || '').trim().toLowerCase();
      if (length === 'auto') payload.length = length;
      else {
        const match = length.match(/^(\d+(?:\.\d+)?)(%|pt)?$/);
        if (!match) throw new Error('Use auto, a percentage such as 50%, or a physical size such as 48pt');
        const value = Number(match[1]);
        const unit = match[2] || 'pt';
        if (unit === '%' && spec.allowPercent === false) throw new Error('Percentage height needs a definite container height; use auto or pt');
        const maximum = unit === '%' ? 100 : 1000000;
        if (!Number.isFinite(value) || value < 0 || (spec.positive !== false && value === 0) || value > maximum) throw new Error(`Use ${spec.positive === false ? 'a non-negative' : 'a positive'} ${unit === '%' ? 'percentage up to 100%' : 'physical size'}`);
        if (match[2]) payload.length = `${value}${unit}`;
        else payload.points = value;
      }
    } else if (spec.kind === 'color') {
      const color = String(rawValue || '').toLowerCase();
      if (!/^#[0-9a-f]{6}$/.test(color)) throw new Error('Use a six-digit color such as #315ee8');
      payload.color = color;
    } else if (spec.kind === 'choice') {
      const choice = String(rawValue || '').trim();
      if (!spec.choices.includes(choice)) throw new Error(`Choose one of: ${spec.choices.join(', ')}`);
      payload[spec.field || 'kind'] = choice;
    } else if (spec.kind === 'text') {
      payload.text = String(rawValue || '');
    } else if (spec.kind === 'boolean') {
      payload.bool = String(rawValue) === 'true';
    } else {
      const value = Number(rawValue);
      if (!Number.isFinite(value) || String(rawValue).trim() === '' || value < spec.min || value > spec.max || (spec.kind === 'integer' && !Number.isInteger(value))) {
        throw new Error(spec.kind === 'integer' ? 'Weight must be a positive whole number' : spec.field === 'number' ? 'Factor must be a finite number in range' : 'Points must be a finite non-negative number');
      }
      if (spec.kind === 'integer') payload.weight = value;
      else if (spec.field === 'number') payload.number = value;
      else payload.points = value;
    }
    return payload;
  }

  function propertiesForSelection(selection, operation) {
    if (operation === 'region') return properties(operation);
    if (operation !== 'table') return properties(operation);
    if (selection?.node?.kind === 'table') return ['split', 'repeat-header'];
    if (selection?.node?.kind === 'table-track') return ['width', 'min-width', 'max-width'];
    if (selection?.node?.kind === 'cell') return ['header'];
    return [];
  }

  function flowDestinations(selection) {
    if (!selection?.root || !flowKinds.has(selection.node?.kind)) return [];
    const descendants = new Set();
    (function mark(node) {
      if (!node) return;
      if (node.id) descendants.add(node.id);
      for (const member of node.members || []) mark(member.node);
    })(selection.node);
    const result = [];
    (function walk(node) {
      if (!node) return;
      if (flowParents.has(node.kind) && node.id && !descendants.has(node.id)) {
        const allowed = node.kind === 'body' || ['heading', 'paragraph', 'use'].includes(selection.node.kind);
        if (allowed) result.push(node);
      }
      for (const member of node.members || []) walk(member.node);
    })(selection.root);
    return result;
  }

  return Object.freeze({coreFonts, findSelection, findTextSelectionAtLine, operations, properties: propertiesForSelection, valueSpec, buildPayload, flowDestinations});
});
