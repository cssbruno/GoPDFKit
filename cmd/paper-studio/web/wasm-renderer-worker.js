(function (root) {
  'use strict';

  importScripts('/wasm_exec.js');

  let runtimeFailure = null;

  async function instantiate(go) {
    const response = await fetch('/paper-studio.wasm', {cache: 'no-cache'});
    if (!response.ok) throw new Error(`WASM renderer unavailable (${response.status})`);
    if (WebAssembly.instantiateStreaming) {
      try {
        return await WebAssembly.instantiateStreaming(response.clone(), go.importObject);
      } catch (_) {
        // Preserve the verified same-origin fallback for proxies that report
        // the wrong media type.
      }
    }
    return WebAssembly.instantiate(await response.arrayBuffer(), go.importObject);
  }

  const ready = (async () => {
    const go = new Go();
    const module = await instantiate(go);
    go.run(module.instance).catch((error) => { runtimeFailure = error; });
    for (let attempt = 0; attempt < 100 && !root.PaperStudioWASM; attempt += 1) {
      await new Promise((resolve) => setTimeout(resolve, 0));
    }
    if (runtimeFailure) throw runtimeFailure;
    if (!root.PaperStudioWASM?.render) throw new Error('Go WASM renderer did not initialize');
    root.postMessage({type: 'ready'});
    return root.PaperStudioWASM;
  })();

  function fail(id, error) {
    root.postMessage({
      type: 'error',
      id,
      error: String(error?.message || error || 'WASM worker failed'),
      status: Number(error?.status || 0),
    });
  }

  async function render(message) {
    const engine = await ready;
    const result = await engine.render(new Uint8Array(message.payload));
    const manifest = result?.manifest;
    const expected = message.expected;
    if (!manifest || manifest.plan_hash !== expected.revision || manifest.page !== expected.page ||
        manifest.identity?.renderer_version !== engine.rendererVersion || manifest.media_type !== 'image/png' ||
        manifest.profile?.dpi !== expected.dpi) {
      throw new Error('WASM renderer returned stale or invalid page evidence');
    }
    const bitmap = await createImageBitmap(new Blob([result.png], {type: 'image/png'}));
    if (bitmap.width !== manifest.pixel_width || bitmap.height !== manifest.pixel_height) {
      bitmap.close();
      throw new Error('WASM renderer pixel dimensions do not match its manifest');
    }
    root.postMessage({
      type: 'rendered',
      id: message.id,
      bitmap,
      manifest,
      viewBox: [manifest.page_bounds.x || 0, manifest.page_bounds.y || 0, manifest.page_bounds.width, manifest.page_bounds.height],
      pixelWidth: manifest.pixel_width,
      pixelHeight: manifest.pixel_height,
    }, [bitmap]);
  }

  let renderQueue = Promise.resolve();
  root.onmessage = (event) => {
    const message = event.data;
    if (message?.type !== 'render') return;
    renderQueue = renderQueue.then(() => render(message)).catch((error) => fail(message.id, error));
  };

  ready.catch((error) => root.postMessage({type: 'fatal', error: String(error?.message || error)}));
})(globalThis);
