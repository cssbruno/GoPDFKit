(function (root, factory) {
  const model = factory();
  if (typeof module === 'object' && module.exports) module.exports = model;
  else root.PaperStudioPageSetupModel = model;
})(typeof globalThis !== 'undefined' ? globalThis : this, function () {
  'use strict';

  const presets = Object.freeze({
    A4: Object.freeze([595.275590551, 841.88976378]),
    A3: Object.freeze([841.88976378, 1190.551181102]),
    Letter: Object.freeze([612, 792]),
    Legal: Object.freeze([612, 1008]),
  });
  const unitPoints = Object.freeze({pt: 1, mm: 72 / 25.4, in: 72});

  function pageNode(workspace) {
    const pages = (workspace?.ast?.root?.members || []).map(member => member.node).filter(node => node?.kind === 'page' && node.id);
    if (pages.length !== 1) throw new Error('Page setup requires one addressed page master');
    return pages[0];
  }

  function property(node, name) {
    return (node.members || []).map(member => member.property).find(item => item?.name === name)?.value;
  }

  function lengthPoints(value) {
    const unit = value?.unit_value;
    if (!unit || !unitPoints[unit.unit]) return 0;
    return Number(unit.number) * unitPoints[unit.unit];
  }

  function dimensions(workspace) {
    const page = pageNode(workspace);
    let width = lengthPoints(property(page, 'width'));
    let height = lengthPoints(property(page, 'height'));
    const named = property(page, 'size')?.string_value;
    if ((!width || !height) && presets[named]) [width, height] = presets[named];
    if (!(width > 0 && height > 0)) throw new Error('Page dimensions are unavailable');
    const orientation = width > height ? 'landscape' : 'portrait';
    const short = Math.min(width, height);
    const long = Math.max(width, height);
    const preset = Object.entries(presets).find(([, value]) => Math.abs(Math.min(...value) - short) < .1 && Math.abs(Math.max(...value) - long) < .1)?.[0] || 'Custom';
    return Object.freeze({target: page.id, width, height, preset, orientation});
  }

  function resolvedPoints(draft) {
    const orientation = draft.orientation === 'landscape' ? 'landscape' : 'portrait';
    let width;
    let height;
    if (presets[draft.preset]) {
      [width, height] = presets[draft.preset];
    } else {
      const factor = unitPoints[draft.unit];
      width = Number(draft.width) * factor;
      height = Number(draft.height) * factor;
      if (!factor) throw new Error('Choose pt, mm, or in');
    }
    if (orientation === 'landscape' && width < height || orientation === 'portrait' && width > height) [width, height] = [height, width];
    if (!Number.isFinite(width) || !Number.isFinite(height) || width < 36 || height < 36 || width > 14400 || height > 14400) {
      throw new Error('Page width and height must be between 36pt and 14400pt');
    }
    return {width, height};
  }

  function buildPayload(workspace, draft) {
    if (!workspace?.source_revision || !workspace?.revision) throw new Error('Exact source and plan revisions are unavailable');
    const page = pageNode(workspace);
    const size = resolvedPoints(draft);
    return Object.freeze({
      source_revision: workspace.source_revision, plan_revision: workspace.revision, scenario: workspace.scenario || '',
      operation: 'page-size', target: page.id, property: 'size', width_points: size.width, height_points: size.height,
    });
  }

  return Object.freeze({presets, dimensions, resolvedPoints, buildPayload});
});
