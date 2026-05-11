export const TRASH_DIR = '回收站';
export const LEGACY_TRASH_DIR = '.trash';

export function isTrashPath(path = '') {
  const value = String(path || '').trim();
  return value === TRASH_DIR || value.startsWith(`${TRASH_DIR}/`);
}

export function isLegacyTrashPath(path = '') {
  const value = String(path || '').trim();
  return value === LEGACY_TRASH_DIR || value.startsWith(`${LEGACY_TRASH_DIR}/`);
}

export function isAnyTrashPath(path = '') {
  return isTrashPath(path) || isLegacyTrashPath(path);
}

export function decorateTree(items, openFolders) {
  return (items || []).map((item) => ({
    ...item,
    id: item.path || item.name,
    isOpen: item.type === 'folder' ? openFolders.has(item.path) : false,
    children: item.children ? decorateTree(item.children, openFolders) : [],
  }));
}

export function seedOpenFolders(items) {
  const next = new Set();
  collectFolderPaths(items, next);
  return next;
}

export function flattenTree(items) {
  const result = [];
  for (const item of items) {
    result.push(item);
    if (item.type === 'folder' && item.isOpen && item.children) {
      result.push(...flattenTree(item.children));
    }
  }
  return result;
}

export function collectDownloadFiles(items, treeItems) {
  const result = [];
  const seen = new Set();
  for (const item of items) {
    const node = findTreeItem(treeItems, item.path);
    if (node) collectFilesFromNode(node, result, seen);
  }
  return result;
}

export function findTreeItem(items, path) {
  for (const item of items || []) {
    if (item.path === path) return item;
    const found = findTreeItem(item.children || [], path);
    if (found) return found;
  }
  return null;
}

export function getParentPath(path) {
  const parts = path.split('/');
  parts.pop();
  return parts.join('/');
}

export function getFileTreeDropZone(clientY, rect, itemType) {
  if (!rect || !Number.isFinite(rect.height) || rect.height <= 0) return 'none';
  const offset = clientY - rect.top;
  const ratio = offset / rect.height;
  if (ratio <= 0.3) return 'before';
  if (ratio >= 0.7) return 'after';
  return itemType === 'folder' ? 'inside' : 'none';
}

export function pathMatches(path, candidate) {
  if (!path || !candidate) return false;
  return path === candidate || path.startsWith(`${candidate}/`);
}

export function remapPathPrefix(path, oldPath, newPath) {
  if (!path || !oldPath || !newPath) return path;
  if (path === oldPath) return newPath;
  if (path.startsWith(`${oldPath}/`)) {
    return `${newPath}${path.slice(oldPath.length)}`;
  }
  return path;
}

export function remapPathSet(current, oldPath, newPath) {
  const next = new Set();
  for (const value of current) {
    next.add(remapPathPrefix(value, oldPath, newPath));
  }
  return next;
}

export function removePathSet(current, removedPath) {
  const next = new Set();
  for (const value of current) {
    if (!pathMatches(value, removedPath)) {
      next.add(value);
    }
  }
  return next;
}

export function renameFolderState(current, oldPath, newPath) {
  const next = new Set();
  for (const value of current) {
    if (pathMatches(value, oldPath)) {
      next.add(remapPathPrefix(value, oldPath, newPath));
    } else {
      next.add(value);
    }
  }
  return next;
}

export function removeFolderState(current, removedPath) {
  const next = new Set();
  for (const value of current) {
    if (!pathMatches(value, removedPath)) {
      next.add(value);
    }
  }
  return next;
}

export function buildUploadPath(fileName, targetPath) {
  if (!targetPath) return fileName;
  return `${targetPath}/${fileName}`;
}

export function detectFileType(path) {
  const name = path.split('/').pop()?.toLowerCase() || '';
  const extension = name.includes('.') ? name.split('.').pop() : '';
  const textExtensions = new Set([
    'txt',
    'md',
    'markdown',
    'json',
    'js',
    'jsx',
    'ts',
    'tsx',
    'css',
    'html',
    'go',
    'py',
    'yaml',
    'yml',
    'toml',
    'csv',
    'log',
    'env',
    'gitignore',
    'xml',
    'sh',
  ]);
  const imageExtensions = new Set(['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg']);

  if (name === '.env' || name === '.gitignore') return 'text';
  if (imageExtensions.has(extension)) return 'image';
  if (textExtensions.has(extension)) return 'text';
  return 'binary';
}

export function isTextPath(path) {
  return detectFileType(path) === 'text';
}

export function attachmentFingerprint(file) {
  return `${file.name || 'unnamed'}:${file.size || 0}:${file.lastModified || 0}:${file.type || 'unknown'}`;
}

