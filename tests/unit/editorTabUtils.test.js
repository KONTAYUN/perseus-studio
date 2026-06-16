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
  assert.equal(isSuccessfulEditorToolEvent({ status: 'success', type: 'workspace_file_changed', changed_paths: ['notes/a.md'] }), true);
  assert.equal(isSuccessfulEditorToolEvent({ status: 'success', tool_name: 'unknown_tool', changed_paths: ['notes/a.md'] }), false);
  assert.equal(isSuccessfulEditorToolEvent({ status: 'failed', tool_name: 'save_workspace_file' }), false);
  assert.equal(isSuccessfulEditorToolEvent({ status: 'success', tool_name: 'web_search' }), false);
});

test('buildActiveFileToolPlan refreshes clean active file after AI write', () => {
  const plan = buildActiveFileToolPlan(
    [{ type: 'workspace_file_changed', tool_name: 'apply_workspace_patch', status_kind: 'workspace_edit', path: 'notes/a.md', changed_paths: ['notes/a.md'], hash: 'h1', version_id: 'v1' }],
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
    [{ type: 'workspace_file_changed', operation: 'patch', path: 'notes/a.md', changed_paths: ['notes/a.md'], hash: 'h-edit', version_id: 'v-edit' }],
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
    [{ type: 'workspace_file_changed', path: 'notes/a.md', changed_paths: ['notes/a.md'], hash: 'h-lines', version_id: 'v-lines' }],
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

test('buildActiveFileToolPlan refreshes active file from changed_paths', () => {
  const plan = buildActiveFileToolPlan(
    [{ type: 'workspace_file_changed', path: 'drafts/long.txt', changed_paths: ['drafts/long.txt'], hash: 'h-change' }],
    { activePath: 'drafts/long.txt', dirty: false }
  );

  assert.deepEqual(plan.reloadPlan, {
    path: 'drafts/long.txt',
    hash: 'h-change',
    versionId: '',
    statusMessage: '',
  });
});

test('buildActiveFileToolPlan ignores historical mutation events without workspace change type', () => {
  const plan = buildActiveFileToolPlan(
    [{ tool_name: 'historical_write', status_kind: 'workspace_edit', changed_paths: ['drafts/long.txt'], hash: 'h-final' }],
    { activePath: 'drafts/long.txt', dirty: false }
  );

  assert.deepEqual(plan.reloadPlan, null);
});

test('buildActiveFileToolPlan marks dirty active file conflict after AI write', () => {
  const plan = buildActiveFileToolPlan(
    [{ type: 'workspace_file_changed', tool_name: 'apply_workspace_patch', status_kind: 'workspace_edit', path: 'notes/a.md', changed_paths: ['notes/a.md'], hash: 'h1', version_id: 'v1' }],
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
    [{ type: 'workspace_file_changed', operation: 'rename', old_path: 'docs', new_path: 'notes', changed_paths: ['docs', 'notes'], hash: 'h2' }],
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
    [{ type: 'workspace_file_changed', operation: 'trash', old_path: 'docs', new_path: `${TRASH_DIR}/docs`, changed_paths: ['docs', `${TRASH_DIR}/docs`] }],
    { activePath: 'docs/a.md', dirty: true }
  );

  assert.equal(plan.activePath, '');
  assert.equal(plan.reloadPlan, null);
  assert.equal(plan.conflictPlan, null);
  assert.equal(plan.closeNotice, '当前打开文件已被 AI 移入回收站');
});

test('buildActiveFileToolPlan still recognizes legacy trash events', () => {
  const plan = buildActiveFileToolPlan(
    [{ type: 'workspace_file_changed', operation: 'trash', old_path: 'docs', new_path: `${LEGACY_TRASH_DIR}/docs`, changed_paths: ['docs', `${LEGACY_TRASH_DIR}/docs`] }],
    { activePath: 'docs/a.md', dirty: false }
  );

  assert.equal(plan.activePath, '');
  assert.equal(plan.closeNotice, '当前打开文件已被 AI 移入回收站');
});

test('pathMovesFromToolEvent extracts batch organize path pairs', () => {
  assert.deepEqual(
    pathMovesFromToolEvent({
      type: 'workspace_file_changed',
      operation: 'organize',
      moved: [
        { old_path: 'episodes/e29-a.md', new_path: 'episode-29/e29-a.md', status: 'success' },
        { old_path: 'episodes/e29-b.md', new_path: 'episode-29/e29-b.md', status: 'success' },
      ],
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
        type: 'workspace_file_changed',
        operation: 'organize',
        moved: [
          { old_path: 'episodes/e29-a.md', new_path: 'episode-29/e29-a.md', status: 'success' },
          { old_path: 'episodes/e29-b.md', new_path: 'episode-29/e29-b.md', status: 'success' },
        ],
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
      type: 'workspace_file_changed',
      operation: 'move',
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
