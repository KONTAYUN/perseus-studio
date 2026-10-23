import test from 'node:test';
import assert from 'node:assert/strict';
import { mergeConversationSnapshotPreservingActive as merge } from '../../src/hooks/chatMessageMerge.js';

test('terminal snapshot replaces a longer draft and discards withdrawn previews', () => {
  const current = [
    { id: 'a', role: 'assistant', content: 'long stale draft', pending: true, runId: 'old' },
    { id: 'withdrawn', role: 'assistant', content: 'discard me', pending: true, runId: 'old' },
    { id: 'user-queued', role: 'user', content: 'next question', queued: true },
  ];
  const snapshot = [{ id: 'a', role: 'assistant', content: 'done', status: 'final' }];
  const got = merge(current, snapshot, { authoritative: true, runId: 'old' });
  assert.deepEqual(got.map(m => m.id), ['a', 'user-queued']);
  assert.equal(got[0].content, 'done');
  assert.equal(got[0].pending, false);
  assert.equal(got[0].loadingState, null);
  assert.deepEqual(merge(got, snapshot, { authoritative: true, runId: 'old' }), got);
});

test('finishing an old run preserves a newer active run against a delayed snapshot', () => {
  const active = { id: 'new', role: 'assistant', content: 'new run in progress', runId: 'new-run', pending: true };
  for (const snapshot of [[], [{ id: 'new', role: 'assistant', content: 'old prefix', status: 'streaming' }]]) {
    const got = merge([active], snapshot, { authoritative: true, runId: 'old-run' });
    assert.equal(got.find(m => m.id === 'new'), active);
  }
});

test('an explicit final message settles a pending message even outside terminal refresh', () => {
  const got = merge([{ id: 'a', role: 'assistant', content: 'long old preview', pending: true }],
    [{ id: 'a', role: 'assistant', content: 'short final', status: 'final' }]);
  assert.equal(got[0].content, 'short final');
  assert.equal(Boolean(got[0].pending), false);
});
