import assert from 'node:assert/strict';
import test from 'node:test';
import { shouldStartFileTreeItemDrag } from '../../src/hooks/fileTreeDragUtils.js';

function targetMatching(match) {
  return {
    closest(selector) {
      return selector.includes(match) ? {} : null;
    },
  };
}

test('file tree row drag is disabled while inline renaming', () => {
  assert.equal(shouldStartFileTreeItemDrag({ target: targetMatching('span'), isInlineRenaming: true }), false);
});

test('file tree row drag starts from normal row content', () => {
  assert.equal(shouldStartFileTreeItemDrag({ target: { closest: () => null }, isInlineRenaming: false }), true);
});

test('file tree row drag does not start from rename input or action controls', () => {
  assert.equal(shouldStartFileTreeItemDrag({ target: targetMatching('input'), isInlineRenaming: false }), false);
  assert.equal(shouldStartFileTreeItemDrag({ target: targetMatching('button'), isInlineRenaming: false }), false);
  assert.equal(shouldStartFileTreeItemDrag({ target: targetMatching('data-file-tree-drag-suppressed'), isInlineRenaming: false }), false);
});
