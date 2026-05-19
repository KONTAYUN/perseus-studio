import { apiGet } from '../api/client';
import { detectFileType } from './ideUtils';

export function createEditorFileLoader({
  editorLoadRequestRef,
  setActivePanel,
  setActiveTabPath,
  setCurrentFileContent,
  setCurrentFileHash,
  setCurrentFileNotice,
  setCurrentFilePath,
  setCurrentFileType,
  setCurrentFileVersionId,
  setErrorMessage,
  setExternalFileConflict,
  setIsEditorLoading,
  setOpenTabs,
  setSavedFileContent,
  setSelectedItemPath,
  setSelectedItemType,
  setEditorStatusMessage,
}) {
  async function loadFileIntoEditor(path, options = {}) {
    const {
      activatePanel = true,
      preserveSelection = false,
      hash = '',
      versionId = '',
      statusMessage = '',
      clearConflict = true,
    } = options;

    const requestId = ++editorLoadRequestRef.current;
    setIsEditorLoading(true);
    setErrorMessage?.('');
    if (clearConflict) {
      setExternalFileConflict(null);
    }
    setEditorStatusMessage(statusMessage);
    if (!preserveSelection) {
      setSelectedItemPath?.(path);
      setSelectedItemType?.('file');
    }

    try {
      const fileType = detectFileType(path);
      setCurrentFilePath(path);
      setCurrentFileType(fileType);
      setCurrentFileHash(hash || '');
      setCurrentFileVersionId(versionId || '');

      if (fileType === 'image') {
        if (requestId !== editorLoadRequestRef.current) return;
        setCurrentFileContent('');
        setSavedFileContent('');
        setCurrentFileNotice('图片文件暂不支持编辑');
        setActiveTabPath(path);
        upsertOpenTab({
          path,
          content: '',
          savedContent: '',
          hash: hash || '',
          versionId: versionId || '',
          fileType,
          notice: '图片文件暂不支持编辑',
          conflict: null,
          statusMessage,
        });
        if (activatePanel) setActivePanel?.('editor');
        return;
      }

      if (fileType === 'binary') {
        if (requestId !== editorLoadRequestRef.current) return;
        setCurrentFileContent('');
        setSavedFileContent('');
        setCurrentFileNotice('该文件类型暂不支持预览或编辑');
        setActiveTabPath(path);
        upsertOpenTab({
          path,
          content: '',
          savedContent: '',
          hash: hash || '',
          versionId: versionId || '',
          fileType,
          notice: '该文件类型暂不支持预览或编辑',
          conflict: null,
          statusMessage,
        });
        if (activatePanel) setActivePanel?.('editor');
        return;
      }

      const file = await apiGet(`/api/files/read?path=${encodeURIComponent(path)}`);
      if (requestId !== editorLoadRequestRef.current) return;
      setCurrentFilePath(file.path);
      setCurrentFileNotice('');
      setCurrentFileContent(file.content);
      setSavedFileContent(file.content);
      setCurrentFileHash(hash || file.hash || '');
      setCurrentFileVersionId(versionId || '');
      setActiveTabPath(file.path);
      upsertOpenTab({
        path: file.path,
        content: file.content,
        savedContent: file.content,
        hash: hash || file.hash || '',
        versionId: versionId || '',
        fileType,
        notice: '',
        conflict: null,
        statusMessage,
      });
      if (activatePanel) setActivePanel?.('editor');
    } catch (error) {
      if (requestId !== editorLoadRequestRef.current) return;
      console.error('loadFileIntoEditor failed', error);
      setErrorMessage?.(error.message);
    } finally {
      if (requestId === editorLoadRequestRef.current) {
        setIsEditorLoading(false);
      }
    }
  }

  function upsertOpenTab(tab) {
    setOpenTabs((current) => {
      const index = current.findIndex((item) => item.path === tab.path);
      if (index === -1) return [...current, { ...tab, updatedAt: Date.now() }];
      const next = [...current];
      next[index] = { ...next[index], ...tab, updatedAt: Date.now() };
      return next;
    });
  }

  return loadFileIntoEditor;
}
