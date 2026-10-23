import assert from 'node:assert/strict';
import test from 'node:test';
import { createEditorToolRefreshActions } from '../../src/hooks/editorToolRefreshActions.js';
import { buildActiveFileToolPlan } from '../../src/hooks/editorTabUtils.js';

const patchEvent = (overrides = {}) => ({
  type: 'workspace_file_changed', tool_name: 'apply_workspace_patch', status: 'success',
  operation: 'patch', path: 'a.md', changed_paths: ['a.md', 'b.md', 'c.md'], hash: 'hash-a',
  ...overrides,
});

function editorHarness(paths = ['a.md', 'b.md', 'c.md']) {
  const openTabsRef = { current: paths.map((path) => ({ path, content: 'old', savedContent: 'old' })) };
  const currentFilePathRef = { current: 'a.md' };
  const actions = createEditorToolRefreshActions({
    openTabsRef, currentFilePathRef, isDirtyRef: { current: false },
    setOpenTabs: (update) => { openTabsRef.current = update(openTabsRef.current); },
    setActiveTabPath: () => {}, setCurrentFilePath: () => {}, setCurrentFileType: () => {},
    setEditorStatusMessage: () => {}, setExternalFileConflict: () => {},
    closeCurrentFile: () => {}, loadFileIntoEditor: async () => {},
  });
  return { actions, openTabsRef, currentFilePathRef };
}

test('one patch refreshes every affected inactive tab with its own hash', async () => {
  const originalFetch = globalThis.fetch;
  try {
    globalThis.fetch = async (url) => {
      const path = new URL(url, 'http://localhost').searchParams.get('path');
      return Response.json({ path, content: `new ${path}`, hash: `hash-${path}` });
    };
    const { actions, openTabsRef } = editorHarness();
    await actions.refreshWorkspaceAfterAssistantTools([patchEvent()]);
    for (const path of ['b.md', 'c.md']) {
      const tab = openTabsRef.current.find((item) => item.path === path);
      assert.equal(tab.content, `new ${path}`);
      assert.equal(tab.hash, `hash-${path}`);
    }
  } finally { globalThis.fetch = originalFetch; }
});

test('refresh keeps edits made while the file request was pending', async () => {
  const originalFetch = globalThis.fetch;
  try {
    const { actions, openTabsRef } = editorHarness(['a.md', 'b.md']);
    globalThis.fetch = async () => {
      openTabsRef.current[1] = { ...openTabsRef.current[1], content: 'unsaved typing' };
      return Response.json({ path: 'b.md', content: 'AI change', hash: 'hash-b' });
    };
    await actions.refreshWorkspaceAfterAssistantTools([patchEvent()]);
    assert.equal(openTabsRef.current[1].content, 'unsaved typing');
    assert.equal(openTabsRef.current[1].savedContent, 'old');
    assert.ok(openTabsRef.current[1].conflict);
  } finally { globalThis.fetch = originalFetch; }
});

test('mixed patch still refreshes an active file when it moves a different file', () => {
  const event = patchEvent({ moved: [{ old_path: 'b.md', new_path: 'renamed.md', status: 'success' }] });
  const plan = buildActiveFileToolPlan([event], { activePath: 'a.md' });
  assert.equal(plan.reloadPlan?.path, 'a.md');
});

test('patch deletion closes the active file instead of reloading a missing path', () => {
  const event = patchEvent({ moved: [{ old_path: 'a.md', new_path: '回收站/id/a.md', status: 'success' }] });
  const plan = buildActiveFileToolPlan([event], { activePath: 'a.md' });
  assert.equal(plan.activePath, '');
  assert.equal(plan.reloadPlan, null);
  assert.ok(plan.closeNotice);
});

test('a secondary active file never inherits the primary file hash', () => {
  const plan = buildActiveFileToolPlan([patchEvent()], { activePath: 'c.md' });
  assert.equal(plan.reloadPlan.hash, '');
});

test('deleting an active folder restores a surviving dirty snapshot without fetching over it', () => {
  const dirty = { path: 'kept.md', content: 'UNSAVED', savedContent: 'old', hash: 'kept-hash', conflict: { message: 'keep me' } };
  const openTabsRef = { current: [{ path: 'gone/a.md' }, { path: 'gone/b.md' }, dirty] };
  const currentFilePathRef = { current: 'gone/a.md' };
  let activated, closed = 0;
  const actions = createEditorToolRefreshActions({
    openTabsRef, currentFilePathRef,
    setOpenTabs: update => { openTabsRef.current = update(openTabsRef.current); },
    openTabSnapshot: snapshot => { activated = snapshot; currentFilePathRef.current = snapshot.path; },
    closeCurrentFile: () => { closed++; currentFilePathRef.current = ''; },
    setEditorStatusMessage: () => {},
  });
  actions.clearPathState('gone');
  assert.deepEqual(openTabsRef.current, [dirty]);
  assert.deepEqual(activated, dirty);
  assert.equal(currentFilePathRef.current, 'kept.md');
  actions.clearPathState('gone');
  assert.equal(closed, 0);
  actions.clearPathState('kept.md');
  assert.equal(closed, 1);
  assert.equal(currentFilePathRef.current, '');
});

test('a patch deleting the active file still refreshes the newly selected file with its own hash', async () => {
  const openTabsRef = { current: [{ path: 'a.md', content: 'a', savedContent: 'a' }, { path: 'b.md', content: 'b', savedContent: 'b' }] };
  const currentFilePathRef = { current: 'a.md' };
  const loads = [];
  const actions = createEditorToolRefreshActions({
    openTabsRef, currentFilePathRef, isDirtyRef: { current: false },
    setOpenTabs: update => { openTabsRef.current = update(openTabsRef.current); },
    openTabSnapshot: tab => { currentFilePathRef.current = tab.path; },
    setActiveTabPath: () => {}, setCurrentFilePath: () => {}, setCurrentFileType: () => {},
    setEditorStatusMessage: () => {}, setExternalFileConflict: () => {}, closeCurrentFile: () => {},
    loadFileIntoEditor: async (path, options) => { loads.push({ path, options }); },
  });
  await actions.refreshWorkspaceAfterAssistantTools([patchEvent({
    changed_paths: ['a.md', 'b.md'],
    moved: [{ old_path: 'a.md', new_path: '回收站/a.md', status: 'success' }],
    changes: [{ operation: 'delete', path: 'a.md' }, { operation: 'update', path: 'b.md', hash: 'b-new-hash' }],
  })]);
  assert.equal(currentFilePathRef.current, 'b.md');
  assert.equal(loads.length, 1);
  assert.equal(loads[0].path, 'b.md');
  assert.equal(loads[0].options.hash, 'b-new-hash');
});
