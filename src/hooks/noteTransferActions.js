import { API_BASE_URL, apiPostForm } from '../api/client';
import { topLevelSelectedItems } from './fileTreeUtils.js';
import { isNotePath } from './noteTreeUtils.js';

export function createNoteTransferActions({
  getDefaultCreateBasePath,
  loadTree,
  selectedItems,
  selectedItemPath,
  selectedItemType,
  setErrorMessage,
  setUploadStatus,
}) {
  async function exportNotes(options = {}) {
    setErrorMessage('');
    const shouldUseSelection = options.useSelection && selectedItems.length > 1;
    const exportSelection = shouldUseSelection ? topLevelSelectedItems(selectedItems) : [];
    const exportPath = options.path ?? selectedItemPath ?? '';
    const exportType = options.type ?? selectedItemType ?? '';
    const format =
      options.format ||
      (exportSelection.length > 1 ? 'zip' : exportType === 'file' && isNotePath(exportPath) ? 'html' : 'zip');

    try {
      const isMultiSelection = exportSelection.length > 1;
      triggerBrowserDownload(`/api/files/export${buildExportQuery(exportPath, exportSelection, format)}`);
      setUploadStatus(
        isMultiSelection
          ? '\u6279\u91cf\u5bfc\u51fa\u5df2\u5f00\u59cb'
          : format === 'docx'
            ? '\u7b14\u8bb0 Word \u6587\u6863\u5bfc\u51fa\u5df2\u5f00\u59cb'
          : format === 'md'
            ? '\u7b14\u8bb0 Markdown \u6587\u6863\u5bfc\u51fa\u5df2\u5f00\u59cb'
          : format === 'html'
            ? '\u7b14\u8bb0 HTML \u6587\u6863\u5bfc\u51fa\u5df2\u5f00\u59cb'
          : exportPath
            ? '\u7b14\u8bb0\u5bfc\u51fa\u5df2\u5f00\u59cb'
            : '\u6574\u68f5\u7b14\u8bb0\u6811\u5bfc\u51fa\u5df2\u5f00\u59cb'
      );
      window.setTimeout(() => setUploadStatus(''), 1800);
    } catch (error) {
      console.error('exportNotes failed', error);
      setErrorMessage(error.message);
      throw error;
    }
  }

  async function importNotes(file) {
    if (!file) return;
    setErrorMessage('');
    const formData = new FormData();
    formData.append('file', file);
    formData.append('parent_path', getDefaultCreateBasePath?.() || '');
    try {
      const isDocx = /\.docx$/i.test(file.name || '');
      const isMarkdown = /\.(md|markdown)$/i.test(file.name || '');
      setUploadStatus(
        isDocx
          ? `\u6b63\u5728\u5bfc\u5165 Word \u6587\u6863 ${file.name}\uff0c\u5e76\u8f6c\u6362\u4e3a HTML \u7b14\u8bb0...`
          : isMarkdown
            ? `\u6b63\u5728\u5bfc\u5165 Markdown \u6587\u6863 ${file.name}\uff0c\u5e76\u8f6c\u6362\u4e3a HTML \u7b14\u8bb0...`
          : `\u6b63\u5728\u5bfc\u5165 ${file.name}...`
      );
      const result = await apiPostForm('/api/files/import', formData);
      await loadTree?.();
      setUploadStatus(`\u5bfc\u5165\u5b8c\u6210\uff0c${result.imported_count || 0} \u4e2a\u9879\u76ee`);
      window.setTimeout(() => setUploadStatus(''), 2200);
      return result;
    } catch (error) {
      console.error('importNotes failed', error);
      setErrorMessage(error.message);
      throw error;
    }
  }

  return {
    exportNotes,
    importNotes,
  };
}

function buildExportQuery(exportPath, exportSelection, format = 'zip') {
  if (exportSelection.length > 1) {
    return `?paths_json=${encodeURIComponent(JSON.stringify(exportSelection.map((item) => item.path)))}&format=${encodeURIComponent(format)}`;
  }
  const params = new URLSearchParams();
  if (exportPath) params.set('path', exportPath);
  if (format) params.set('format', format);
  const query = params.toString();
  return query ? `?${query}` : '';
}

function triggerBrowserDownload(path) {
  const anchor = document.createElement('a');
  anchor.href = `${API_BASE_URL}${path}`;
  anchor.download = '';
  anchor.rel = 'noopener';
  anchor.style.display = 'none';
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
}
