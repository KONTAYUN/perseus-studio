import React, { useEffect, useRef, useState } from 'react';
import { AlertTriangle, Code2, FileText, LoaderCircle } from 'lucide-react';
import { AnchoredPopover, Button, IconButton } from '../ui';
import useMarkdownDiagnostics from '../../hooks/useMarkdownDiagnostics.js';

export default function EditorDocumentActions({ ide, editor, markdown, mode, onModeChange }) {
  const warningRef = useRef(null);
  const [open, setOpen] = useState(false);
  const { issues, pending, error, repair } = useMarkdownDiagnostics({ ide, editor, enabled: markdown });
  useEffect(() => setOpen(false), [editor.currentFilePath]);
  if (!markdown) return null;
  const source = mode === 'source';
  const blocked = !ide.selectedModel?.id || editor.externalFileConflict || editor.isEditorLoading;
  return (
    <div className="editor-document-actions" onClick={event => event.stopPropagation()}>
      {issues.length > 0 && <>
        <IconButton ref={warningRef} variant="warning" data-testid="markdown-format-warning"
          title={`发现 ${issues.length} 处疑似格式问题`} aria-expanded={open} aria-haspopup="dialog"
          onClick={() => setOpen(value => !value)}><AlertTriangle size={16} /></IconButton>
        <AnchoredPopover anchorRef={warningRef} open={open} onClose={() => setOpen(false)} width={340}
          estimatedHeight={300} maxHeight={420} testId="markdown-format-popover" className="markdown-format-popover">
          <div role="dialog" aria-label="Markdown 格式检查">
            <div className="markdown-format-title"><AlertTriangle size={16} />检查到疑似格式问题</div>
            <p>以下是格式检查线索，可能是合法写法。是否让当前 Agent 检查并修复？</p>
            <ul className="markdown-format-issues">{issues.map((issue, index) => <li key={`${issue.line}-${index}`}>
              <span>第 {issue.line} 行</span>{issue.message}
            </li>)}</ul>
            <p>修复前会保存当前版本；Agent 将先读取文件，尽量仅调整格式，保留正文和代码。</p>
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
      <IconButton data-testid="editor-mode-toggle" data-mode={mode}
        title={source ? '切换为富文本（当前：纯文本）' : '切换为纯文本（当前：富文本）'}
        onClick={() => onModeChange(source ? 'rich' : 'source')}>
        {source ? <FileText size={16} /> : <Code2 size={16} />}
      </IconButton>
    </div>
  );
}
