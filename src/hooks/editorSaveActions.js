import { apiGet, apiPost } from '../api/client.js';
import { flushDocumentSessions } from '../components/editor/document/registry.js';
import { DOCUMENT_PROFILE } from '../components/editor/document/profile.js';
import { isMarkdownFile } from '../components/markdown/markdownPolicy.js';

export function createEditorSaveAction({
  currentFilePathRef, currentFileContentRef, currentFileHashRef, savedFileContentRef,
  isTextFileRef, externalFileConflictRef, saveRequestRef,
  setIsSavingFile, setErrorMessage, setEditorStatusMessage, setSavedFileContent,
  setCurrentFileHash, setCurrentFileVersionId, setOpenTabs, setExternalFileConflict, loadTree,
}) {
  return async function saveFileContent({ auto = false } = {}) {
    await flushDocumentSessions(currentFilePathRef.current);
    while (saveRequestRef.current) await saveRequestRef.current;
    const path = currentFilePathRef.current;
    const content = currentFileContentRef.current;
    if (!path || !isTextFileRef.current || content === savedFileContentRef.current) return;
    const conflict = externalFileConflictRef.current;
    if (conflict && (auto || !conflict.keepLocal)) {
      setErrorMessage?.('文件已被修改，请先选择保留本地修改或重新加载文件');
      return;
    }
    setIsSavingFile(true);
    setErrorMessage?.('');
    setEditorStatusMessage('');
    const request = (async () => {
      try {
        const hash = conflict?.keepLocal
          ? (await apiGet(`/api/files/read?path=${encodeURIComponent(path)}`)).hash
          : currentFileHashRef.current;
        const result = await apiPost('/api/files/write', { path, content, expected_hash: hash, ...(isMarkdownFile(path) ? { profile_version: DOCUMENT_PROFILE } : {}) });
        if (currentFilePathRef.current === path) {
          savedFileContentRef.current = content;
          currentFileHashRef.current = result.hash || '';
          setSavedFileContent(content);
          setCurrentFileHash(result.hash || '');
          setCurrentFileVersionId(result.version_id || '');
          setExternalFileConflict(null);
          externalFileConflictRef.current = null;
          setEditorStatusMessage('');
        }
        // A save response acknowledges the submitted text; newer typing stays local.
        setOpenTabs((tabs) => tabs.map((tab) => tab.path === path ? {
          ...tab, savedContent: content, hash: result.hash || '',
          versionId: result.version_id || '', conflict: null, statusMessage: '',
        } : tab));
        await loadTree?.();
      } catch (error) {
        if (currentFilePathRef.current === path) {
          if (error.status === 409) {
            const nextConflict = { reloadPath: path, message: error.message };
            externalFileConflictRef.current = nextConflict;
            setExternalFileConflict(nextConflict);
          }
          setErrorMessage?.(error.message);
          setEditorStatusMessage(auto ? '保存失败' : '');
        }
        throw error;
      } finally {
        setIsSavingFile(false);
      }
    })();
    saveRequestRef.current = request;
    try { await request; } finally {
      if (saveRequestRef.current === request) saveRequestRef.current = null;
    }
  };
}
