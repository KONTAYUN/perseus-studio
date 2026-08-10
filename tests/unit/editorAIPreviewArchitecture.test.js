import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');

function source(relativePath) {
  return fs.readFileSync(path.join(root, relativePath), 'utf8');
}

test('AI note previews use block transactions instead of whole-document replacement', () => {
  const transactions = source(
    'src/components/editor/richText/richTextAITransactions.js'
  );
  const sync = source('src/components/editor/richText/useRichTextDocumentSync.js');

  assert.match(transactions, /transaction\.setMeta\('addToHistory', false\)/);
  assert.match(transactions, /transaction\.setMeta\('aiPreview', true\)/);
  assert.match(sync, /applyAIBlockChanges/);
  assert.doesNotMatch(transactions, /\.setContent\(/);
});

test('authoritative Agent commits advance the revision baseline without a timing lock', () => {
  const tabs = source('src/hooks/useEditorTabs.js');
  const loader = source('src/hooks/createEditorFileLoader.js');

  assert.match(tabs, /userRevision > savedUserRevision/);
  assert.match(tabs, /applyAuthoritativeFileSnapshot/);
  assert.match(tabs, /status: 'reconciling'/);
  assert.match(tabs, /acknowledgeAIEditReconciled/);
  assert.match(tabs, /setSavedUserRevisionState\(nextRevision\)/);
  assert.doesNotMatch(tabs, /authoritativeSyncRef/);
  assert.match(loader, /currentFileContentRef\.current = file\.content/);
  assert.match(loader, /savedFileContentRef\.current = file\.content/);
  assert.match(loader, /isDirtyRef\.current = false/);
  assert.match(loader, /setAIEditPreview\?\.\(null\)/);
  assert.doesNotMatch(loader, /requestAnimationFrame/);
});

test('rich text preview lifecycle preserves scroll anchors and restores reverted content', () => {
  const sync = source('src/components/editor/richText/useRichTextDocumentSync.js');
  const transactions = source(
    'src/components/editor/richText/richTextAITransactions.js'
  );

  assert.match(sync, /activePreviewIDRef/);
  assert.match(sync, /applyAuthoritativeEditorHTML/);
  assert.match(sync, /onAIEditReconciled\?\.\(aiPreview\.preview_id\)/);
  assert.match(sync, /buildProgressiveHTMLFrames/);
  assert.match(sync, /restoreEditorScrollAnchor\(editor, anchor\)/);
  assert.match(transactions, /transactionOrigin/);
  assert.match(transactions, /editorBlockElementKey/);
  assert.match(transactions, /selection: \{/);
  assert.match(transactions, /restoreSelection\(editor, anchor\.selection\)/);
});
