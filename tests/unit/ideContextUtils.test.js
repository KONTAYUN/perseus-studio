import assert from 'node:assert/strict';
import test from 'node:test';

import { buildEditorContextFromEntries, editorPositionLabels } from '../../src/hooks/ideContextUtils.js';

test('editorPositionLabels maps common editor counts to natural labels', () => {
  assert.deepEqual(editorPositionLabels(1), ['current']);
  assert.deepEqual(editorPositionLabels(2), ['left', 'right']);
  assert.deepEqual(editorPositionLabels(3), ['left', 'middle', 'right']);
  assert.deepEqual(editorPositionLabels(5), ['left', 'middle-1', 'middle-2', 'middle-3', 'right']);
});

test('buildEditorContextFromEntries returns path-only panel metadata for a single active editor', () => {
  const context = buildEditorContextFromEntries([
    {
      panel: { id: 'editor_1' },
      editor: {
        currentFilePath: 'notes/today.md',
        currentFileType: 'text',
        openTabs: [{ path: 'notes/today.md' }],
      },
    },
  ]);

  assert.equal(context.type, 'editor_panels_context');
  assert.deepEqual(context.panels, [{
    panel_id: 'editor_1',
    index: 1,
    position_label: 'current',
    active_file_path: 'notes/today.md',
    active_file_name: 'today.md',
    open_tabs: ['notes/today.md'],
  }]);
});

test('buildEditorContextFromEntries returns panel metadata without file bodies', () => {
  const context = buildEditorContextFromEntries([
    {
      panel: { id: 'editor_1' },
      editor: {
        currentFilePath: 'left.md',
        currentFileType: 'text',
        openTabs: [{ path: 'left.md', content: 'secret left body' }],
      },
    },
    {
      panel: { id: 'editor_2' },
      editor: {
        currentFilePath: 'right.md',
        currentFileType: 'text',
        openTabs: [{ path: 'right.md', content: 'secret right body' }],
      },
    },
  ]);

  assert.equal(context.type, 'editor_panels_context');
  assert.deepEqual(context.panels, [
    {
      panel_id: 'editor_1',
      index: 1,
      position_label: 'left',
      active_file_path: 'left.md',
      active_file_name: 'left.md',
      open_tabs: ['left.md'],
    },
    {
      panel_id: 'editor_2',
      index: 2,
      position_label: 'right',
      active_file_path: 'right.md',
      active_file_name: 'right.md',
      open_tabs: ['right.md'],
    },
  ]);
  assert.equal(JSON.stringify(context).includes('secret left body'), false);
  assert.equal(JSON.stringify(context).includes('secret right body'), false);
});
