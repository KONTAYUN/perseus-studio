import assert from 'node:assert/strict';
import test from 'node:test';

import {
  defaultMarkdownViewState,
  markdownViewStateFromTab,
  resolveWorkspaceMarkdownLink,
  resolveWorkspaceMarkdownPath,
} from '../../src/components/editor/workspaceMarkdownUtils.js';
import { buildMarkdownAssetUploadPath } from '../../src/hooks/noteTreeUtils.js';

test('Markdown files default to rich-text while other text files stay in source mode', () => {
  assert.equal(defaultMarkdownViewState('notes/story.md').markdownViewMode, 'rich-text');
  assert.equal(defaultMarkdownViewState('notes/story.markdown').markdownViewMode, 'rich-text');
  assert.equal(defaultMarkdownViewState('notes/story.txt').markdownViewMode, 'source');
});

test('Markdown tab view state keeps mode, independent scroll positions, and selection', () => {
  assert.deepEqual(markdownViewStateFromTab({
    path: 'notes/story.md',
    markdownViewMode: 'source',
    markdownSourceScrollTop: 120,
    markdownPreviewScrollTop: 360,
    markdownSelectionStart: 8,
    markdownSelectionEnd: 14,
  }), {
    markdownViewMode: 'source',
    markdownSourceScrollTop: 120,
    markdownRichTextScrollTop: 360,
    markdownSelectionStart: 8,
    markdownSelectionEnd: 14,
  });
});

test('legacy preview mode is read as rich-text mode', () => {
  assert.equal(markdownViewStateFromTab({ path: 'story.md', markdownViewMode: 'preview' }).markdownViewMode, 'rich-text');
});

test('Markdown image uploads stay beside the document in a hidden assets folder', () => {
  const uploadPath = buildMarkdownAssetUploadPath('docs/story.markdown', 'cover image.png');
  assert.match(uploadPath, /^docs\/\.assets\/story\/\d+-cover-image\.png$/);
});

test('workspace Markdown paths resolve relative to the current document without escaping root', () => {
  assert.equal(resolveWorkspaceMarkdownPath('docs/chapter/readme.md', '../images/shot.png'), 'docs/images/shot.png');
  assert.equal(resolveWorkspaceMarkdownPath('docs/readme.md', '/shared/info.md#part'), 'shared/info.md');
  assert.equal(resolveWorkspaceMarkdownPath('readme.md', '../../secret.txt'), '');
  assert.equal(resolveWorkspaceMarkdownPath('readme.md', '//example.com/file.md'), '');
});

test('workspace Markdown links separate external, local, and unsafe targets', () => {
  assert.deepEqual(resolveWorkspaceMarkdownLink('docs/readme.md', 'https://example.com'), {
    href: 'https://example.com',
    external: true,
  });
  assert.deepEqual(resolveWorkspaceMarkdownLink('docs/readme.md', './next.md'), {
    workspacePath: 'docs/next.md',
  });
  assert.equal(resolveWorkspaceMarkdownLink('docs/readme.md', 'javascript:alert(1)'), null);
});
