import assert from 'node:assert/strict';
import test from 'node:test';

import { getFileTreeDropZone } from '../../src/hooks/ideUtils.js';
import { nextAvailablePathInTree, topLevelSelectedItems } from '../../src/hooks/fileTreeUtils.js';

const rect = { top: 100, height: 40 };

test('getFileTreeDropZone returns before and after for row edges', () => {
  assert.equal(getFileTreeDropZone(105, rect, 'file'), 'before');
  assert.equal(getFileTreeDropZone(135, rect, 'file'), 'after');
});

test('getFileTreeDropZone returns inside only for folder row middle', () => {
  assert.equal(getFileTreeDropZone(120, rect, 'folder'), 'inside');
  assert.equal(getFileTreeDropZone(120, rect, 'file'), 'none');
});

test('nextAvailablePathInTree finds a non-conflicting numbered name', () => {
  const flatFiles = [{ path: 'docs/新建文件夹' }, { path: 'docs/新建文件夹 (2)' }];
  assert.equal(nextAvailablePathInTree(flatFiles, 'docs', '新建文件夹'), 'docs/新建文件夹 (3)');
});

test('topLevelSelectedItems removes descendants of selected folders', () => {
  const selected = [
    { path: 'docs', type: 'folder' },
    { path: 'docs/a.txt', type: 'file' },
    { path: 'other.txt', type: 'file' },
  ];
  assert.deepEqual(topLevelSelectedItems(selected), [
    { path: 'docs', type: 'folder' },
    { path: 'other.txt', type: 'file' },
  ]);
});
