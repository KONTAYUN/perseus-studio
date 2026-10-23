import { useEffect, useRef, useState } from 'react';
import { diagnoseMarkdown } from '../components/markdown/markdownDiagnostics.js';
import { submitMarkdownRepair } from './editorMarkdownRepair.js';

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
    setError('');
    if (!enabled || editor.isEditorLoading) { setResult(null); return undefined; }
    const timer = window.setTimeout(() => setResult({ path, content, issues: diagnoseMarkdown(content) }), 350);
    return () => window.clearTimeout(timer);
  }, [path, content, enabled, editor.isEditorLoading]);

  function getCurrent() {
    if (!mounted.current) return null;
    const { ide: currentIde, editor: currentEditor } = latest.current;
    return {
      path: currentEditor.currentFilePath, content: currentEditor.currentFileContent,
      conversationId: currentIde.currentConversationId || '', modelId: currentIde.selectedModel?.id || '',
      conflict: currentEditor.externalFileConflict,
    };
  }
  async function repair() {
    if (submitting.current) return false;
    submitting.current = true; setPending(true); setError('');
    const snapshot = getCurrent();
    try {
      await submitMarkdownRepair({ snapshot, getCurrent,
        prepareFile: value => latest.current.editor.prepareCurrentFileForAgent(value),
        send: prompt => latest.current.ide.sendChatMessage(prompt),
      });
      return true;
    } catch (failure) {
      if (mounted.current) setError(failure.message || '无法提交修复，请重试。');
      return false;
    } finally {
      submitting.current = false;
      if (mounted.current) setPending(false);
    }
  }
  return { issues: result?.path === path && result.content === content ? result.issues : [], error, pending, repair };
}
