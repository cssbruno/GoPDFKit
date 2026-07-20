// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

(function installPaperStudioRailModel(root) {
  'use strict';

  function baselineLabel(baseline = {}) {
    switch (baseline.status) {
      case 'available':
        if (baseline.removed_page_count) return `${baseline.changed_page_count} affected · ${baseline.removed_page_count} removed`;
        return baseline.changed_page_count ? `${baseline.changed_page_count} changed` : 'No changes';
      case 'scenario_mismatch': return 'Other scenario';
      case 'current_unavailable': return 'Plan unavailable';
      case 'none': return 'No baseline';
      default: return 'Baseline cleared';
    }
  }

  function pageSummaryMap(summaries = []) {
    return new Map(summaries.map((summary) => [summary.page, summary]));
  }

  function fallbackPageSummary(page) {
    return {page, selector: page === 1 ? 'first' : page % 2 ? 'odd' : 'even', regions: [], repeated_regions: [], issues: []};
  }

  const model = Object.freeze({baselineLabel, pageSummaryMap, fallbackPageSummary});
  root.PaperStudioRailModel = model;
  if (typeof module !== 'undefined' && module.exports) module.exports = model;
})(globalThis);
