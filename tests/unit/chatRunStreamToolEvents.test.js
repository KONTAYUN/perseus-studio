import assert from 'node:assert/strict';
import test from 'node:test';

import { toolEventFromPayload } from '../../src/hooks/chatRunStreamToolEvents.js';
import { pathMovesFromToolEvent } from '../../src/hooks/editorTabUtils.js';
import { shouldReloadWorkspaceForToolEvents } from '../../src/hooks/ideUtils.js';

test('toolEventFromPayload preserves typed workspace change events for editor refresh', () => {
  const event = toolEventFromPayload(
    {
      tool_call_id: 'tc1',
      tool_name: 'apply_workspace_patch',
      status_kind: 'workspace_edit',
      status: 'success',
      path: 'drafts/a.md',
      changed_paths: ['drafts/a.md'],
      operation: 'patch',
      operation_hash: 'hash-op',
      hash: 'hash-content',
      version_id: 'v1',
    },
    'workspace_file_changed'
  );

  assert.equal(event.type, 'workspace_file_changed');
  assert.equal(event.tool_call_id, 'tc1');
  assert.equal(event.operation_hash, 'hash-op');
  assert.equal(event.version_id, 'v1');
  assert.deepEqual(event.changed_paths, ['drafts/a.md']);
  assert.equal(shouldReloadWorkspaceForToolEvents([event]), true);
});

test('toolEventFromPayload does not infer workspace refresh from generic tool completion', () => {
  const event = toolEventFromPayload({
    tool_call_id: 'tc1',
    tool_name: 'apply_workspace_patch',
    status_kind: 'workspace_edit',
    status: 'success',
    path: 'drafts/a.md',
    changed_paths: ['drafts/a.md'],
  });

  assert.equal(event.type, '');
  assert.equal(shouldReloadWorkspaceForToolEvents([event]), false);
});

test('toolEventFromPayload preserves batch move details from workspace change events', () => {
  const event = toolEventFromPayload(
    {
      tool_name: 'organize_workspace_matches',
      status: 'success',
      operation: 'organize',
      moved: [
        { old_path: 'episodes/e01.md', new_path: 'archive/e01.md', status: 'success' },
        { old_path: 'episodes/e02.md', new_path: 'archive/e02.md', status: 'success' },
      ],
    },
    'workspace_file_changed'
  );

  assert.deepEqual(pathMovesFromToolEvent(event), [
    { oldPath: 'episodes/e01.md', newPath: 'archive/e01.md' },
    { oldPath: 'episodes/e02.md', newPath: 'archive/e02.md' },
  ]);
});
