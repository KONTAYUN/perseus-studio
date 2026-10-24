import React, { lazy, Suspense, useCallback, useEffect, useRef, useState } from 'react';
import { AlertTriangle, FileType2, History, LoaderCircle } from 'lucide-react';
import { AnchoredPopover, Button, IconButton } from '../ui';
import useMarkdownDiagnostics from '../../hooks/useMarkdownDiagnostics.js';
const DocumentHistoryDialog = lazy(() => import('./DocumentHistoryDialog.jsx'));

export default function EditorDocumentActions({ ide, editor, markdown }) {
  const warningRef = useRef(null);
  const [open, setOpen] = useState(false);
  const [history, setHistory] = useState(false);
  const closeHistory = useCallback(() => setHistory(false), []);
  const { issues, pending, error, repair } = useMarkdownDiagnostics({ ide, editor, enabled: markdown });
  useEffect(() => setOpen(false), [editor.currentFilePath]);
  const convertible = editor.isTextFile && /\.txt$/i.test(editor.currentFilePath);
  if (!markdown && !convertible) return null;
  if (convertible) return (
    <div className="editor-document-actions" onClick={event => event.stopPropagation()}>
      <IconButton data-testid="editor-convert-markdown" title="转换为 Markdown（保留原文内容）"
        disabled={Boolean(editor.isEditorLoading || editor.externalFileConflict || editor.isConvertingFile)}
        aria-busy={editor.isConvertingFile} onClick={() => editor.convertToMarkdown()}>
        {editor.isConvertingFile ? <LoaderCircle size={16} className="animate-spin" /> : <FileType2 size={16} />}
      </IconButton>
    </div>
  );
  const blocked = !ide.selectedModel?.id || editor.externalFileConflict || editor.isEditorLoading;
  return (
    <div className="editor-document-actions" onClick={event => event.stopPropagation()}>
      <IconButton title="版本历史与格式建议" onClick={event => { event.currentTarget.focus(); setHistory(true); }}><History size={16} /></IconButton>
      {history && <Suspense fallback={<span role="status">正在读取版本…</span>}><DocumentHistoryDialog editor={editor} onClose={closeHistory} /></Suspense>}
      {issues.length > 0 && <>
        <IconButton ref={warningRef} variant="warning" data-testid="markdown-format-warning"
          title={`发现 ${issues.length} 处格式异常`} aria-expanded={open} aria-haspopup="dialog"
          onClick={() => setOpen(value => !value)}><AlertTriangle size={16} /></IconButton>
        <AnchoredPopover anchorRef={warningRef} open={open} onClose={() => setOpen(false)} width={340}
          estimatedHeight={300} maxHeight={420} testId="markdown-format-popover" className="markdown-format-popover">
          <div role="dialog" aria-label="Markdown 格式检查">
            <div className="markdown-format-title"><AlertTriangle size={16} />文稿格式检查</div>
            <p>这部分内容未能通过保存检查。是否让当前 Agent 提交格式调整建议？</p>
            <ul className="markdown-format-issues">{issues.map((issue, index) => <li key={`${issue.line}-${index}`}>
              <span>第 {issue.line} 行</span>{issue.message}
            </li>)}</ul>
            <p>助手只能读取和提交当前文稿的候选；可能影响正文的建议需要再次确认。版本变化时会停止旧修复。</p>
            {!ide.selectedModel?.id && <p className="markdown-format-warning-text">请先在聊天区选择可用模型。</p>}
            {editor.externalFileConflict && <p className="markdown-format-warning-text">请先处理文件保存冲突。</p>}
            {error && <p role="alert" className="markdown-format-warning-text">{error}</p>}
            <div className="markdown-format-buttons">
              <Button variant="secondary" size="xs" onClick={() => setOpen(false)}>暂不修复</Button>
              <Button size="xs" disabled={Boolean(blocked || pending)} onClick={async () => { if (await repair()) setOpen(false); }}>
                {pending && <LoaderCircle size={13} className="animate-spin" />}
                {pending ? '正在请求修复…' : ide.isChatSending ? '排队让 Agent 修复' : '让 Agent 修复'}
              </Button>
            </div>
          </div>
        </AnchoredPopover>
      </>}
    </div>
  );
}
