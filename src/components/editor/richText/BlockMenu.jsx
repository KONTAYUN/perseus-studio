import React, { useRef, useState } from 'react';
import { AlignLeft, ArrowDown, Braces, CheckSquare, Code2, GripVertical, Heading1, Heading2, Heading3, List, ListOrdered, Minus, Quote, Table2, Trash2 } from 'lucide-react';
import { AnchoredPopover, IconButton } from '../../ui';

export default function BlockMenu({ editor, block, ownerId }) {
  const anchor = useRef(null);
  const [open, setOpen] = useState(false);
  if (!block) return null;
  const actions = [
    ['正文', AlignLeft, editor.isActive('paragraph'), () => editor.chain().focus().clearNodes().setParagraph().run()],
    ['一级标题', Heading1, editor.isActive('heading', { level: 1 }), () => editor.chain().focus().toggleHeading({ level: 1 }).run()],
    ['二级标题', Heading2, editor.isActive('heading', { level: 2 }), () => editor.chain().focus().toggleHeading({ level: 2 }).run()],
    ['三级标题', Heading3, editor.isActive('heading', { level: 3 }), () => editor.chain().focus().toggleHeading({ level: 3 }).run()],
    ['无序列表', List, editor.isActive('bulletList'), () => editor.chain().focus().toggleBulletList().run()],
    ['有序列表', ListOrdered, editor.isActive('orderedList'), () => editor.chain().focus().toggleOrderedList().run()],
    ['任务列表', CheckSquare, editor.isActive('taskList'), () => editor.chain().focus().toggleTaskList().run()],
    ['引用', Quote, editor.isActive('blockquote'), () => editor.chain().focus().toggleBlockquote().run()],
    ['代码块', Code2, editor.isActive('codeBlock'), () => editor.chain().focus().toggleCodeBlock().run()],
    ['分隔线', Minus, false, () => editor.chain().focus().setHorizontalRule().run()],
    ['插入表格', Table2, false, () => editor.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run()],
  ];
  function execute(action) { action(); setOpen(false); }
  return (
    <>
      <IconButton ref={anchor} title="段落格式" data-testid="block-format-handle" className="rich-block-handle" style={{ top: block.top }}
        data-rich-controls={ownerId} aria-expanded={open} aria-haspopup="menu" active={open}
        onMouseDown={event => event.preventDefault()} onClick={() => setOpen(value => !value)}><GripVertical size={18} /></IconButton>
      <AnchoredPopover anchorRef={anchor} open={open} onClose={() => setOpen(false)} align="start" width={224} maxHeight={440} estimatedHeight={430} testId="block-format-menu" className="rich-block-menu">
        <div role="menu" aria-label="段落格式" data-rich-controls={ownerId} onMouseDown={event => event.preventDefault()}>
          <div className="rich-menu-caption"><Braces size={12} />段落格式</div>
          {actions.map(([label, Icon, active, action]) => (
            <button key={label} type="button" role="menuitem" className={`rich-menu-item motion-row ${active ? 'is-active' : ''}`} onClick={() => execute(action)}>
              <Icon size={15} /><span>{label}</span>{active && <span className="rich-menu-check">✓</span>}
            </button>
          ))}
          {editor.isActive('table') && <>
            <button type="button" role="menuitem" className="rich-menu-item motion-row" onClick={() => execute(() => editor.chain().focus().addRowAfter().run())}>在下方插入行</button>
            <button type="button" role="menuitem" className="rich-menu-item motion-row" onClick={() => execute(() => editor.chain().focus().addColumnAfter().run())}>在右侧插入列</button>
            <button type="button" role="menuitem" className="rich-menu-item motion-row" onClick={() => execute(() => editor.chain().focus().deleteTable().run())}>删除表格</button>
          </>}
          <div className="rich-menu-separator" />
          <button type="button" role="menuitem" className="rich-menu-item motion-row" onClick={() => execute(() => {
            const { $from } = editor.state.selection;
            const position = $from.depth ? $from.after(1) : editor.state.doc.content.size;
            editor.chain().focus().insertContentAt(position, { type: 'paragraph' }).setTextSelection(position + 1).run();
          })}><ArrowDown size={15} /><span>在下方插入段落</span></button>
          <button type="button" role="menuitem" className="rich-menu-item rich-menu-danger motion-row" onClick={() => execute(() => editor.chain().focus().deleteNode(editor.state.selection.$from.parent.type.name).run())}><Trash2 size={15} /><span>删除当前段落</span></button>
        </div>
      </AnchoredPopover>
    </>
  );
}
