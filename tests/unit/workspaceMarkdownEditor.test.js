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
  assert.match(source, /imageUploadHandler: handleImageUpload/);
  assert.match(source, /imagePreviewHandler: handleImagePreview/);
  assert.match(source, /linkDialogPlugin\(\{ LinkDialog: MarkdownLinkDialog \}\)/);
  assert.match(source, /markdownContextualToolsPlugin\(\)/);
  assert.match(source, /'uploadImage\.dialogTitle': '插入图片'/);
  assert.match(source, /'uploadImage\.uploadInstructions': '从设备上传图片'/);
  assert.match(source, /'dialogControls\.save': '保存'/);
  assert.match(source, /'dialogControls\.cancel': '取消'/);
  assert.match(source, /'createLink\.urlPlaceholder': '粘贴或输入链接地址'/);
  assert.match(source, /'linkPreview\.copyToClipboard': '复制链接地址'/);
});

test('workspace Markdown editor keeps contextual tools in a dedicated plugin surface', async () => {
  const contextualToolsSource = await readFile(
    new URL('../../src/components/editor/markdown/MarkdownContextualTools.jsx', import.meta.url),
    'utf8'
  );
  assert.match(contextualToolsSource, /realmPlugin/);
  assert.match(contextualToolsSource, /addComposerChild\$/);
  assert.match(contextualToolsSource, /currentSelection\$/);
  assert.match(contextualToolsSource, /CreateLink/);
  assert.match(contextualToolsSource, /BlockTypeSelect/);
  assert.match(contextualToolsSource, /MarkdownCodeLanguageSelect/);
  assert.match(contextualToolsSource, /normalizeCodeBlockLanguages/);
  assert.match(contextualToolsSource, /markdown-code-copy/);
  assert.match(contextualToolsSource, /markdown-code-delete/);
  assert.match(contextualToolsSource, /getNearestEditorFromDOMNode/);
  assert.match(contextualToolsSource, /getElementByKey/);
  assert.match(contextualToolsSource, /publishActiveEditor/);
  assert.match(source, /codeMirrorPlugin\(/);
});

test('workspace Markdown editor does not feed every change back through the markdown prop', () => {
  assert.equal((source.match(/markdown=\{String\(content/g) || []).length, 1);
  assert.equal((source.match(/setMarkdown\(nextMarkdown\)/g) || []).length, 1);
  assert.match(source, /if \(syncingExternalMarkdownRef\.current \|\| initialMarkdownNormalize \|\| readOnlyRef\.current\) return/);
});

test('workspace Markdown editor keeps contextual controls stable and removes the fixed history toolbar', async () => {
  assert.doesNotMatch(source, /UndoRedo|toolbarPlugin/);
  assert.match(source, /React\.memo\(WorkspaceMarkdownEditor\)/);
  assert.match(source, /const StableMarkdownEditor = React\.memo\(/);
  assert.match(source, /previous\.readOnly === next\.readOnly/);
  assert.match(source, /translation=\{translateMarkdownEditor\}/);
  assert.match(source, /overlayContainer=\{overlayContainer\}/);
  assert.match(source, /workspace-markdown-editor-overlay-root/);

  const contextualToolsSource = await readFile(
    new URL('../../src/components/editor/markdown/MarkdownContextualTools.jsx', import.meta.url),
    'utf8'
  );
  const linkDialogSource = await readFile(
    new URL('../../src/components/editor/markdown/MarkdownLinkDialog.jsx', import.meta.url),
    'utf8'
  );
  assert.match(contextualToolsSource, /editorRootElementRef\$/);
  assert.match(contextualToolsSource, /document\.addEventListener\('focusin'/);
  assert.match(contextualToolsSource, /document\.addEventListener\('selectionchange'/);
  assert.match(contextualToolsSource, /isSameGeometry/);
  assert.match(contextualToolsSource, /CodeMirrorFocusBridge/);
  assert.match(contextualToolsSource, /rootNode\?\.getCode/);
  assert.match(contextualToolsSource, /createPortal\([\s\S]*popupHost\n  \);/);
  assert.doesNotMatch(contextualToolsSource, /popupHost \|\| document\.body/);
  assert.doesNotMatch(contextualToolsSource, /console\.debug/);
  assert.match(linkDialogSource, /editorRootElementRef\$/);
  assert.match(linkDialogSource, /createPortal\([\s\S]*popupHost\n  \);/);
  assert.doesNotMatch(linkDialogSource, /document\.body/);
});
