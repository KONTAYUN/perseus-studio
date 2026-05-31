import assert from 'node:assert/strict';
import test from 'node:test';

import {
  buildActiveFileToolPlan,
  buildInactiveTabWriteConflict,
  filterTabsOutsidePath,
  isSuccessfulEditorToolEvent,
  isTabDirty,
  nextActiveTabAfterClose,
  pathMovesFromToolEvent,
  remapEditorPath,
  remapEditorTabs,
  removeTabByPath,
  reorderTabsByPath,
} from '../../src/hooks/editorTabUtils.js';
import { LEGACY_TRASH_DIR, TRASH_DIR } from '../../src/hooks/ideUtils.js';

test('isTabDirty compares current and saved content safely', () => {
  assert.equal(isTabDirty(null), false);
  assert.equal(isTabDirty({ content: '', savedContent: undefined }), false);
  assert.equal(isTabDirty({ content: 'draft', savedContent: 'saved' }), true);
});

test('removeTabByPath and nextActiveTabAfterClose keep tab order stable', () => {
  const tabs = [{ path: 'a.md' }, { path: 'b.md' }, { path: 'c.md' }];
  assert.deepEqual(removeTabByPath(tabs, 'b.md'), [{ path: 'a.md' }, { path: 'c.md' }]);
  assert.deepEqual(nextActiveTabAfterClose(tabs, 'c.md'), { path: 'b.md' });
  assert.deepEqual(nextActiveTabAfterClose(tabs, 'a.md'), { path: 'c.md' });
});

test('reorderTabsByPath moves tabs before or after a target tab', () => {
  const tabs = [{ path: 'a.md' }, { path: 'b.md' }, { path: 'c.md' }, { path: 'd.md' }];

  assert.deepEqual(reorderTabsByPath(tabs, 'd.md', 'b.md', 'before').map((tab) => tab.path), [
    'a.md',
    'd.md',
    'b.md',
    'c.md',
  ]);
  assert.deepEqual(reorderTabsByPath(tabs, 'a.md', 'c.md', 'after').map((tab) => tab.path), [
    'b.md',
    'c.md',
    'a.md',
    'd.md',
  ]);
  assert.deepEqual(reorderTabsByPath(tabs, 'b.md', 'b.md', 'after'), tabs);
});

test('remapEditorTabs updates file and nested folder tabs', () => {
  const tabs = [
    { path: 'docs/a.md', fileType: 'text' },
    { path: 'docs/images/logo.png', fileType: 'text' },
    { path: 'other.md', fileType: 'text' },
  ];

  assert.deepEqual(remapEditorTabs(tabs, 'docs', 'notes'), [
    { path: 'notes/a.md', fileType: 'text' },
    { path: 'notes/images/logo.png', fileType: 'image' },
    { path: 'other.md', fileType: 'text' },
  ]);
});

test('remapEditorPath only changes matching paths', () => {
  assert.equal(remapEditorPath('docs/a.md', 'docs', 'notes'), 'notes/a.md');
  assert.equal(remapEditorPath('docs-old/a.md', 'docs', 'notes'), 'docs-old/a.md');
});

test('filterTabsOutsidePath removes deleted file or folder descendants', () => {
  const tabs = [{ path: 'docs/a.md' }, { path: 'docs/nested/b.md' }, { path: 'other.md' }];
  assert.deepEqual(filterTabsOutsidePath(tabs, 'docs'), [{ path: 'other.md' }]);
  assert.deepEqual(filterTabsOutsidePath(tabs, 'docs/a.md'), [
    { path: 'docs/nested/b.md' },
    { path: 'other.md' },
  ]);
});

