import assert from 'node:assert/strict';
import test from 'node:test';

import { filterFreshEditorToolEvents, getAncestorFolderPaths, getFileTreeDropZone, shouldReloadWorkspaceForToolEvents } from '../../src/hooks/ideUtils.js';
import { nextAvailablePathInTree, topLevelSelectedItems } from '../../src/hooks/fileTreeUtils.js';

const rect = { top: 100, height: 40 };

test('getFileTreeDropZone returns before and after for row edges', () => {
  assert.equal(getFileTreeDropZone(105, rect, 'file'), 'before');
  assert.equal(getFileTreeDropZone(135, rect, 'file'), 'after');
});

test('getFileTreeDropZone returns inside only for folder row middle', () => {
  assert.equal(getFileTreeDropZone(120, rect, 'folder'), 'inside');
  assert.equal(getFileTreeDropZone(120, rect, 'file'), 'none');
});

test('nextAvailablePathInTree finds a non-conflicting numbered name', () => {
  const flatFiles = [{ path: 'docs/新建文件夹' }, { path: 'docs/新建文件夹 (2)' }];
  assert.equal(nextAvailablePathInTree(flatFiles, 'docs', '新建文件夹'), 'docs/新建文件夹 (3)');
});

test('topLevelSelectedItems removes descendants of selected folders', () => {
  const selected = [
    { path: 'docs', type: 'folder' },
    { path: 'docs/a.txt', type: 'file' },
    { path: 'other.txt', type: 'file' },
  ];
  assert.deepEqual(topLevelSelectedItems(selected), [
    { path: 'docs', type: 'folder' },
    { path: 'other.txt', type: 'file' },
  ]);
});

test('getAncestorFolderPaths returns every parent from root to leaf parent', () => {
  assert.deepEqual(getAncestorFolderPaths('docs/guides/readme.md'), ['docs', 'docs/guides']);
  assert.deepEqual(getAncestorFolderPaths('readme.md'), []);
});

test('filterFreshEditorToolEvents deduplicates repeated AI file refresh events', () => {
  const seen = new Set();
  const event = {
    tool_call_id: 'tc1',
    tool_name: 'save_workspace_file',
    status: 'success',
    path: 'docs/a.txt',
    hash: 'h1',
    version_id: 'v1',
  };

  assert.deepEqual(filterFreshEditorToolEvents([event], seen), [event]);
  assert.deepEqual(filterFreshEditorToolEvents([{ ...event }], seen), []);
  assert.deepEqual(filterFreshEditorToolEvents([{ ...event, hash: 'h2' }], seen), [{ ...event, hash: 'h2' }]);
});

test('shouldReloadWorkspaceForToolEvents includes precise AI edits', () => {
  assert.equal(shouldReloadWorkspaceForToolEvents([{ status: 'success', tool_name: 'edit_workspace_text' }]), true);
  assert.equal(shouldReloadWorkspaceForToolEvents([{ status: 'success', tool_name: 'replace_workspace_lines' }]), true);
  assert.equal(shouldReloadWorkspaceForToolEvents([{ status: 'success', tool_name: 'write_workspace_chunk' }]), true);
  assert.equal(shouldReloadWorkspaceForToolEvents([{ status: 'success', tool_name: 'finalize_workspace_write' }]), true);
  assert.equal(shouldReloadWorkspaceForToolEvents([{ status: 'failed', tool_name: 'edit_workspace_text' }]), false);
  assert.equal(shouldReloadWorkspaceForToolEvents([{ status: 'success', tool_name: 'read_workspace_text' }]), false);
});
