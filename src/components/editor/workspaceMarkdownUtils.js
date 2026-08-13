import { workspaceFileContentUrl } from '../../api/client.js';
import { getParentPath } from '../../hooks/ideUtils.js';
import { isMarkdownPath } from '../../hooks/noteTreeUtils.js';
import { safeMarkdownUrl } from '../markdown/markdownUtils.js';

const EXTERNAL_PATTERN = /^(https?:|mailto:|tel:)/i;

function splitMarkdownTarget(raw = '') {
  const value = String(raw || '').trim();
  const marker = value.search(/[?#]/);
  if (marker < 0) return { path: value, suffix: '' };
  return { path: value.slice(0, marker), suffix: value.slice(marker) };
}

export function resolveWorkspaceMarkdownPath(markdownPath, target) {
  const safeTarget = safeMarkdownUrl(target);
  if (!safeTarget || safeTarget.startsWith('//') || EXTERNAL_PATTERN.test(safeTarget) || safeTarget.startsWith('#') || safeTarget.startsWith('?')) {
    return '';
  }
  const { path } = splitMarkdownTarget(safeTarget);
  if (!path) return '';
  let decoded;
  try {
    decoded = decodeURIComponent(path).replace(/\\/g, '/');
  } catch {
    return '';
  }
  const base = decoded.startsWith('/') ? [] : getParentPath(markdownPath).split('/').filter(Boolean);
  const parts = decoded.replace(/^\/+/, '').split('/').filter(Boolean);
  const resolved = [...base];
  for (const part of parts) {
    if (part === '.') continue;
    if (part === '..') {
      if (resolved.length === 0) return '';
      resolved.pop();
      continue;
    }
    resolved.push(part);
  }
  return resolved.join('/');
}

export function resolveWorkspaceMarkdownLink(markdownPath, href) {
  const safeHref = safeMarkdownUrl(href);
  if (!safeHref) return null;
  if (EXTERNAL_PATTERN.test(safeHref)) return { href: safeHref, external: true };
  if (safeHref.startsWith('#') || safeHref.startsWith('?')) return { href: safeHref, external: false };
  const workspacePath = resolveWorkspaceMarkdownPath(markdownPath, safeHref);
  return workspacePath ? { workspacePath } : null;
}

export function resolveWorkspaceMarkdownImage(markdownPath, src) {
  const safeSrc = safeMarkdownUrl(src);
  if (!safeSrc) return '';
  if (/^https?:/i.test(safeSrc)) return safeSrc;
  const workspacePath = resolveWorkspaceMarkdownPath(markdownPath, safeSrc);
  return workspacePath ? workspaceFileContentUrl(workspacePath) : '';
}

export function defaultMarkdownViewState(path = '') {
  return {
    markdownViewMode: isMarkdownPath(path) ? 'rich-text' : 'source',
    markdownSourceScrollTop: 0,
    markdownRichTextScrollTop: 0,
    markdownSelectionStart: 0,
    markdownSelectionEnd: 0,
  };
}

export function markdownViewStateFromTab(tab = {}) {
  const defaults = defaultMarkdownViewState(tab.path);
  const storedMode = tab.markdownViewMode === 'preview' ? 'rich-text' : tab.markdownViewMode;
  return {
    ...defaults,
    markdownViewMode: storedMode || defaults.markdownViewMode,
    markdownSourceScrollTop: Number(tab.markdownSourceScrollTop || 0),
    markdownRichTextScrollTop: Number(
      tab.markdownRichTextScrollTop ?? tab.markdownPreviewScrollTop ?? 0
    ),
    markdownSelectionStart: Number(tab.markdownSelectionStart || 0),
    markdownSelectionEnd: Number(tab.markdownSelectionEnd || 0),
  };
}