test('isSuccessfulEditorToolEvent accepts only successful editor-affecting tools', () => {
  assert.equal(isSuccessfulEditorToolEvent({ status: 'success', tool_name: 'edit_workspace_text' }), true);
  assert.equal(isSuccessfulEditorToolEvent({ status: 'success', tool_name: 'replace_workspace_lines' }), true);
  assert.equal(isSuccessfulEditorToolEvent({ status: 'success', tool_name: 'save_workspace_file' }), true);
  assert.equal(isSuccessfulEditorToolEvent({ status: 'success', tool_name: 'write_workspace_chunk' }), true);
  assert.equal(isSuccessfulEditorToolEvent({ status: 'success', tool_name: 'finalize_workspace_write' }), true);
  assert.equal(isSuccessfulEditorToolEvent({ status: 'failed', tool_name: 'save_workspace_file' }), false);
  assert.equal(isSuccessfulEditorToolEvent({ status: 'success', tool_name: 'web_search' }), false);
});

test('buildActiveFileToolPlan refreshes clean active file after AI write', () => {
  const plan = buildActiveFileToolPlan(
    [{ tool_name: 'save_workspace_file', path: 'notes/a.md', hash: 'h1', version_id: 'v1' }],
    { activePath: 'notes/a.md', dirty: false }
  );

  assert.deepEqual(plan.reloadPlan, {
    path: 'notes/a.md',
    hash: 'h1',
    versionId: 'v1',
    statusMessage: '',
  });
  assert.equal(plan.conflictPlan, null);
});

test('buildActiveFileToolPlan refreshes clean active file after precise AI edit', () => {
  const plan = buildActiveFileToolPlan(
    [{ tool_name: 'edit_workspace_text', path: 'notes/a.md', hash: 'h-edit', version_id: 'v-edit' }],
    { activePath: 'notes/a.md', dirty: false }
  );

  assert.deepEqual(plan.reloadPlan, {
    path: 'notes/a.md',
    hash: 'h-edit',
    versionId: 'v-edit',
    statusMessage: '',
  });
  assert.equal(plan.conflictPlan, null);
});

test('buildActiveFileToolPlan refreshes clean active file after line-range AI edit', () => {
  const plan = buildActiveFileToolPlan(
    [{ tool_name: 'replace_workspace_lines', path: 'notes/a.md', hash: 'h-lines', version_id: 'v-lines' }],
    { activePath: 'notes/a.md', dirty: false }
  );

  assert.deepEqual(plan.reloadPlan, {
    path: 'notes/a.md',
    hash: 'h-lines',
    versionId: 'v-lines',
    statusMessage: '',
  });
  assert.equal(plan.conflictPlan, null);
});

test('buildActiveFileToolPlan refreshes active file after chunked AI write events', () => {
  const plan = buildActiveFileToolPlan(
    [{ tool_name: 'write_workspace_chunk', path: 'drafts/long.txt', changed_paths: ['drafts/long.txt'], hash: 'h-chunk' }],
    { activePath: 'drafts/long.txt', dirty: false }
  );

  assert.deepEqual(plan.reloadPlan, {
    path: 'drafts/long.txt',
    hash: 'h-chunk',
    versionId: '',
    statusMessage: '',
  });
});

test('buildActiveFileToolPlan uses changed_paths for finalize write refresh', () => {
  const plan = buildActiveFileToolPlan(
    [{ tool_name: 'finalize_workspace_write', changed_paths: ['drafts/long.txt'], hash: 'h-final' }],
    { activePath: 'drafts/long.txt', dirty: false }
  );

  assert.deepEqual(plan.reloadPlan, {
    path: 'drafts/long.txt',
    hash: 'h-final',
    versionId: '',
    statusMessage: '',
  });
});

test('buildActiveFileToolPlan marks dirty active file conflict after AI write', () => {
  const plan = buildActiveFileToolPlan(
    [{ tool_name: 'save_workspace_file', path: 'notes/a.md', hash: 'h1', version_id: 'v1' }],
    { activePath: 'notes/a.md', dirty: true }
  );

  assert.deepEqual(plan.conflictPlan, {
    reloadPath: 'notes/a.md',
    hash: 'h1',
    versionId: 'v1',
    message: '该文件已被 AI 修改，但你有未保存内容，请选择保留本地修改或重新加载文件',
  });
  assert.equal(plan.reloadPlan, null);
});

