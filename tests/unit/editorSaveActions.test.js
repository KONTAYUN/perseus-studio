import assert from 'node:assert/strict';
import test from 'node:test';
import { createEditorSaveAction } from '../../src/hooks/editorSaveActions.js';
import { createEditorFileLoader } from '../../src/hooks/createEditorFileLoader.js';

function saveHarness() {
  const refs = {
    currentFilePathRef: { current: 'a.md' }, currentFileContentRef: { current: 'typed' },
    currentFileHashRef: { current: 'old-hash' }, savedFileContentRef: { current: 'original' },
    isTextFileRef: { current: true }, externalFileConflictRef: { current: null }, saveRequestRef: { current: null },
  };
  let tabs = [{ path: 'a.md', content: 'typed', savedContent: 'original' }];
  const save = createEditorSaveAction({
    ...refs, setIsSavingFile() {}, setErrorMessage() {}, setEditorStatusMessage() {},
    setSavedFileContent() {}, setCurrentFileHash() {}, setCurrentFileVersionId() {},
    setExternalFileConflict() {}, setOpenTabs(update) { tabs = update(tabs); },
  });
  return { save, refs, tabs: () => tabs, type(content) {
    refs.currentFileContentRef.current = content;
    tabs[0] = { ...tabs[0], content };
  } };
}

test('save sends expected hash and keeps typing made while awaiting response', async () => {
  const originalFetch = globalThis.fetch;
  try {
    const h = saveHarness();
    globalThis.fetch = async (_url, options) => {
      assert.equal(JSON.parse(options.body).expected_hash, 'old-hash');
      h.type('typed more');
      return Response.json({ hash: 'new-hash' });
    };
    await h.save();
    assert.equal(h.tabs()[0].content, 'typed more');
    assert.equal(h.tabs()[0].savedContent, 'typed');
    assert.equal(h.refs.savedFileContentRef.current, 'typed');
  } finally { globalThis.fetch = originalFetch; }
});

test('conflicting autosave keeps local content and blocks further automatic writes', async () => {
  const originalFetch = globalThis.fetch;
  const originalError = console.error;
  try {
    console.error = () => {};
    let calls = 0;
    globalThis.fetch = async () => { calls++; return Response.json({ error: 'changed externally' }, { status: 409 }); };
    const h = saveHarness();
    await assert.rejects(h.save({ auto: true }), /changed externally/);
    assert.equal(h.tabs()[0].content, 'typed');
    assert.equal(h.refs.externalFileConflictRef.current.reloadPath, 'a.md');
    h.refs.externalFileConflictRef.current.keepLocal = true;
    await h.save({ auto: true });
    assert.equal(calls, 1);
  } finally { globalThis.fetch = originalFetch; console.error = originalError; }
});

test('automatic reload preserves input entered during a file read', async () => {
  const originalFetch = globalThis.fetch;
  try {
    let replaced = false;
    let conflict;
    const contentRef = { current: 'original' };
    const load = createEditorFileLoader({
      editorLoadRequestRef: { current: 0 }, currentFileContentRef: contentRef, isDirtyRef: { current: false },
      setIsEditorLoading() {}, setExternalFileConflict(value) { conflict = value; },
      setEditorStatusMessage() {}, setCurrentFilePath() {}, setCurrentFileType() {},
      setCurrentFileHash() {}, setCurrentFileVersionId() {},
      setCurrentFileContent() { replaced = true; },
    });
    globalThis.fetch = async () => {
      contentRef.current = 'new local input';
      return Response.json({ path: 'a.md', content: 'AI change', hash: 'ai-hash' });
    };
    await load('a.md', { preserveSelection: true, preserveLocalEdits: true });
    assert.equal(replaced, false);
    assert.equal(conflict.hash, 'ai-hash');
  } finally { globalThis.fetch = originalFetch; }
});
