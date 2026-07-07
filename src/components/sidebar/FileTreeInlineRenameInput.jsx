import React from 'react';

export default function InlineRenameInput({ item, ide }) {
  const inputRef = React.useRef(null);

  React.useEffect(() => {
    const input = inputRef.current;
    if (!input) return;
    input.focus();
    const dotIndex = item.type === 'file' ? ide.inlineRenameDraft.lastIndexOf('.') : -1;
    const selectionEnd = dotIndex > 0 ? dotIndex : ide.inlineRenameDraft.length;
    input.setSelectionRange(0, selectionEnd);
  }, [item.path, item.type]);

  return (
    <input
      ref={inputRef}
      data-testid={`inline-rename-${item.path}`}
      data-file-tree-drag-suppressed="true"
      draggable={false}
      value={ide.inlineRenameDraft}
      onChange={(e) => ide.setInlineRenameDraft(e.target.value)}
      onClick={(e) => e.stopPropagation()}
      onMouseDown={(e) => e.stopPropagation()}
      onDragStart={(e) => {
        e.stopPropagation();
      }}
      onBlur={() => {
        ide.commitInlineRename?.().catch((error) => console.error(error));
      }}
      onKeyDown={(e) => {
        if (e.key === 'Enter') {
          e.preventDefault();
          ide.commitInlineRename?.().catch((error) => console.error(error));
        }
        if (e.key === 'Escape') {
          e.preventDefault();
          ide.cancelInlineRename?.();
        }
      }}
      className="inline-rename-input settings-expand-enter min-w-0 flex-1 rounded-md border border-blue-400/80 bg-white px-1.5 py-0.5 text-[13px] text-gray-800 outline-none ring-2 ring-blue-500/15 dark:bg-[#252527] dark:text-gray-100"
    />
  );
}
