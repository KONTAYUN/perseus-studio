import test from 'node:test';
import assert from 'node:assert/strict';
import {
  buildNoteAssetUploadPath,
  buildNotePath,
  filterNoteTree,
  getNoteAssetReferencePath,
  getNoteTitle,
  isNotePath,
  resolveNoteAssetPath,
} from '../../src/hooks/noteTreeUtils.js';

test('isNotePath and getNoteTitle hide html extensions', () => {
  assert.equal(isNotePath('novel/chapter-1.html'), true);
  assert.equal(isNotePath('novel/chapter-1.md'), false);
  assert.equal(isNotePath('novel/chapter-1.txt'), false);
  assert.equal(getNoteTitle('novel/chapter-1.html'), 'chapter-1');
});

test('buildNotePath normalizes note names', () => {
  assert.equal(buildNotePath('世界观设定'), '世界观设定.html');
  assert.equal(buildNotePath('人物卡.html'), '人物卡.html');
});

test('filterNoteTree keeps only note files and visible folders', () => {
  const tree = [
    {
      path: 'notes',
      name: 'notes',
      type: 'folder',
      children: [
        { path: 'notes/chapter-1.html', name: 'chapter-1.html', type: 'file' },
        { path: 'notes/legacy.md', name: 'legacy.md', type: 'file' },
        { path: 'notes/reference.txt', name: 'reference.txt', type: 'file' },
        {
          path: 'notes/.assets',
          name: '.assets',
          type: 'folder',
          children: [{ path: 'notes/.assets/image.png', name: 'image.png', type: 'file' }],
        },
      ],
    },
  ];
  const filtered = filterNoteTree(tree);
  assert.equal(filtered.length, 1);
  assert.deepEqual(filtered[0].children.map((item) => item.path), ['notes/chapter-1.html']);
});

test('note asset helpers preserve relative references', () => {
  const uploadPath = buildNoteAssetUploadPath('novel/chapter-1.html', 'cover.png');
  assert.match(uploadPath, /^novel\/\.assets\/chapter-1\//);
  const referencePath = getNoteAssetReferencePath('novel/chapter-1.html', 'novel/.assets/chapter-1/cover.png');
  assert.equal(referencePath, '.assets/chapter-1/cover.png');
  assert.equal(resolveNoteAssetPath('novel/chapter-1.html', referencePath), 'novel/.assets/chapter-1/cover.png');
});
