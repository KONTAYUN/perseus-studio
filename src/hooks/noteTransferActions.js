import { apiGetBlob, apiPostForm } from '../api/client';
import { downloadBlob } from './fileTreeDownloadUtils';
import { topLevelSelectedItems } from './fileTreeUtils.js';
import { getNoteTitle, isNotePath } from './noteTreeUtils.js';

function parseDownloadFileName(contentDisposition, fallback = 'perseus-notes.zip') {
  const utf8Match = /filename\*\s*=\s*UTF-8''([^;]+)/i.exec(contentDisposition || '');
  if (utf8Match?.[1]) {
    try {
      return decodeURIComponent(utf8Match[1]);
    } catch (_error) {
      // fall through to legacy filename parsing
    }
  }
  const match = /filename="?([^"]+)"?/i.exec(contentDisposition || '');
  return match?.[1] || fallback;
}

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
      const fallbackName =
        isMultiSelection
          ? 'selected-notes.zip'
          : exportType === 'file' && isNotePath(exportPath)
            ? `${getNoteTitle(exportPath) || 'note'}.${exportExtensionForFormat(format)}`
            : `${getNoteTitle(exportPath) || 'perseus-notes'}.zip`;
      const { blob, contentDisposition } = await apiGetBlob(`/api/files/export${buildExportQuery(exportPath, exportSelection, format)}`);
      downloadBlob(parseDownloadFileName(contentDisposition, fallbackName), blob);
      setUploadStatus(
        isMultiSelection
          ? '\u6279\u91cf\u5bfc\u51fa\u6210\u529f'
          : format === 'docx'
            ? '\u7b14\u8bb0 Word \u6587\u6863\u5bfc\u51fa\u6210\u529f'
          : format === 'md'
            ? '\u7b14\u8bb0 Markdown \u6587\u6863\u5bfc\u51fa\u6210\u529f'
          : format === 'html'
            ? '\u7b14\u8bb0 HTML \u6587\u6863\u5bfc\u51fa\u6210\u529f'
          : exportPath
            ? '\u7b14\u8bb0\u5bfc\u51fa\u6210\u529f'
            : '\u6574\u68f5\u7b14\u8bb0\u6811\u5bfc\u51fa\u6210\u529f'
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

function exportExtensionForFormat(format) {
  if (format === 'docx') return 'docx';
  if (format === 'md') return 'md';
  if (format === 'html') return 'html';
  return 'zip';
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
