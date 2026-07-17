// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const rail = require('../web/rail-model.js');

test('baseline labels never imply a usable mismatched revision', () => {
  assert.equal(rail.baselineLabel({status: 'available', changed_page_count: 2}), '2 changed');
  assert.equal(rail.baselineLabel({status: 'available', changed_page_count: 2, removed_page_count: 1}), '2 affected · 1 removed');
  assert.equal(rail.baselineLabel({status: 'available', changed_page_count: 0}), 'No changes');
  assert.equal(rail.baselineLabel({status: 'scenario_mismatch', changed_page_count: 9}), 'Other scenario');
  assert.equal(rail.baselineLabel({status: 'current_unavailable'}), 'Plan unavailable');
  assert.equal(rail.baselineLabel({status: 'none'}), 'No baseline');
  assert.equal(rail.baselineLabel({status: 'unexpected'}), 'Baseline cleared');
});

test('page summaries preserve exact page identity and deterministic selector fallback', () => {
  const first = {page: 1, selector: 'first', content_hash: 'a'};
  const third = {page: 3, selector: 'odd', content_hash: 'c'};
  const indexed = rail.pageSummaryMap([first, third]);
  assert.equal(indexed.get(1), first);
  assert.equal(indexed.get(3), third);
  assert.deepEqual(rail.fallbackPageSummary(2), {page: 2, selector: 'even', regions: [], repeated_regions: [], issues: []});
});
