import { apiGetBlob } from '../api/client';
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

export async function readFileForDownload(path) {
  return apiGetBlob(`/api/files/download?path=${encodeURIComponent(path)}`);
}

export async function buildZipDownloadBlob(items, treeData) {
  const files = collectDownloadFiles(items, treeData);
  if (files.length === 0) throw new Error('没有可下载的文件');
  if (files.length > 65535) throw new Error('文件过多，请分批下载');
  const entries = [];
  let totalBytes = 0;
  for (const item of files) {
    const blob = await readFileForDownload(item.path);
    totalBytes += blob.size;
    if (totalBytes > 256 * 1024 * 1024) throw new Error('批量下载超过 256 MiB，请分批下载');
    entries.push({ path: item.path, bytes: new Uint8Array(await blob.arrayBuffer()) });
  }
  return { blob: createZipBlob(entries) };
}
