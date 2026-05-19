import { apiGet } from '../api/client';
import { collectDownloadFiles, createZipBlob, isTextPath } from './ideUtils';

export function downloadBlob(fileName, blob) {
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = fileName;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}

export function downloadText(path, content) {
  const blob = new Blob([content], { type: 'text/plain;charset=utf-8' });
  downloadBlob(path.split('/').pop() || 'download.txt', blob);
}

export function handleDownloadDragStart(e, path, content) {
  if (!path || !isTextPath(path)) return;
  const blob = new Blob([content], { type: 'text/plain;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const fileName = path.split('/').pop() || 'download.txt';
  e.dataTransfer.effectAllowed = 'copy';
  e.dataTransfer.setData('DownloadURL', `text/plain:${fileName}:${url}`);
  e.dataTransfer.setData('text/plain', content);
  window.setTimeout(() => URL.revokeObjectURL(url), 30000);
}

export async function readTextFileForDownload(path) {
  if (!isTextPath(path)) {
    throw new Error('第一版仅支持文本文件下载');
  }
  const file = await apiGet(`/api/files/read?path=${encodeURIComponent(path)}`);
  return { path: file.path, content: file.content };
}

export async function buildZipDownloadBlob(items, treeData) {
  const downloadFiles = collectDownloadFiles(items, treeData);
  const textFiles = downloadFiles.filter((file) => isTextPath(file.path));
  const skippedCount = downloadFiles.length - textFiles.length;

  if (textFiles.length === 0) {
    throw new Error('没有可下载的文本文件');
  }

  const entries = [];
  for (const item of textFiles) {
    const file = await readTextFileForDownload(item.path);
    entries.push({ path: file.path, content: file.content });
  }

  return {
    blob: createZipBlob(entries),
    skippedCount,
  };
}
