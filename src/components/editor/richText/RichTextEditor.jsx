import React, { useEffect, useId, useRef, useState } from 'react';
import { EditorContent, useEditor } from '@tiptap/react';
import { documentExtensions } from '../document/editorExtensions.js';
import useDocumentSession from '../../../hooks/useDocumentSession.js';
import { RichDocumentAtoms } from './DocumentAtomView.jsx';
import DocumentNodeDialog from './DocumentNodeDialog.jsx';
import { RichCodeBlock } from './RichCodeBlock.jsx';
import useRichTextControls from '../../../hooks/useRichTextControls.js';
import SelectionToolbar from './SelectionToolbar.jsx';
import BlockMenu from './BlockMenu.jsx';
import TableToolbar from './TableToolbar.jsx';
import { RichPreservedBlock, RichPreservedInline } from './PreservedContentView.jsx';
import { Button, Input } from '../../ui';

export default function RichTextEditor({ content, onChange, path, userId }) {
  const document = useDocumentSession(content, onChange, path, userId);
  const changePage = page => document.navigate(page).catch(() => {});
  return <div className="rich-document-editor">
    {document.failure && <div className="rich-document-node-error" role="alert">{document.failure}<Button size="xs" variant="secondary" onClick={document.retry}>重试</Button></div>}
    {!!document.archives.length && <div className="rich-section-toolbar"><select aria-label="浏览器历史草稿" value="" onChange={event => document.selectRecovery(document.archives[Number(event.target.value)])}>
      <option value="">查看保留的浏览器草稿</option>{document.archives.map((draft, index) => <option key={draft.at} value={index}>{new Date(draft.at).toLocaleString()}</option>)}
    </select></div>}
    {document.recovery && <div className="rich-section-toolbar" role="status">有尚未写入文件的编辑草稿。
      <Button size="xs" variant="secondary" onClick={document.recover}>恢复草稿</Button>
      <Button size="xs" variant="ghost" onClick={() => {
        const text = JSON.stringify(document.recovery, null, 2), link = window.document.createElement('a');
        link.href = URL.createObjectURL(new Blob([text], { type: 'application/json' })); link.download = '文稿草稿.json'; link.click(); URL.revokeObjectURL(link.href);
      }}>导出草稿</Button>
    </div>}
    {document.view ? <>
      {document.view.pages > 1 && <div className="rich-section-toolbar rich-pager" role="group" aria-label="长文档分段编辑">
        <span title="长文稿自动分段显示；各段之间的全文引用与撤销历史保持连续">第 {document.view.page + 1} / {document.view.pages} 段 · 长文稿自动分段</span>
        <Button size="xs" variant="ghost" disabled={!document.view.page} onClick={() => changePage(document.view.page - 1)}>上一段</Button>
        <Button size="xs" variant="ghost" disabled={document.view.page + 1 === document.view.pages} onClick={() => changePage(document.view.page + 1)}>下一段</Button>
        <form className="rich-pager-form" onSubmit={event => { event.preventDefault(); const page = Number(new FormData(event.currentTarget).get('page')); if (page >= 1 && page <= document.view.pages) changePage(page - 1); }}>
          <Input key={document.view.page} name="page" aria-label="跳转到第几段" type="number" min="1" max={document.view.pages} defaultValue={document.view.page + 1} />
          <Button size="xs" variant="ghost" type="submit">前往</Button>
        </form>
      </div>}
      <RichTextPage key={document.view.page} document={document} path={path} />
    </> : <div className="rich-document-loading">正在打开文稿…</div>}
  </div>;
}

function RichTextPage({ document, path }) {
  const scrollRef = useRef(null), ownerId = useId();
  const [notice, setNotice] = useState('');
  const editor = useEditor({
    extensions: documentExtensions(RichCodeBlock, [RichPreservedBlock, RichPreservedInline], RichDocumentAtoms, path),
    content: document.view.doc,
    editorProps: {
      handleKeyDown(_view, event) {
        if ((event.ctrlKey || event.metaKey) && ['z', 'y'].includes(event.key.toLowerCase())) {
          event.preventDefault(); document.undo(event.shiftKey || event.key.toLowerCase() === 'y').catch(() => {}); return true;
        }
        return false;
      },
      handleDoubleClickOn(_view, pos, node) {
        if (node.type.name !== 'image') return false;
        document.editorRef.current?.emit('documentEdit', { kind: 'image', pos, attrs: node.attrs }); return true;
      },
      attributes: { class: 'markdown-content rich-document-content', 'aria-label': 'Markdown 富文本编辑器', 'data-testid': 'rich-text-editor', role: 'textbox', 'aria-multiline': 'true' },
    },
    onUpdate: ({ editor: current }) => document.update(current),
  });
  document.editorRef.current = editor;
  useEffect(() => {
    if (!editor) return;
    const assist = detail => window.dispatchEvent(new CustomEvent('perseus:document-assist', { detail: { ...detail, path } }));
    editor.on('documentAssist', assist); return () => editor.off('documentAssist', assist);
  }, [editor, path]);
  useEffect(() => { if (!editor) return; editor.on('documentNotice', setNotice); return () => editor.off('documentNotice', setNotice); }, [editor]);
  const controls = useRichTextControls(editor, scrollRef, ownerId);
  return <>
    {notice && <div className="rich-section-toolbar" role="status">{notice}<Button variant="ghost" size="xs" onClick={() => setNotice('')}>知道了</Button></div>}
    {editor && controls.table && <TableToolbar editor={editor} table={controls.table} ownerId={ownerId} />}
    <div ref={scrollRef} className="rich-document-scroll custom-scrollbar" data-testid="rich-document-surface">
      <EditorContent editor={editor} />
      {editor && <><SelectionToolbar editor={editor} selection={controls.selection} ownerId={ownerId} />
        <BlockMenu editor={editor} block={controls.block} ownerId={ownerId} /><DocumentNodeDialog editor={editor} controller={document} path={path} />
      </>}
    </div>
  </>;
}
