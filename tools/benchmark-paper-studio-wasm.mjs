// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

import {readFile, writeFile} from 'node:fs/promises';
import vm from 'node:vm';
import {performance} from 'node:perf_hooks';

const baseURL = String(process.argv[2] || '').replace(/\/$/, '');
const sampleCount = Number(process.argv[3] || process.env.PAPER_STUDIO_BENCH_SAMPLES || 10);
if (!baseURL) throw new Error('Paper Studio base URL is required');
if (!Number.isInteger(sampleCount) || sampleCount < 1 || sampleCount > 100) {
  throw new Error('sample count must be an integer from 1 through 100');
}

const sourceFile = process.env.PAPER_STUDIO_BENCH_SOURCE || '';
const optionalScenario = process.env.PAPER_STUDIO_BENCH_SCENARIO || '';

function now() {
  return performance.now();
}

async function timed(operation) {
  const started = now();
  const value = await operation();
  return {value, milliseconds: now() - started};
}

async function responseBytes(path) {
  const response = await fetch(`${baseURL}${path}`, {cache: 'no-store'});
  if (!response.ok) throw new Error(`${path} returned ${response.status}`);
  return new Uint8Array(await response.arrayBuffer());
}

async function responseJSON(path) {
  const response = await fetch(`${baseURL}${path}`, {cache: 'no-store'});
  if (!response.ok) throw new Error(`${path} returned ${response.status}`);
  return response.json();
}

function percentile(values, fraction) {
  const sorted = [...values].sort((left, right) => left - right);
  return sorted[Math.min(sorted.length - 1, Math.floor((sorted.length - 1) * fraction))];
}

function summarize(values) {
  return {
    p50_ms: percentile(values, 0.50),
    p95_ms: percentile(values, 0.95),
    max_ms: Math.max(...values),
  };
}

function query(params) {
  const encoded = new URLSearchParams(params).toString();
  return encoded ? `?${encoded}` : '';
}

async function loadWASM() {
  const runtime = await timed(async () => {
    const response = await fetch(`${baseURL}/wasm_exec.js`, {cache: 'no-store'});
    if (!response.ok) throw new Error(`wasm_exec.js returned ${response.status}`);
    return response.text();
  });
  vm.runInThisContext(await runtime.value, {filename: 'wasm_exec.js'});

  const module = await timed(async () => {
    const response = await fetch(`${baseURL}/paper-studio.wasm`, {cache: 'no-store'});
    if (!response.ok) throw new Error(`paper-studio.wasm returned ${response.status}`);
    return new Uint8Array(await response.arrayBuffer());
  });
  const go = new Go();
  const compiled = await timed(() => WebAssembly.instantiate(module.value, go.importObject));
  let runtimeFailure = null;
  const started = now();
  go.run(compiled.value.instance).catch((error) => { runtimeFailure = error; });
  for (let attempt = 0; attempt < 200 && !globalThis.PaperStudioWASM; attempt += 1) {
    await new Promise((resolve) => setTimeout(resolve, 1));
  }
  if (runtimeFailure) throw runtimeFailure;
  if (!globalThis.PaperStudioWASM?.render) throw new Error('WASM renderer did not initialize');
  return {
    runtime_ms: runtime.milliseconds,
    module_fetch_decode_ms: module.milliseconds,
    module_compile_ms: compiled.milliseconds,
    runtime_start_ms: now() - started,
    module_bytes: module.value.byteLength,
    engine: globalThis.PaperStudioWASM,
  };
}

async function renderPayload(engine, payload) {
  const started = now();
  const result = await engine.render(payload);
  if (!result?.manifest || !result.png?.length) throw new Error('WASM renderer returned no raster evidence');
  return {result, milliseconds: now() - started};
}

async function readUntil(reader, marker, timeoutMilliseconds = 5000) {
  const decoder = new TextDecoder();
  let text = '';
  const deadline = now() + timeoutMilliseconds;
  while (!text.includes(marker)) {
    const remaining = deadline - now();
    if (remaining <= 0) throw new Error(`timed out waiting for ${marker}`);
    const result = await Promise.race([
      reader.read(),
      new Promise((_, reject) => setTimeout(() => reject(new Error(`timed out waiting for ${marker}`)), remaining)),
    ]);
    if (result.done) throw new Error(`change stream ended before ${marker}`);
    text += decoder.decode(result.value, {stream: true});
  }
  return text;
}

