import { safeMarkdownUrl } from '../../markdown/markdownPolicy.js';
const ORIGIN = 'https://workspace.invalid/';
export function imageDisplayURL(value, path = '') {
  const source = safeMarkdownUrl(value);
  if (!source || source.startsWith('#')) return '';
  if (/^https?:/i.test(source)) return source;
  try {
    const local = new URL(source, new URL(path, ORIGIN));
    if (local.origin !== new URL(ORIGIN).origin) return '';
    return `/api/files/image?path=${encodeURIComponent(decodeURIComponent(local.pathname.slice(1)))}`;
  } catch { return ''; }
}
export function imageSourceFromHTML(element) {
  const source = element.getAttribute('src') || '';
  try {
    const url = new URL(source, location.origin);
    if (url.origin === location.origin && url.pathname === '/api/files/image') return `/${url.searchParams.get('path')}`;
  } catch { /* Invalid addresses remain inert. */ }
  return safeMarkdownUrl(source);
}
