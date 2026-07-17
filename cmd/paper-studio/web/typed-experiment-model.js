(function (root, factory) {
  const model = factory();
  if (typeof module === 'object' && module.exports) module.exports = model;
  else root.PaperStudioTypedExperimentModel = model;
})(typeof globalThis !== 'undefined' ? globalThis : this, function () {
  function breakLabel(decision) {
    if (!decision) return 'break';
    const reason = String(decision.reason || 'break').replaceAll('_', ' ');
    return `${decision.from_page || '?'} → ${decision.to_page || '?'} · ${reason}`;
  }

  function normalize(payload, revision = '') {
    const projection = payload?.projection || {};
    return {
      revision: String(payload?.revision || revision),
      sourceRevision: String(payload?.source_revision || ''),
      inventoryHash: String(projection.inventory_hash || ''),
      fixtures: (projection.fixtures || []).map((fixture) => ({
        name: String(fixture?.name || 'unnamed'),
        outcome: String(fixture?.outcome || 'unknown'),
        pages: Number.isFinite(fixture?.pages) ? fixture.pages : 0,
        planHash: String(fixture?.plan_hash || ''),
        rasterStatus: String(fixture?.raster_status || 'not-applicable'),
        breaks: (fixture?.break_ledger || []).map((decision) => ({
          label: breakLabel(decision),
          reason: String(decision?.reason || 'break'),
          fromPage: decision?.from_page || 0,
          toPage: decision?.to_page || 0,
        })),
      })),
    };
  }

  function summary(experiments) {
    const fixtures = experiments?.fixtures || [];
    const planned = fixtures.filter((fixture) => fixture.outcome === 'planned').length;
    const rejected = fixtures.filter((fixture) => ['rejected', 'unsupported', 'resource-limit'].includes(fixture.outcome)).length;
    return {total: fixtures.length, planned, rejected};
  }

  return {breakLabel, normalize, summary};
});
