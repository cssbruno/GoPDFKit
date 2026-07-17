(function (root, factory) {
  const model = factory();
  if (typeof module === 'object' && module.exports) module.exports = model;
  else root.PaperStudioInspectionModel = model;
})(typeof globalThis !== 'undefined' ? globalThis : this, function () {
  function baselineMarks(lines, page) {
    return (lines || []).flatMap((entry) => {
      const line = entry?.line;
      const bounds = line?.bounds;
      const baseline = line?.baseline;
      if (entry?.page !== page || !bounds || !Number.isFinite(baseline) ||
          !Number.isFinite(bounds.x) || !Number.isFinite(bounds.y) ||
          !Number.isFinite(bounds.width) || !Number.isFinite(bounds.height) ||
          baseline < bounds.y || baseline > bounds.y + bounds.height) return [];
      return [{
        index: line.index,
        fragment: line.fragment,
        rect: {x: bounds.x, y: baseline, width: bounds.width, height: 0},
        label: `baseline ${line.index + 1}`,
      }];
    });
  }

  function tableCellMarks(fragments, page) {
    const seen = new Set();
    return (fragments || []).flatMap((fragment) => {
      const semantic = fragment?.semantic_ownership;
      const rect = fragment?.border_box;
      if (fragment?.page !== page || !semantic?.cell || !rect) return [];
      const key = `${semantic.cell}\0${rect.x}\0${rect.y}\0${rect.width}\0${rect.height}`;
      if (seen.has(key)) return [];
      seen.add(key);
      return [{
        cell: semantic.cell,
        fragment: fragment.id,
        rect,
        tableHeader: semantic.table_header === true,
        label: semantic.table_header === true ? 'table header cell' : 'table cell',
      }];
    });
  }

  function gridTrackMarks(tracks, page) {
    return (tracks || []).flatMap((entry) => {
      const track = entry?.track;
      const rect = track?.bounds;
      if (track?.page !== page || !['column', 'row'].includes(track?.axis) || !rect ||
          !Number.isFinite(rect.x) || !Number.isFinite(rect.y) ||
          !(rect.width > 0) || !(rect.height > 0)) return [];
      return [{
        index: entry.index,
        group: track.group,
        axis: track.axis,
        trackIndex: track.index,
        gapAfter: track.gap_after || 0,
        rect,
        label: `${track.axis} track ${track.index + 1} · grid ${track.group}`,
      }];
    });
  }

  function pageRegionMarks(regions, page) {
    return (regions || []).flatMap((entry) => {
      const region = entry?.region;
      const rect = region?.bounds;
      if (region?.page !== page || !['header', 'body', 'footer'].includes(region?.region) || !rect ||
          !Number.isFinite(rect.x) || !Number.isFinite(rect.y) || !(rect.width > 0) || !(rect.height > 0)) return [];
      return [{index: entry.index, region: region.region, master: region.master || '', rect,
        label: `${region.region} region${region.master ? ` · ${region.master}` : ''}`}];
    });
  }

  function boxModelMarks(fragments, page) {
    const finiteRect = (rect) => rect && Number.isFinite(rect.x) && Number.isFinite(rect.y) &&
      rect.width >= 0 && rect.height >= 0;
    const contains = (outer, inner) => inner.x >= outer.x && inner.y >= outer.y &&
      inner.x + inner.width <= outer.x + outer.width &&
      inner.y + inner.height <= outer.y + outer.height;
    return (fragments || []).flatMap((fragment) => {
      const margin = fragment?.margin_box;
      const border = fragment?.border_box;
      const padding = fragment?.padding_box;
      const content = fragment?.content_box;
      if (fragment?.page !== page || ![margin, border, padding, content].every(finiteRect) ||
          !contains(margin, border) || !contains(border, padding) || !contains(padding, content)) return [];
      return [{fragment: fragment.id, margin, border, padding, content}];
    });
  }

  function issueMarks(target, page) {
    const marks = {overflow: [], clips: [], collisions: []};
    const finiteRect = (rect) => rect && Number.isFinite(rect.x) && Number.isFinite(rect.y) && rect.width >= 0 && rect.height >= 0;
    const diagnosticEntries = target?.diagnostics || [];
    for (const entry of diagnosticEntries) {
      const diagnostic = entry?.diagnostic || {};
      const location = diagnostic.location || {};
      if (location.page !== page || !location.has_bounds || !finiteRect(location.bounds)) continue;
      const code = String(diagnostic.code || '');
      const evidence = diagnostic.evidence || [];
      if (code.includes('OVERFLOW') || evidence.some((item) => String(item?.key || '').includes('overflow'))) {
        marks.overflow.push({rect: location.bounds, label: code.toLowerCase().replaceAll('_', ' ')});
      }
    }
    for (const entry of target?.commands || []) {
      const command = entry?.command || {};
      if (entry?.page === page && command.kind === 'clip' && finiteRect(command.bounds)) {
        marks.clips.push({rect: command.bounds, label: 'clip'});
      }
    }
    const pageCommands = new Set((target?.commands || []).filter((entry) => entry?.page === page).map((entry) => entry.index));
    for (const entry of target?.images || []) {
	  const image = entry?.image || {};
	  const clip = image?.crop?.clip;
	  if (pageCommands.has(entry?.command_index) && finiteRect(clip)) marks.clips.push({rect: clip, label: 'image clip'});
    }
    const fragments = (target?.fragments || []).filter((fragment) => fragment?.page === page && finiteRect(fragment.border_box));
    const contains = (outer, inner) => inner.x >= outer.x && inner.y >= outer.y && inner.x + inner.width <= outer.x + outer.width && inner.y + inner.height <= outer.y + outer.height;
    for (let left = 0; left < fragments.length; left++) {
      for (let right = left + 1; right < fragments.length; right++) {
        const a = fragments[left].border_box;
        const b = fragments[right].border_box;
        if (contains(a, b) || contains(b, a)) continue;
        const x = Math.max(a.x, b.x), y = Math.max(a.y, b.y);
        const edgeX = Math.min(a.x + a.width, b.x + b.width), edgeY = Math.min(a.y + a.height, b.y + b.height);
        if (edgeX > x && edgeY > y) marks.collisions.push({rect: {x, y, width: edgeX - x, height: edgeY - y}, label: `collision · ${fragments[left].id}/${fragments[right].id}`});
      }
    }
    return marks;
  }

  return {baselineMarks, tableCellMarks, gridTrackMarks, pageRegionMarks, boxModelMarks, issueMarks};
});