export function shouldReloadWorkspaceForToolEvents(toolEvents) {
  return (toolEvents || []).some(
    (item) =>
      item?.status === 'success' &&
      [
        'save_workspace_file',
        'manage_workspace_path',
        'organize_workspace_matches',
        'run_workspace_command',
      ].includes(item?.tool_name)
  );
}

export function createZipBlob(entries) {
  const encoder = new TextEncoder();
  const localParts = [];
  const centralParts = [];
  let offset = 0;

  for (const entry of entries) {
    const nameBytes = encoder.encode(entry.path.replace(/\\/g, '/'));
    const contentBytes = encoder.encode(entry.content);
    const crc = crc32(contentBytes);
    const localHeader = createZipLocalHeader(nameBytes, contentBytes, crc);
    const centralHeader = createZipCentralHeader(nameBytes, contentBytes, crc, offset);
    localParts.push(localHeader, contentBytes);
    centralParts.push(centralHeader);
    offset += localHeader.length + contentBytes.length;
  }

  const centralSize = centralParts.reduce((size, part) => size + part.length, 0);
  const endRecord = createZipEndRecord(entries.length, centralSize, offset);
  return new Blob([...localParts, ...centralParts, endRecord], { type: 'application/zip' });
}

function collectFilesFromNode(node, result, seen) {
  if (node.type === 'file') {
    if (!seen.has(node.path)) {
      seen.add(node.path);
      result.push(node);
    }
    return;
  }
  for (const child of node.children || []) {
    collectFilesFromNode(child, result, seen);
  }
}

function collectFolderPaths(items, target) {
  for (const item of items || []) {
    if (item.type === 'folder') {
      target.add(item.path);
      if (item.children) collectFolderPaths(item.children, target);
    }
  }
}

function createZipLocalHeader(nameBytes, contentBytes, crc) {
  const header = new Uint8Array(30 + nameBytes.length);
  const view = new DataView(header.buffer);
  view.setUint32(0, 0x04034b50, true);
  view.setUint16(4, 20, true);
  view.setUint16(6, 0x0800, true);
  view.setUint16(8, 0, true);
  setDosDateTime(view, 10);
  view.setUint32(14, crc, true);
  view.setUint32(18, contentBytes.length, true);
  view.setUint32(22, contentBytes.length, true);
  view.setUint16(26, nameBytes.length, true);
  header.set(nameBytes, 30);
  return header;
}

function createZipCentralHeader(nameBytes, contentBytes, crc, offset) {
  const header = new Uint8Array(46 + nameBytes.length);
  const view = new DataView(header.buffer);
  view.setUint32(0, 0x02014b50, true);
  view.setUint16(4, 20, true);
  view.setUint16(6, 20, true);
  view.setUint16(8, 0x0800, true);
  view.setUint16(10, 0, true);
  setDosDateTime(view, 12);
  view.setUint32(16, crc, true);
  view.setUint32(20, contentBytes.length, true);
  view.setUint32(24, contentBytes.length, true);
  view.setUint16(28, nameBytes.length, true);
  view.setUint32(42, offset, true);
  header.set(nameBytes, 46);
  return header;
}

function createZipEndRecord(entryCount, centralSize, centralOffset) {
  const header = new Uint8Array(22);
  const view = new DataView(header.buffer);
  view.setUint32(0, 0x06054b50, true);
  view.setUint16(8, entryCount, true);
  view.setUint16(10, entryCount, true);
  view.setUint32(12, centralSize, true);
  view.setUint32(16, centralOffset, true);
  return header;
}

function setDosDateTime(view, offset) {
  const date = new Date();
  const dosTime =
    (date.getHours() << 11) | (date.getMinutes() << 5) | Math.floor(date.getSeconds() / 2);
  const dosDate =
    ((date.getFullYear() - 1980) << 9) | ((date.getMonth() + 1) << 5) | date.getDate();
  view.setUint16(offset, dosTime, true);
  view.setUint16(offset + 2, dosDate, true);
}

function crc32(bytes) {
  let crc = 0xffffffff;
  for (const byte of bytes) {
    crc = (crc >>> 8) ^ CRC_TABLE[(crc ^ byte) & 0xff];
  }
  return (crc ^ 0xffffffff) >>> 0;
}

const CRC_TABLE = (() => {
  const table = new Uint32Array(256);
  for (let i = 0; i < 256; i += 1) {
    let value = i;
    for (let bit = 0; bit < 8; bit += 1) {
      value = value & 1 ? 0xedb88320 ^ (value >>> 1) : value >>> 1;
    }
    table[i] = value >>> 0;
  }
  return table;
})();