async function measureChangeNotification(workspace) {
  if (!sourceFile) return {status: 'not-run'};
  const controller = new AbortController();
  const response = await fetch(`${baseURL}/api/changes${query({source_revision: workspace.source_revision})}`, {signal: controller.signal});
  if (!response.ok || !response.body) throw new Error(`change stream returned ${response.status}`);
  const reader = response.body.getReader();
  await readUntil(reader, ': connected');
  const source = await readFile(sourceFile, 'utf8');
  const updated = source.replace('Exact box model', 'Exact box model changed');
  if (updated === source) throw new Error('benchmark fixture does not contain its replacement marker');
  const started = now();
  await writeFile(sourceFile, updated, 'utf8');
  await readUntil(reader, 'event: changed');
  controller.abort();
  await reader.cancel().catch(() => {});
  return {status: 'measured', milliseconds: now() - started};
}

const coldWorkspace = await timed(() => responseJSON('/api/workspace'));
const workspace = coldWorkspace.value;
if (!workspace.revision || !workspace.source_revision || !workspace.pages) throw new Error('workspace has no usable plan');

const wasm = await loadWASM();
const payloadPath = `/api/page/1.render${query({revision: workspace.revision})}`;
const firstPayload = await timed(() => responseBytes(payloadPath));
const firstPaint = await renderPayload(wasm.engine, firstPayload.value);

let scenarioWorkspace = null;
if (optionalScenario) {
  const scenario = await timed(() => responseJSON(`/api/workspace${query({scenario: optionalScenario})}`));
  scenarioWorkspace = {milliseconds: scenario.milliseconds, pages: scenario.value.pages, revision: scenario.value.revision};
}

const samples = [];
for (let index = 0; index < sampleCount; index += 1) {
  const warmWorkspace = await timed(() => responseJSON('/api/workspace'));
  const revision = warmWorkspace.value.revision;
  const payload = await timed(() => responseBytes(`/api/page/1.render${query({revision})}`));
  const paint = await renderPayload(wasm.engine, payload.value);
  samples.push({
    workspace_ms: warmWorkspace.milliseconds,
    payload_ms: payload.milliseconds,
    paint_ms: paint.milliseconds,
    visible_update_ms: warmWorkspace.milliseconds + payload.milliseconds + paint.milliseconds,
  });
}

const notification = await measureChangeNotification(workspace);
let incrementalWorkspace = null;
if (notification.status === 'measured') {
  const changed = await timed(() => responseJSON('/api/workspace'));
  incrementalWorkspace = {milliseconds: changed.milliseconds, revision: changed.value.revision, pages: changed.value.pages};
}

const report = {
  schema_version: 1,
  renderer_version: wasm.engine.rendererVersion,
  samples: sampleCount,
  workspace: {cold_ms: coldWorkspace.milliseconds, pages: workspace.pages, revision: workspace.revision},
  wasm: {
    runtime_ms: wasm.runtime_ms,
    module_fetch_decode_ms: wasm.module_fetch_decode_ms,
    module_compile_ms: wasm.module_compile_ms,
    runtime_start_ms: wasm.runtime_start_ms,
    module_bytes: wasm.module_bytes,
  },
  first_visible_page: {
    payload_fetch_decode_ms: firstPayload.milliseconds,
    paint_ms: firstPaint.milliseconds,
    total_ms: firstPayload.milliseconds + firstPaint.milliseconds,
    pixel_width: firstPaint.result.manifest.pixel_width,
    pixel_height: firstPaint.result.manifest.pixel_height,
    png_bytes: firstPaint.result.png.length,
  },
  warm: {
    workspace_ms: summarize(samples.map((sample) => sample.workspace_ms)),
    payload_ms: summarize(samples.map((sample) => sample.payload_ms)),
    paint_ms: summarize(samples.map((sample) => sample.paint_ms)),
    visible_update_ms: summarize(samples.map((sample) => sample.visible_update_ms)),
    samples,
  },
  cold_scenario_workspace: scenarioWorkspace,
  change_notification: notification,
  incremental_workspace: incrementalWorkspace,
};

console.log(JSON.stringify(report, null, 2));
