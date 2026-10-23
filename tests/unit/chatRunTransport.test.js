import assert from 'node:assert/strict';
import test from 'node:test';
import { consumeRunTransport } from '../../src/hooks/chatRunTransport.js';

class Source {
  static instances = [];
  constructor() { this.listeners = {}; Source.instances.push(this); }
  addEventListener(type, callback) { this.listeners[type] = callback; }
  close() { this.closed = true; }
  emit(type, data) { this.listeners[type]?.({ data }); }
}

const tick = () => new Promise((resolve) => setImmediate(resolve));

test('resume cursor rejects old events and an early stop opens no stream', async () => {
  const original = globalThis.EventSource;
  globalThis.EventSource = Source;
  try {
    let canceled = 0;
    const count = Source.instances.length;
    await assert.rejects(consumeRunTransport({ control: { canceled: true }, onCancel() { canceled++; } }), { name: 'AbortError' });
    assert.equal(Source.instances.length, count);
    assert.equal(canceled, 1);
    const control = {};
    let calls = 0, settled = false;
    const pending = consumeRunTransport({ initialSequence: 10, control, url: (sequence) => { assert.equal(sequence, 10); return '/'; },
      onEvent() { calls++; settled = true; }, isSettled: () => settled });
    const source = Source.instances.at(-1);
    source.emit('event', '{"seq":9}'); source.emit('event', '{"seq":11}');
    await pending;
    assert.equal(calls, 1);
  } finally { globalThis.EventSource = original; }
});

test('malformed event rejects and closes stream before later events apply', async () => {
  const original = globalThis.EventSource;
  globalThis.EventSource = Source;
  try {
    let calls = 0;
    const promise = consumeRunTransport({ url: () => '/', control: {}, onRun() {}, onEvent() { calls++; }, isSettled: () => false });
    const source = Source.instances.at(-1);
    source.emit('event', '{broken');
    source.emit('event', '{"seq":2}');
    await assert.rejects(promise, SyntaxError);
    await tick();
    assert.equal(calls, 0);
    assert.equal(source.closed, true);
  } finally { globalThis.EventSource = original; }
});

test('handler rejection stops queued delivery and clears abort ownership', async () => {
  const original = globalThis.EventSource;
  globalThis.EventSource = Source;
  try {
    let calls = 0;
    const control = {};
    const promise = consumeRunTransport({ url: () => '/', control, onRun() {}, onEvent() { calls++; throw new Error('refresh failed'); }, isSettled: () => false });
    const source = Source.instances.at(-1);
    source.emit('event', '{"seq":1}'); source.emit('event', '{"seq":2}');
    await assert.rejects(promise, /refresh failed/); await tick();
    assert.equal(calls, 1); assert.equal(source.closed, true);
    assert.doesNotThrow(() => control.abort());
  } finally { globalThis.EventSource = original; }
});

test('cancel while reconnecting removes the pending timer and cancels once', async () => {
  const original = globalThis.EventSource, originalWindow = globalThis.window;
  globalThis.EventSource = Source;
  let timer, cleared = false;
  globalThis.window = { setTimeout(fn) { timer = fn; return 42; }, clearTimeout(id) { cleared = id === 42; } };
  try {
    let canceled = 0;
    const control = {};
    const promise = consumeRunTransport({ url: () => '/', control, onCancel() { canceled++; }, isSettled: () => false });
    Source.instances.at(-1).onerror(); await tick();
    assert.equal(typeof timer, 'function');
    control.abort(); control.abort();
    await assert.rejects(promise, { name: 'AbortError' });
    assert.equal(cleared, true); assert.equal(canceled, 1);
  } finally { globalThis.EventSource = original; globalThis.window = originalWindow; }
});
