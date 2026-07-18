(function (root) {
  'use strict';

  const worker = new Worker('/wasm-renderer-worker.js');
  const pending = new Map();
  let requestID = 0;
  let resolveReady;
  let rejectReady;
  const ready = new Promise((resolve, reject) => {
    resolveReady = resolve;
    rejectReady = reject;
  });

  function rejectPending(error) {
    rejectReady(error);
    for (const request of pending.values()) request.reject(error);
    pending.clear();
  }

  worker.addEventListener('message', (event) => {
    const message = event.data;
    if (message?.type === 'ready') {
      resolveReady();
      return;
    }
    if (message?.type === 'fatal') {
      rejectPending(new Error(message.error || 'WASM worker failed'));
      return;
    }
    const request = pending.get(message?.id);
    if (!request) {
      message?.bitmap?.close?.();
      return;
    }
    pending.delete(message.id);
    if (message.type === 'error') {
      const error = new Error(message.error || 'WASM render failed');
      if (message.status) error.status = message.status;
      request.reject(error);
      return;
    }
    request.resolve({
      bitmap: message.bitmap,
      manifest: message.manifest,
      viewBox: message.viewBox,
      pixelWidth: message.pixelWidth,
      pixelHeight: message.pixelHeight,
    });
  });
  worker.addEventListener('error', (event) => rejectPending(new Error(event.message || 'WASM worker crashed')));

  async function renderResponse(response, expected) {
    await ready;
    const payload = await response.arrayBuffer();
    const id = ++requestID;
    const result = new Promise((resolve, reject) => pending.set(id, {resolve, reject}));
    worker.postMessage({type: 'render', id, payload, expected}, [payload]);
    return result;
  }

  root.PaperStudioWASMRenderer = Object.freeze({ready, renderResponse});
})(globalThis);
