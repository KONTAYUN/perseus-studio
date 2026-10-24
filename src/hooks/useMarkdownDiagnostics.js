import { useEffect, useRef, useState } from 'react';
import { diagnoseMarkdown } from '../components/markdown/markdownDiagnostics.js';
import { submitMarkdownRepair } from './editorMarkdownRepair.js';
import { documentHealth } from '../components/editor/document/registry.js';

export default function useMarkdownDiagnostics({ ide, editor, enabled }) {
  const [result, setResult] = useState(null);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');
  const latest = useRef(null);
  const mounted = useRef(true);
  const submitting = useRef(false);
  latest.current = { ide, editor };
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
  const path = editor.currentFilePath;
  const content = editor.currentFileContent;
  useEffect(() => {
    const health = event => {
      if (event.detail.path !== path || event.detail.source !== content) return;
      setResult({ path, content, issues: [...diagnoseMarkdown(content), ...(event.detail.category === 'format_issue' ? event.detail.issues : [])] });
    };
    window.addEventListener('perseus:document-health', health);
    return () => window.removeEventListener('perseus:document-health', health);
  }, [path, content]);
  useEffect(() => {
    setError('');
    if (!enabled || editor.isEditorLoading) { setResult(null); return undefined; }
    const timer = window.setTimeout(() => {
      const health = documentHealth(path, content);
      setResult({ path, content, issues: [...diagnoseMarkdown(content), ...(health?.category === 'format_issue' ? health.issues : [])] });
    }, 350);
    return () => window.clearTimeout(timer);
  }, [path, content, enabled, editor.isEditorLoading]);

  function getCurrent() {
    if (!mounted.current) return null;
    const { ide: currentIde, editor: currentEditor } = latest.current;
    return {
      path: currentEditor.currentFilePath, content: currentEditor.currentFileContent,
      issues: result?.issues || [],
      conversationId: currentIde.currentConversationId || '', modelId: currentIde.selectedModel?.id || '',
      conflict: currentEditor.externalFileConflict,
    };
  }
  async function repair(explicitIssues) {
    if (submitting.current) return false;
    submitting.current = true; setPending(true); setError('');
    const snapshot = getCurrent();
    try {
      if (!snapshot) throw new Error('文稿已关闭，请重新打开后操作。');
      if (explicitIssues) snapshot.issues = explicitIssues;
      await submitMarkdownRepair({ snapshot, getCurrent,
        prepareFile: value => latest.current.editor.prepareCurrentFileForAgent(value),
        send: (prompt, options) => latest.current.ide.sendChatMessage(prompt, options),
      });
      return true;
    } catch (failure) {
      if (mounted.current) setError(failure.message || '无法提交修复，请重试。');
      latest.current.editor.setEditorStatusMessage?.(failure.message || '无法提交格式调整，请重试。');
      return false;
    } finally {
      submitting.current = false;
      if (mounted.current) setPending(false);
    }
  }
  useEffect(() => {
    const assist = event => { if (enabled && event.detail.path === path) repair([{ code: 'preserved_content', message: `请将指定的特殊内容调整为通用格式；定位内容：${event.detail.raw.slice(0, 600)}` }]); };
    window.addEventListener('perseus:document-assist', assist);
    return () => window.removeEventListener('perseus:document-assist', assist);
  }, [path, enabled]);
  return { issues: result?.path === path && result.content === content ? result.issues : [], error, pending, repair };
}
