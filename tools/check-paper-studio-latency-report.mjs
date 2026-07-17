// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

import {readFile} from 'node:fs/promises';

const file = process.argv[2];
if (!file) throw new Error('latency report path is required');
const report = JSON.parse(await readFile(file, 'utf8'));
const budgets = {
  cold_workspace_ms: 500,
  wasm_initialization_ms: 500,
  first_visible_page_ms: 500,
  warm_visible_update_p95_ms: 100,
  change_notification_ms: 1000,
  incremental_workspace_ms: 500,
};
const values = {
  cold_workspace_ms: report.workspace?.cold_ms,
  wasm_initialization_ms: Number(report.wasm?.module_fetch_decode_ms || 0) + Number(report.wasm?.module_compile_ms || 0) + Number(report.wasm?.runtime_start_ms || 0),
  first_visible_page_ms: report.first_visible_page?.total_ms,
  warm_visible_update_p95_ms: report.warm?.visible_update_ms?.p95_ms,
  change_notification_ms: report.change_notification?.milliseconds,
  incremental_workspace_ms: report.incremental_workspace?.milliseconds,
};
if (!Number.isInteger(report.samples) || report.samples < 10) throw new Error('latency report requires at least ten samples');
const failures = [];
for (const [name, budget] of Object.entries(budgets)) {
  const value = Number(values[name]);
  if (!Number.isFinite(value) || value > budget) failures.push(`${name}=${value}ms > ${budget}ms`);
}
if (failures.length) throw new Error(`Paper Studio latency budget failed: ${failures.join(', ')}`);
console.log(JSON.stringify({status: 'pass', samples: report.samples, budgets, measured: values}, null, 2));
