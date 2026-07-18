const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, '../web/wasm-renderer.js'), 'utf8');

class MockWorker {
  constructor(url) {
    this.url = url;
    this.listeners = new Map();
    this.messages = [];
    MockWorker.instance = this;
  }

  addEventListener(type, listener) {
    this.listeners.set(type, listener);
  }

  postMessage(message, transfer) {
    this.messages.push({message, transfer});
  }

  emit(type, data) {
    this.listeners.get(type)?.({data});
  }
}

function loadRenderer() {
  const context = {Worker: MockWorker, Promise, Error, Map, Object};
  context.globalThis = context;
  vm.runInNewContext(source, context, {filename: 'wasm-renderer.js'});
  return {renderer: context.PaperStudioWASMRenderer, worker: MockWorker.instance};
}

test('moves render payloads to the worker and resolves transferred bitmaps', async () => {
  const {renderer, worker} = loadRenderer();
  assert.equal(worker.url, '/wasm-renderer-worker.js');
  const payload = Uint8Array.from([1, 2, 3, 4]).buffer;
  const pending = renderer.renderResponse({arrayBuffer: async () => payload}, {revision: 'r1', page: 1, dpi: 144});
  worker.emit('message', {type: 'ready'});
  await new Promise(setImmediate);
  assert.equal(worker.messages.length, 1);
  assert.equal(worker.messages[0].message.type, 'render');
  assert.equal(worker.messages[0].message.payload, payload);
  assert.equal(worker.messages[0].transfer[0], payload);
  const id = worker.messages[0].message.id;
  const bitmap = {width: 20, height: 30};
  worker.emit('message', {
    type: 'rendered', id, bitmap, manifest: {page: 1}, viewBox: [0, 0, 10, 15], pixelWidth: 20, pixelHeight: 30,
  });
  const rendered = await pending;
  assert.equal(rendered.bitmap, bitmap);
  assert.deepEqual(rendered.viewBox, [0, 0, 10, 15]);
});

test('propagates worker render failures', async () => {
  const {renderer, worker} = loadRenderer();
  const pending = renderer.renderResponse({arrayBuffer: async () => new ArrayBuffer(1)}, {revision: 'r1', page: 1, dpi: 144});
  worker.emit('message', {type: 'ready'});
  await new Promise(setImmediate);
  const id = worker.messages[0].message.id;
  worker.emit('message', {type: 'error', id, error: 'render failed', status: 409});
  await assert.rejects(pending, (error) => error.message === 'render failed' && error.status === 409);
});
