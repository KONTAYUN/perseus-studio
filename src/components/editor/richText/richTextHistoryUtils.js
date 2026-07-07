import { workspaceFileContentUrl } from '../../../api/client';
import { getNoteTitle, isNotePath, resolveNoteAssetPath } from '../../../hooks/noteTreeUtils.js';

export function toDate(value) {
  const date = value ? new Date(value) : null;
  return date && !Number.isNaN(date.getTime()) ? date : null;
}

export function formatDate(value) {
  const date = toDate(value);
  if (!date) return '未知日期';
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).format(date);
}

export function formatTime(value) {
  const date = toDate(value);
  if (!date) return '未知时间';
  return new Intl.DateTimeFormat('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

export function formatTimestamp(value) {
  const date = toDate(value);
  if (!date) return '未知时间';
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

export function isAiSource(source = '') {
  return String(source || '').toLowerCase().startsWith('ai_');
}

export function sourceLabel(source = '') {
  const normalized = String(source || '').toLowerCase();
  if (normalized === 'restore_version') return '恢复版本';
  if (normalized === 'manual_write') return '手动保存';
  if (normalized === 'create') return '新建';
  if (normalized.startsWith('ai_')) return 'AI 修改';
  if (normalized) return normalized.replaceAll('_', ' ');
  return '修改';
}

export function baseName(path = '') {
  return String(path || '').split('/').filter(Boolean).pop() || '未命名文件';
}

export function parentPath(path = '') {
  const parts = String(path || '').split('/').filter(Boolean);
  parts.pop();
  return parts.join('/');
}

export function displayName(path = '') {
  return isNotePath(path) ? getNoteTitle(path) : baseName(path);
}

export function decoratePreviewHtml(notePath, html) {
  if (typeof window === 'undefined') return html;
  const template = window.document.createElement('template');
  template.innerHTML = html || '';
  template.content.querySelectorAll('img').forEach((image) => {
    const original = image.getAttribute('src') || '';
    const resolvedPath = resolveNoteAssetPath(notePath, original);
    if (resolvedPath) {
      image.setAttribute('src', workspaceFileContentUrl(resolvedPath));
    }
  });
  return template.innerHTML;
}

export function groupHistoryItems(items) {
  return items.reduce((groups, item) => {
    const label = formatDate(item.created_at);
    const group = groups.find((entry) => entry.label === label);
    if (group) {
      group.items.push(item);
    } else {
      groups.push({ label, items: [item] });
    }
    return groups;
  }, []);
}
