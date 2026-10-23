import React, { useEffect, useLayoutEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { Bold, Code2, Italic, Link2, RemoveFormatting, Strikethrough, Unlink } from 'lucide-react';
import { IconButton, Input, Button } from '../../ui';
import { safeMarkdownUrl } from '../../markdown/markdownPolicy.js';

export default function SelectionToolbar({ editor, selection, ownerId }) {
  const ref = useRef(null);
  const [position, setPosition] = useState({ left: 12, top: 12 });
  const [linkOpen, setLinkOpen] = useState(false);
  const [url, setUrl] = useState('');
  const [error, setError] = useState('');
  useEffect(() => {
    if (!selection) { setLinkOpen(false); setError(''); }
  }, [selection]);
  useLayoutEffect(() => {
    if (!selection || !ref.current) return;
    const rect = ref.current.getBoundingClientRect();
    setPosition({
      left: Math.max(8, Math.min((selection.left + selection.right - rect.width) / 2, window.innerWidth - rect.width - 8)),
      top: selection.top > rect.height + 16 ? selection.top - rect.height - 8 : Math.min(selection.bottom + 8, window.innerHeight - rect.height - 8),
    });
  }, [selection, linkOpen, error]);
  if (!selection) return null;
  function applyLink(event) {
    event.preventDefault();
    const href = safeMarkdownUrl(url);
    if (!href) { setError('请输入有效链接，如 https://example.com'); return; }
    editor.chain().focus().extendMarkRange('link').setLink({ href }).run();
    setLinkOpen(false);
  }
  const actions = [
    ['粗体', Bold, 'bold', () => editor.chain().focus().toggleBold().run()],
    ['斜体', Italic, 'italic', () => editor.chain().focus().toggleItalic().run()],
    ['删除线', Strikethrough, 'strike', () => editor.chain().focus().toggleStrike().run()],
    ['行内代码', Code2, 'code', () => editor.chain().focus().toggleCode().run()],
  ];
  return createPortal(
    <div ref={ref} className="rich-selection-toolbar ui-popover-surface" style={position} role="toolbar" aria-label="选中文本格式" data-rich-controls={ownerId} data-testid="selection-toolbar"
      onKeyDown={event => { if (event.key === 'Escape') { event.preventDefault(); setLinkOpen(false); editor.commands.focus(); } }}>
      <div className="rich-toolbar-row" onMouseDown={event => event.preventDefault()}>
        {actions.map(([title, Icon, mark, action]) => <IconButton key={mark} title={title} active={editor.isActive(mark)} aria-pressed={editor.isActive(mark)} onClick={action}><Icon size={16} /></IconButton>)}
        <span className="rich-toolbar-divider" />
        <IconButton title="编辑链接" active={editor.isActive('link')} onClick={() => { setUrl(editor.getAttributes('link').href || ''); setLinkOpen(value => !value); setError(''); }}><Link2 size={16} /></IconButton>
        {editor.isActive('link') && <IconButton title="移除链接" onClick={() => editor.chain().focus().extendMarkRange('link').unsetLink().run()}><Unlink size={16} /></IconButton>}
        <IconButton title="清除文字格式" onClick={() => editor.chain().focus().unsetAllMarks().run()}><RemoveFormatting size={16} /></IconButton>
      </div>
      {linkOpen && <form className="rich-link-form" onSubmit={applyLink}>
        <Input autoFocus aria-label="链接地址" value={url} placeholder="https://example.com" onChange={event => { setUrl(event.target.value); setError(''); }} />
        <Button type="submit" size="xs">应用</Button>
        {error && <p role="alert">{error}</p>}
      </form>}
    </div>, document.body,
  );
}
