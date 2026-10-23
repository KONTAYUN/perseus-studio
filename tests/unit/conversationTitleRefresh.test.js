import assert from 'node:assert/strict';
import test from 'node:test';
import { createConversationTitleRefresh } from '../../src/hooks/useConversationTitleRefresh.js';
import { createChatConversationActions } from '../../src/hooks/chatConversationActions.js';

test('title refresh cancels its previous owner and stops when the title arrives', async () => {
  const timers = new Map();
  let nextId = 0, release, oldSignal;
  const refresher = createConversationTitleRefresh({
    schedule(fn) { timers.set(++nextId, fn); return nextId; }, clear(id) { timers.delete(id); },
    refresh({ signal, quiet }) {
      assert.equal(quiet, true);
      if (!oldSignal) { oldSignal = signal; return new Promise((resolve) => { release = resolve; }); }
      return [{ id: 'new', title: 'Generated title' }];
    },
  });
  refresher.start('old');
  const oldTick = timers.get(1)(); timers.delete(1);
  refresher.start('new');
  assert.equal(oldSignal.aborted, true);
  release([]); await oldTick;
  assert.equal(timers.size, 1, 'old refresh must not reschedule itself');
  const newTick = timers.get(2)(); timers.delete(2); await newTick;
  assert.equal(timers.size, 0, 'generated titles need no more polling');
  refresher.start('unmounted'); refresher.stop();
  assert.equal(timers.size, 0);
});

test('background conversation listing preserves active error and recovery controls', async () => {
  const original = globalThis.fetch;
  let error = 'new run failed', resumable = { runId: 'new-run' }, listed;
  const actions = createChatConversationActions({
    currentConversationIdRef: { current: 'conv' },
    setChatError(value) { error = value; }, setResumableRun(value) { resumable = value; },
    setConversations(value) { listed = value; },
  });
  try {
    globalThis.fetch = async () => Response.json({ items: [{ id: 'conv', title: 'Generated' }] });
    await actions.loadConversations({ quiet: true });
    assert.equal(listed[0].title, 'Generated');
    assert.equal(error, 'new run failed');
    assert.equal(resumable.runId, 'new-run');
    const controller = new AbortController();
    globalThis.fetch = async () => { controller.abort(); return Response.json({ items: [] }); };
    await actions.loadConversations({ quiet: true, signal: controller.signal });
    assert.equal(listed[0].title, 'Generated', 'canceled response cannot clear the current conversation');
  } finally { globalThis.fetch = original; }
});
