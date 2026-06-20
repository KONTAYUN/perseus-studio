import { getParentPath, isAnyTrashPath } from './ideUtils.js';

export function isNotePath(path = '') {
  const lower = String(path || '').toLowerCase();
  return lower.endsWith('.html');
}

export function getNoteTitle(path = '') {
  const name = String(path || '').split('/').pop() || '';
  if (name.toLowerCase().endsWith('.html')) return name.slice(0, -5);
  return name;
}

export function getNotePathLabel(path = '') {
  return String(path || '')
    .split('/')
    .map((segment, index, all) => (index === all.length - 1 ? getNoteTitle(segment) : segment))
    .join('/');
}

export function isSystemNoteAssetFolder(path = '') {
  return String(path || '')
    .split('/')
    .some((segment) => segment === '.assets');
}

export function filterNoteTree(items) {
  const next = [];
  for (const item of items || []) {
    if (!item?.path || isAnyTrashPath(item.path) || isSystemNoteAssetFolder(item.path)) continue;
    if (item.type === 'folder') {
      const children = filterNoteTree(item.children || []);
      next.push({ ...item, children });
      continue;
    }
    if (isNotePath(item.path)) next.push({ ...item });
  }
  return next;
}

export function buildNotePath(name = '') {
  const trimmed = String(name || '').trim();
  if (!trimmed) return '';
  const base = trimmed.replace(/[\\/:*?"<>|]/g, ' ').replace(/\s+/g, ' ').trim();
  if (!base) return '';
  return isNotePath(base) ? base : `${base}.html`;
}

export function buildNoteAssetUploadPath(notePath, fileName) {
  const normalizedNote = String(notePath || '').trim().replace(/\\/g, '/');
  const cleanedName = String(fileName || 'asset').replace(/[\\/:*?"<>|]/g, '-');
  const parentPath = getParentPath(normalizedNote);
  const noteTitle = getNoteTitle(normalizedNote) || 'note';
  const timestamp = Date.now();
  const assetFolder = parentPath ? `${parentPath}/.assets/${noteTitle}` : `.assets/${noteTitle}`;
  return `${assetFolder}/${timestamp}-${cleanedName}`;
}

function normalizeWorkspacePath(path = '') {
  return String(path || '')
    .replace(/\\/g, '/')
    .split('/')
    .filter(Boolean)
    .join('/');
}

export function getNoteAssetReferencePath(notePath, assetPath) {
  const noteParentParts = normalizeWorkspacePath(getParentPath(notePath)).split('/').filter(Boolean);
  const assetParts = normalizeWorkspacePath(assetPath).split('/').filter(Boolean);
  let commonIndex = 0;
  while (
    commonIndex < noteParentParts.length &&
    commonIndex < assetParts.length &&
    noteParentParts[commonIndex] === assetParts[commonIndex]
  ) {
    commonIndex += 1;
  }
  const upward = noteParentParts.slice(commonIndex).map(() => '..');
  const downward = assetParts.slice(commonIndex);
  return [...upward, ...downward].join('/') || assetParts.at(-1) || '';
}

export function resolveNoteAssetPath(notePath, assetReferencePath = '') {
  const reference = normalizeWorkspacePath(assetReferencePath);
  if (!reference) return '';
  if (!reference.startsWith('.')) return reference;
  const baseParts = normalizeWorkspacePath(getParentPath(notePath)).split('/').filter(Boolean);
  const parts = reference.split('/').filter(Boolean);
  const resolved = [...baseParts];
  for (const part of parts) {
    if (part === '.') continue;
    if (part === '..') {
      resolved.pop();
      continue;
    }
    resolved.push(part);
  }
  return resolved.join('/');
}
