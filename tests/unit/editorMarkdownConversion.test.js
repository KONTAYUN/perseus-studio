import assert from 'node:assert/strict';
import test from 'node:test';
import { createMarkdownConversionAction } from '../../src/hooks/editorMarkdownConversion.js';
import { nextAvailablePathInTree } from '../../src/hooks/fileTreeUtils.js';

test('new Markdown names include existing files inside collapsed folders', () => {
  const tree = [{ path: 'docs', type: 'folder', isOpen: false, children: [
    { path: 'docs/新建文稿.md' }, { path: 'docs/新建文稿 (2).md' },
  ] }];
  assert.equal(nextAvailablePathInTree(tree, 'docs', '新建文稿.md'), 'docs/新建文稿 (3).md');
});

function harness(overrides = {}) {
  const ref = current => ({ current });
  const state = { busy: false, error: '', modes: {}, renamed: [] };
  const options = {
    conversionRequestRef: ref(null), autoSaveTimerRef: ref(null),
    currentFilePathRef: ref('资料/草稿.TXT'), currentFileContentRef: ref('# 标题\n\n未保存的正文\n'),
    savedFileContentRef: ref('old'), currentFileHashRef: ref('old-hash'),
    externalFileConflictRef: ref(null), isTextFileRef: ref(true),
    async saveFileContent() {
      options.savedFileContentRef.current = options.currentFileContentRef.current;
      options.currentFileHashRef.current = 'saved-hash';
    },
    async renamePath(...args) { state.renamed.push(args); },
    setIsConvertingFile(value) { state.busy = value; },
    setErrorMessage(value) { state.error = value; },
    setDocumentModes(update) { state.modes = update(state.modes); },
    ...overrides,
  };
  return { options, state, convert: createMarkdownConversionAction(options) };
}

test('conversion saves the draft first, uses its hash, and deduplicates clicks', async () => {
  const h = harness();
  const first = h.convert();
  assert.equal(h.state.busy, true);
  assert.equal(h.convert(), first);
  assert.equal(await first, true);
  assert.deepEqual(h.state.renamed, [['资料/草稿.TXT', '资料/草稿.md', { expectedHash: 'saved-hash' }]]);
  assert.equal(h.options.savedFileContentRef.current, '# 标题\n\n未保存的正文\n');
  assert.equal(h.state.modes['资料/草稿.md'], 'rich');
  assert.equal(h.state.busy, false);
  assert.equal(h.options.conversionRequestRef.current, null);
});

test('failed save or rename keeps source state and exposes a retryable error', async () => {
  for (const stage of ['saveFileContent', 'renamePath']) {
    const h = harness({ [stage]: async () => { throw new Error('failure'); } });
    assert.equal(await h.convert(), false);
    assert.equal(h.options.currentFilePathRef.current, '资料/草稿.TXT');
    assert.equal(h.options.currentFileContentRef.current, '# 标题\n\n未保存的正文\n');
    assert.deepEqual(h.state.renamed, []);
    assert.deepEqual(h.state.modes, {});
    assert.match(h.state.error, /failure/);
    assert.equal(h.state.busy, false);
  }
});

test('conversion aborts if path, content, or conflict changes during save', async () => {
  for (const [key, value] of [['currentFilePathRef', 'other.txt'], ['currentFileContentRef', 'new typing'], ['externalFileConflictRef', {}]]) {
    const h = harness();
    h.options.saveFileContent = async () => { h.options[key].current = value; };
    // Recreate after injecting the delayed-save behavior.
    assert.equal(await createMarkdownConversionAction(h.options)(), false);
    assert.deepEqual(h.state.renamed, []);
    assert.match(h.state.error, /停止转换/);
  }
});

test('Markdown, other text formats and conflicted files cannot convert', async () => {
  for (const path of ['draft.md', 'draft.csv', '']) {
    const h = harness(); h.options.currentFilePathRef.current = path;
    assert.equal(await h.convert(), false);
    assert.equal(h.state.busy, false);
  }
  const h = harness(); h.options.externalFileConflictRef.current = {};
  assert.equal(await h.convert(), false);
  assert.deepEqual(h.state.renamed, []);
});
