import test from 'node:test';
import assert from 'node:assert/strict';

import { canApplyAIEditPreviewDelta, mergeAIEditPreview } from '../../src/hooks/aiPreviewState.js';
import { reduceTransactionOriginState } from '../../src/components/editor/richText/richTextTransactionOrigin.js';

test('preview deltas replace changed blocks and remove deleted blocks', () => {
  const current = {
    preview_id: 'preview-1',
    revision: 2,
    changes: [
      { block_id: 'b0001', operation: 'replace_text', replace: 'old' },
      { block_id: 'b0002', operation: 'delete' },
    ],
  };
  const event = {
    preview_id: 'preview-1',
    base_revision: 2,
    revision: 3,
    snapshot_full: false,
    changed_blocks: [{ block_id: 'b0001', operation: 'replace_text', replace: 'new' }],
    removed_blocks: ['b0002'],
  };
  assert.equal(canApplyAIEditPreviewDelta(current, event), true);
  assert.deepEqual(mergeAIEditPreview(current, event).changes, [
    { block_id: 'b0001', operation: 'replace_text', replace: 'new' },
  ]);
});

test('preview deltas reject missing revision baselines', () => {
  assert.equal(
    canApplyAIEditPreviewDelta(
      { preview_id: 'preview-1', revision: 4 },
      { preview_id: 'preview-1', base_revision: 3, revision: 5, snapshot_full: false }
    ),
    false
  );
});

test('transaction source persists across appended document transactions', () => {
  const idle = {
    origin: 'user',
    mutationId: '',
    documentVersion: '',
    phase: 'idle',
    lastOrigin: 'user',
  };
  const active = reduceTransactionOriginState(idle, {
    context: {
      action: 'begin',
      origin: 'authoritative_sync',
      mutationId: 'mutation-1',
      documentVersion: 'version-1',
      phase: 'reconciling',
    },
    docChanged: false,
  });
  const first = reduceTransactionOriginState(active, { docChanged: true });
  const appended = reduceTransactionOriginState(first, { docChanged: true });
  assert.equal(first.lastOrigin, 'authoritative_sync');
  assert.equal(appended.lastOrigin, 'authoritative_sync');
  assert.equal(appended.mutationId, 'mutation-1');
});
