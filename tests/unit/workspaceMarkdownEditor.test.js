import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const source = await readFile(new URL('../../src/components/editor/WorkspaceMarkdownEditor.jsx', import.meta.url), 'utf8');

test('workspace Markdown editor uses MDXEditor and keeps Markdown as the canonical value', () => {
  assert.match(source, /MDXEditor/);
  assert.match(source, /markdown=\{String\(content/);
  assert.match(source, /onChange=\{handleChange\}/);
  assert.match(source, /editorRef\.current\.setMarkdown\(nextMarkdown\)/);
});

test('workspace Markdown editor uses structured plugins for GFM and images', () => {
  for (const plugin of ['headingsPlugin', 'listsPlugin', 'tablePlugin', 'imagePlugin', 'codeBlockPlugin']) {
    assert.match(source, new RegExp(`${plugin}\\(`));
  }
  assert.match(source, /imageUploadHandler: onImageUpload \|\| null/);
  assert.match(source, /imagePreviewHandler: handleImagePreview/);
});

test('workspace Markdown editor does not feed every change back through the markdown prop', () => {
  assert.equal((source.match(/markdown=\{String\(content/g) || []).length, 1);
  assert.equal((source.match(/setMarkdown\(nextMarkdown\)/g) || []).length, 1);
  assert.match(source, /if \(syncingExternalMarkdownRef\.current \|\| initialMarkdownNormalize \|\| readOnly\) return/);
});
