// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

import vm from 'node:vm';

const baseURL = process.argv[2];
if (!baseURL) throw new Error('Paper Studio base URL is required');

const runtimeResponse = await fetch(`${baseURL}/wasm_exec.js`);
if (!runtimeResponse.ok) throw new Error(`wasm_exec.js returned ${runtimeResponse.status}`);
vm.runInThisContext(await runtimeResponse.text(), {filename: 'wasm_exec.js'});

const go = new Go();
const moduleResponse = await fetch(`${baseURL}/paper-studio.wasm`);
if (!moduleResponse.ok) throw new Error(`paper-studio.wasm returned ${moduleResponse.status}`);
const module = await WebAssembly.instantiate(await moduleResponse.arrayBuffer(), go.importObject);
let runtimeFailure = null;
go.run(module.instance).catch((error) => { runtimeFailure = error; });
for (let attempt = 0; attempt < 100 && !globalThis.PaperStudioWASM; attempt += 1) {
  await new Promise((resolve) => setTimeout(resolve, 1));
}
if (runtimeFailure) throw runtimeFailure;
if (!globalThis.PaperStudioWASM?.render) throw new Error('WASM renderer did not initialize');

const workspaceResponse = await fetch(`${baseURL}/api/workspace`);
if (!workspaceResponse.ok) throw new Error(`workspace returned ${workspaceResponse.status}`);
const workspace = await workspaceResponse.json();
const payloadResponse = await fetch(`${baseURL}/api/page/1.render?revision=${encodeURIComponent(workspace.revision)}`);
if (!payloadResponse.ok) throw new Error(`render payload returned ${payloadResponse.status}`);
const result = await globalThis.PaperStudioWASM.render(new Uint8Array(await payloadResponse.arrayBuffer()));
const png = result.png;
const manifest = result.manifest;
if (manifest.plan_hash !== workspace.revision || manifest.page !== 1 || manifest.identity?.renderer_version !== globalThis.PaperStudioWASM.rendererVersion ||
    manifest.pixel_width <= 0 || manifest.pixel_height <= 0 || png.length < 8 || png[0] !== 0x89 || png[1] !== 0x50 || png[2] !== 0x4e || png[3] !== 0x47) {
  throw new Error('WASM renderer returned invalid page evidence');
}
console.log(`paper-studio-wasm smoke: page 1 ${manifest.pixel_width}x${manifest.pixel_height}, ${png.length} PNG bytes`);
process.exit(0);
