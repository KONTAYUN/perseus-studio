export function shouldStartFileTreeItemDrag({ target, isInlineRenaming } = {}) {
  if (isInlineRenaming) return false;
  if (!target || typeof target.closest !== 'function') return true;
  return !target.closest('input, textarea, select, button, label, [contenteditable="true"], [data-file-tree-drag-suppressed="true"]');
}
