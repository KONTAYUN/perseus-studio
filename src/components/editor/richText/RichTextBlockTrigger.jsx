import React from 'react';
import { createPortal } from 'react-dom';

export default function RichTextBlockTrigger({
  position,
  active,
  visible,
  onInteract,
  onToggle,
}) {
  if (!visible || !position || typeof document === 'undefined') return null;
  return createPortal(
    <button
      type="button"
      className={`rich-text-block-trigger fixed z-[1130] ${active ? 'is-active' : ''}`}
      style={{
        left: position.left,
        top: position.top,
      }}
      onMouseDown={(event) => {
        onInteract?.();
        event.preventDefault();
      }}
      onClick={onToggle}
      aria-label="打开行内工具栏"
      title="打开行内工具栏"
    >
      {Array.from({ length: 6 }).map((_, index) => (
        <span key={`dot-${index}`} className="rich-text-block-trigger-dot" />
      ))}
    </button>,
    document.body
  );
}
