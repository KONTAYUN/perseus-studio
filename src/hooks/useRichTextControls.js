import { useEffect, useState } from 'react';
import { selectedTable } from '../components/editor/richText/tableActions.js';

export default function useRichTextControls(editor, scrollRef, ownerId) {
  const [controls, setControls] = useState({ selection: null, block: null, table: null, revision: 0 });
  useEffect(() => {
    if (!editor) return undefined;
    let frame;
    const update = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        if (editor.isDestroyed || !scrollRef.current) return;
        const { selection } = editor.state;
        const viewport = scrollRef.current.getBoundingClientRect();
        const inside = editor.isFocused || document.activeElement?.closest('[data-rich-controls]')?.getAttribute('data-rich-controls') === ownerId;
        const from = editor.view.coordsAtPos(selection.from);
        const to = editor.view.coordsAtPos(selection.to);
        const visible = from.bottom > viewport.top && from.top < viewport.bottom;
        let block = null;
        const preserved = selection.node?.type.name?.startsWith('preserved');
        if (inside && visible && !preserved) {
          const $from = selection.$from;
          let depth = $from.depth;
          while (depth > 0 && !$from.node(depth).isTextblock) depth--;
          const node = depth > 0 ? editor.view.nodeDOM($from.before(depth)) : null;
          const rect = node?.getBoundingClientRect?.() || from;
          block = { top: rect.top - viewport.top + scrollRef.current.scrollTop, type: $from.parent.type.name };
        }
        setControls(current => ({
          revision: current.revision + 1, block,
          table: inside ? selectedTable(editor)?.node || null : null,
          selection: inside && visible && !selection.empty && !preserved && !editor.isActive('codeBlock') ? {
            left: Math.min(from.left, to.left), right: Math.max(from.right, to.right),
            top: Math.max(viewport.top, Math.min(from.top, to.top)), bottom: Math.max(from.bottom, to.bottom),
          } : null,
        }));
      });
    };
    for (const name of ['selectionUpdate', 'transaction', 'focus', 'blur']) editor.on(name, update);
    window.addEventListener('resize', update);
    window.addEventListener('scroll', update, true);
    const resizeObserver = new ResizeObserver(update);
    resizeObserver.observe(scrollRef.current);
    update();
    return () => {
      cancelAnimationFrame(frame);
      for (const name of ['selectionUpdate', 'transaction', 'focus', 'blur']) editor.off(name, update);
      window.removeEventListener('resize', update);
      window.removeEventListener('scroll', update, true);
      resizeObserver.disconnect();
    };
  }, [editor, scrollRef, ownerId]);
  return controls;
}