test('buildActiveFileToolPlan remaps active file after rename', () => {
  const plan = buildActiveFileToolPlan(
    [{ tool_name: 'manage_workspace_path', old_path: 'docs', new_path: 'notes', hash: 'h2' }],
    { activePath: 'docs/a.md', dirty: false }
  );

  assert.equal(plan.activePath, 'notes/a.md');
  assert.deepEqual(plan.reloadPlan, {
    path: 'notes/a.md',
    hash: 'h2',
    versionId: '',
    statusMessage: '',
  });
});

test('buildActiveFileToolPlan closes active file after delete', () => {
  const plan = buildActiveFileToolPlan(
    [{ tool_name: 'manage_workspace_path', old_path: 'docs', new_path: `${TRASH_DIR}/docs` }],
    { activePath: 'docs/a.md', dirty: true }
  );

  assert.equal(plan.activePath, '');
  assert.equal(plan.reloadPlan, null);
  assert.equal(plan.conflictPlan, null);
  assert.equal(plan.closeNotice, '当前打开文件已被 AI 移入回收站');
});

test('buildActiveFileToolPlan still recognizes legacy trash events', () => {
  const plan = buildActiveFileToolPlan(
    [{ tool_name: 'manage_workspace_path', old_path: 'docs', new_path: `${LEGACY_TRASH_DIR}/docs` }],
    { activePath: 'docs/a.md', dirty: false }
  );

  assert.equal(plan.activePath, '');
  assert.equal(plan.closeNotice, '当前打开文件已被 AI 移入回收站');
});

test('pathMovesFromToolEvent extracts batch organize path pairs', () => {
  assert.deepEqual(
    pathMovesFromToolEvent({
      tool_name: 'organize_workspace_matches',
      changed_paths: ['episodes/e29-a.md', 'episode-29/e29-a.md', 'episodes/e29-b.md', 'episode-29/e29-b.md'],
    }),
    [
      { oldPath: 'episodes/e29-a.md', newPath: 'episode-29/e29-a.md' },
      { oldPath: 'episodes/e29-b.md', newPath: 'episode-29/e29-b.md' },
    ]
  );
});

test('buildActiveFileToolPlan remaps active file after batch organize', () => {
  const plan = buildActiveFileToolPlan(
    [
      {
        tool_name: 'organize_workspace_matches',
        changed_paths: ['episodes/e29-a.md', 'episode-29/e29-a.md', 'episodes/e29-b.md', 'episode-29/e29-b.md'],
      },
    ],
    { activePath: 'episodes/e29-b.md', dirty: false }
  );

  assert.equal(plan.activePath, 'episode-29/e29-b.md');
  assert.deepEqual(plan.reloadPlan, {
    path: 'episode-29/e29-b.md',
    hash: '',
    versionId: '',
    statusMessage: '',
  });
});

test('pathMovesFromToolEvent extracts direct move event', () => {
  assert.deepEqual(
    pathMovesFromToolEvent({
      tool_name: 'manage_workspace_path',
      old_path: 'drafts',
      new_path: 'archive/drafts',
    }),
    [{ oldPath: 'drafts', newPath: 'archive/drafts' }]
  );
});

test('buildInactiveTabWriteConflict creates conflict payload for dirty inactive tabs', () => {
  assert.deepEqual(
    buildInactiveTabWriteConflict({ path: 'a.md', hash: 'h', version_id: 'v' }),
    {
      reloadPath: 'a.md',
      hash: 'h',
      versionId: 'v',
      message: '该文件已被 AI 修改，但这个标签页有未保存内容，请切换过去后选择保留本地修改或重新加载文件',
    }
  );
});
