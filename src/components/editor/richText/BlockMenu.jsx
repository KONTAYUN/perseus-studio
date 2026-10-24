import React, { useEffect, useRef, useState } from 'react';
import { AlignLeft, ArrowDown, Braces, CheckSquare, Code2, GripVertical, Heading1, Heading2, Heading3, Heading4, Heading5, Heading6, List, ListOrdered, Minus, Quote, Table2, Trash2 } from 'lucide-react';
import { AnchoredPopover, IconButton } from '../../ui';
import { paragraphOutsideTable, runTableAction, selectedTable, tableActionGroups } from './tableActions.js';

export default function BlockMenu({ editor, block, ownerId }) {
  const anchor = useRef(null);
  const [open, setOpen] = useState(false);
  const hasBlock = Boolean(block);
  // Hiding the handle on blur/scroll must also discard its open menu state.
  useEffect(() => {
    if (!hasBlock) setOpen(false);
  }, [hasBlock]);
  useEffect(() => {
    if (!open) return undefined;
    const close = () => setOpen(false);
    editor.on('selectionUpdate', close);
    editor.on('update', close);
    return () => {
      editor.off('selectionUpdate', close);
      editor.off('update', close);
    };
  }, [editor, open]);
  if (!block) return null;
  const table = selectedTable(editor)?.node;
  const actions = table ? [] : [
    ['正文', AlignLeft, editor.isActive('paragraph'), () => editor.chain().focus().clearNodes().setParagraph().run()],
    ...[['一级标题', Heading1], ['二级标题', Heading2], ['三级标题', Heading3], ['四级标题', Heading4], ['五级标题', Heading5], ['六级标题', Heading6]]
      .map(([label, Icon], index) => [label, Icon, editor.isActive('heading', { level: index + 1 }), () => editor.chain().focus().toggleHeading({ level: index + 1 }).run()]),
    ['无序列表', List, editor.isActive('bulletList'), () => editor.chain().focus().toggleBulletList().run()],
    ['有序列表', ListOrdered, editor.isActive('orderedList'), () => editor.chain().focus().toggleOrderedList().run()],
    ['任务列表', CheckSquare, editor.isActive('taskList'), () => editor.chain().focus().toggleTaskList().run()],
    ['引用', Quote, editor.isActive('blockquote'), () => editor.chain().focus().toggleBlockquote().run()],
    ['代码块', Code2, editor.isActive('codeBlock'), () => editor.chain().focus().toggleCodeBlock().run()],
    ['分隔线', Minus, false, () => editor.chain().focus().setHorizontalRule().run()],
    ['插入表格', Table2, false, () => editor.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run()],
    ...[['图片', 'image'], ['脚注', 'footnoteReference'], ['行内公式', 'inlineMath'], ['独立公式', 'blockMath'], ['提示段落', 'callout']].map(([label, kind]) => [label, Braces, false, () => editor.emit('documentEdit', { kind })]),
  ];
  function execute(action) { action(); setOpen(false); }
  return (
    <>
      <IconButton ref={anchor} title="段落格式" data-testid="block-format-handle" className="rich-block-handle" style={{ top: block.top }}
        data-rich-controls={ownerId} aria-expanded={open} aria-haspopup="menu" active={open}
        onMouseDown={event => event.preventDefault()} onClick={() => setOpen(value => !value)}><GripVertical size={18} /></IconButton>
      <AnchoredPopover anchorRef={anchor} open={open} onClose={() => setOpen(false)} showBackdrop={false} align="start" width={224} maxHeight={440} estimatedHeight={430} testId="block-format-menu" className="rich-block-menu">
        <div role="menu" aria-label="段落格式" data-rich-controls={ownerId} onMouseDown={event => event.preventDefault()}>
          <div className="rich-menu-caption"><Braces size={12} />{table ? '表格操作' : '段落格式'}</div>
          {actions.map(([label, Icon, active, action]) => (
            <button key={label} type="button" role="menuitem" className={`rich-menu-item motion-row ${active ? 'is-active' : ''}`} onClick={() => execute(action)}>
              <Icon size={15} /><span>{label}</span>{active && <span className="rich-menu-check">✓</span>}
            </button>
          ))}
          {table && <>
            {Object.values(tableActionGroups(table)).flat().map(([label, command, disabled]) =>
              <button key={command} type="button" role="menuitem" disabled={Boolean(disabled)} className="rich-menu-item motion-row"
                onClick={() => execute(() => runTableAction(editor, command))}>{label}</button>)}
            <button type="button" role="menuitem" className="rich-menu-item motion-row" onClick={() => execute(() => paragraphOutsideTable(editor, true))}>在表格前插入段落</button>
            <button type="button" role="menuitem" className="rich-menu-item rich-menu-danger motion-row" onClick={() => execute(() => runTableAction(editor, 'deleteTable'))}>删除表格</button>
          </>}
          <div className="rich-menu-separator" />
          <button type="button" role="menuitem" className="rich-menu-item motion-row" onClick={() => execute(() => {
            if (table) return paragraphOutsideTable(editor);
            const { $from } = editor.state.selection;
            const position = $from.depth ? $from.after(1) : editor.state.doc.content.size;
            editor.chain().focus().insertContentAt(position, { type: 'paragraph' }).setTextSelection(position + 1).run();
          })}><ArrowDown size={15} /><span>在下方插入段落</span></button>
          {!table && <button type="button" role="menuitem" className="rich-menu-item rich-menu-danger motion-row" onClick={() => execute(() => editor.chain().focus().deleteNode(editor.state.selection.$from.parent.type.name).run())}><Trash2 size={15} /><span>删除当前段落</span></button>}
        </div>
      </AnchoredPopover>
    </>
  );
}
