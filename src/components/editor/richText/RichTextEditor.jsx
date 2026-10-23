import React, { useId, useRef } from 'react';
import { EditorContent, useEditor } from '@tiptap/react';
import { markdownExtensions, serializeMarkdown } from './markdownExtensions.js';
import { RichCodeBlock } from './RichCodeBlock.jsx';
import useRichTextControls from '../../../hooks/useRichTextControls.js';
import SelectionToolbar from './SelectionToolbar.jsx';
import BlockMenu from './BlockMenu.jsx';

export default function RichTextEditor({ content, onChange }) {
  const scrollRef = useRef(null);
  const ownerId = useId();
  const callback = useRef(onChange);
  callback.current = onChange;
  const original = useRef(content);
  const editor = useEditor({
    extensions: markdownExtensions(RichCodeBlock),
    content, contentType: 'markdown',
    editorProps: {
      attributes: { class: 'markdown-content rich-document-content', 'aria-label': 'Markdown 富文本编辑器', 'data-testid': 'rich-text-editor', role: 'textbox', 'aria-multiline': 'true' },
    },
    onUpdate: ({ editor: current }) => callback.current(serializeMarkdown(current, original.current)),
  });
  const controls = useRichTextControls(editor, scrollRef, ownerId);
  return (
    <div ref={scrollRef} className="rich-document-scroll custom-scrollbar" data-testid="rich-document-surface">
      <EditorContent editor={editor} />
      {editor && <>
        <SelectionToolbar editor={editor} selection={controls.selection} ownerId={ownerId} />
        <BlockMenu editor={editor} block={controls.block} ownerId={ownerId} />
      </>}
    </div>
  );
}
