(function (root) {
  'use strict';

  let runtimeFailure = null;

  async function instantiate(go) {
    const response = await fetch('/paper-studio.wasm', {cache: 'no-cache'});
    if (!response.ok) throw new Error(`WASM renderer unavailable (${response.status})`);
    if (WebAssembly.instantiateStreaming) {
      try {
        return await WebAssembly.instantiateStreaming(response.clone(), go.importObject);
      } catch (_) {
        // Some local proxies omit application/wasm. The verified same-origin
        // bytes still use the identical WebAssembly module below.
      }
    }
    return WebAssembly.instantiate(await response.arrayBuffer(), go.importObject);
  }

  const ready = (async () => {
    if (typeof Go !== 'function') throw new Error('Go WASM runtime bootstrap is unavailable');
    const go = new Go();
    const module = await instantiate(go);
    go.run(module.instance).catch((error) => { runtimeFailure = error; });
    for (let attempt = 0; attempt < 100 && !root.PaperStudioWASM; attempt += 1) {
      await new Promise((resolve) => setTimeout(resolve, 0));
    }
    if (runtimeFailure) throw runtimeFailure;
    if (!root.PaperStudioWASM?.render) throw new Error('Go WASM renderer did not initialize');
    return root.PaperStudioWASM;
  })();

  async function renderResponse(response, expected) {
    const engine = await ready;
    const payload = new Uint8Array(await response.arrayBuffer());
    const result = await engine.render(payload);
    const manifest = result?.manifest;
    if (!manifest || manifest.plan_hash !== expected.revision || manifest.page !== expected.page ||
        manifest.identity?.renderer_version !== engine.rendererVersion || manifest.media_type !== 'image/png' || manifest.profile?.dpi !== expected.dpi) {
      throw new Error('WASM renderer returned stale or invalid page evidence');
    }
    const blob = new Blob([result.png], {type: 'image/png'});
    const bitmap = await createImageBitmap(blob);
    if (bitmap.width !== manifest.pixel_width || bitmap.height !== manifest.pixel_height) {
      bitmap.close();
      throw new Error('WASM renderer pixel dimensions do not match its manifest');
    }
    const bounds = manifest.page_bounds;
    return {
      bitmap,
      manifest,
      viewBox: [bounds.x || 0, bounds.y || 0, bounds.width, bounds.height],
      pixelWidth: manifest.pixel_width,
      pixelHeight: manifest.pixel_height,
    };
  }

  root.PaperStudioWASMRenderer = Object.freeze({ready, renderResponse});
})(globalThis);
