export function createHiddenBlockToolbarState() {
  return {
    triggerVisible: false,
    triggerPosition: null,
    toolbarVisible: false,
    toolbarPosition: null,
  };
}

function shallowSameObject(left, right) {
  if (left === right) return true;
  if (!left || !right) return !left && !right;
  const leftKeys = Object.keys(left);
  const rightKeys = Object.keys(right);
  if (leftKeys.length !== rightKeys.length) return false;
  return leftKeys.every((key) => left[key] === right[key]);
}

function sameToolbarState(left, right) {
  if (!left || !right) return left === right;
  const keys = new Set([...Object.keys(left), ...Object.keys(right)]);
  for (const key of keys) {
    if (key === 'position' || key === 'range' || key === 'triggerPosition' || key === 'toolbarPosition') {
      if (!shallowSameObject(left[key], right[key])) return false;
      continue;
    }
    if (left[key] !== right[key]) return false;
  }
  return true;
}

export function preserveToolbarState(current, next) {
  return sameToolbarState(current, next) ? current : next;
}

export function resolveBlockTriggerLeft(blockElement, blockRect, editorRect) {
  const listItem = blockElement?.closest?.('li');
  const listElement = listItem?.closest?.('ul, ol');
  const quoteElement = blockElement?.closest?.('blockquote');
  const anchorElement = listElement || quoteElement || blockElement;
  const anchorRect = anchorElement?.getBoundingClientRect?.() || blockRect;
  const needsExtraGutter = Boolean(listElement || quoteElement);
  const minLeft = Math.max(editorRect.left + (needsExtraGutter ? -18 : 6), 4);
  const maxLeft = editorRect.right - 38;
  const offset = needsExtraGutter ? 44 : 34;
  const anchorLeft = Math.min(blockRect.left, anchorRect.left);

  return Math.max(minLeft, Math.min(anchorLeft - offset, maxLeft));
}
