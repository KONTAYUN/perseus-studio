import assert from 'node:assert/strict';
import test from 'node:test';

import { visibleMessageAttachments } from '../../src/components/chat/chatUtils.js';
import { shouldReloadWorkspaceForToolEvents } from '../../src/hooks/ideUtils.js';

test('visibleMessageAttachments hides editor context metadata', () => {
  const visible = visibleMessageAttachments({
    attachments: [
      { type: 'phase_message', name: 'phase metadata' },
      { type: 'current_file', name: 'current.md' },
      { kind: 'editor_panels_context', name: 'editor context' },
      { type: 'workspace_file', name: 'referenced.md' },
      { kind: 'text', name: 'uploaded.txt' },
    ],
  });

  assert.deepEqual(
    visible.map((item) => item.name),
    ['referenced.md', 'uploaded.txt']
  );
});

test('shouldReloadWorkspaceForToolEvents reloads for AI file writes', () => {
  assert.equal(
    shouldReloadWorkspaceForToolEvents([{ tool_name: 'save_workspace_file', status: 'success', path: 'new-file.md' }]),
    true
  );
  assert.equal(
    shouldReloadWorkspaceForToolEvents([{ tool_name: 'save_workspace_file', status: 'failed', path: 'new-file.md' }]),
    false
  );
});
