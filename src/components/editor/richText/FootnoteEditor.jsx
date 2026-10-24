import React, { useEffect } from 'react';
import { EditorContent, useEditor } from '@tiptap/react';
import { documentExtensions } from '../document/editorExtensions.js';
import { RichPreservedBlock, RichPreservedInline } from './PreservedContentView.jsx';
import { RichDocumentAtoms } from './DocumentAtomView.jsx';
import { Button } from '../../ui';

export default function FootnoteEditor({ content, onChange, onReady }) {
  const editor = useEditor({
    extensions: documentExtensions(null, [RichPreservedBlock, RichPreservedInline], RichDocumentAtoms),
    content: { type: 'doc', content },
    autofocus: 'end',
    editorProps: { attributes: { role: 'textbox', 'aria-label': '脚注内容', class: 'markdown-content rich-footnote-input custom-scrollbar' } },
    onUpdate: ({ editor }) => onChange(editor.getJSON().content),
  });
  useEffect(() => {
    if (!editor) return undefined;
    onReady?.(editor);
    return () => onReady?.(null);
  }, [editor, onReady]);
  return <div><span>脚注内容</span><div role="toolbar" aria-label="脚注格式">
    {['bold', 'italic', 'strike'].map((mark, i) => <Button key={mark} variant="ghost" size="xs" onClick={() => editor?.chain().focus().toggleMark(mark).run()}>{['加粗', '斜体', '删除线'][i]}</Button>)}
  </div><EditorContent editor={editor} /></div>;
}
