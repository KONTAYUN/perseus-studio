import { withoutSupportedSpans } from './inlineSpan.js';

export function safeMarkdownUrl(value = '') {
  const url = String(value).trim();
  if (/[\u0000-\u0020\u007f\\]/.test(url)) return '';
  if (/^(?:https?:|mailto:)/i.test(url)) return url;
  if (/^[a-z][a-z\d+.-]*:/i.test(url) || url.startsWith('//')) return '';
  return url;
}

export function isMarkdownFile(path = '') {
  return /\.(?:md|markdown|mdown)$/i.test(path);
}

function withoutCode(source) {
  let fence = null;
  return source.split(/\r?\n/).map(line => {
    const marker = /^ {0,3}(`{3,}|~{3,})(.*)$/.exec(line);
    if (fence) {
      if (marker && marker[1][0] === fence[0] && marker[1].length >= fence.length && !marker[2].trim()) fence = null;
      return '';
    }
    if (marker) { fence = marker[1]; return ''; }
    if (/^(?: {4}|\t)/.test(line)) return '';
    return line;
  }).join('\n').replace(/(`+)(?!`)([\s\S]*?[^`])\1(?!`)/g, '');
}

// These constructs have no lossless counterpart in the editable document schema.
// Keep the original source; preview is available without any serialization.
export function richEditingRestriction(source = '') {
  if (source.length > 512 * 1024) return '文档较大，已使用只读预览；请切换纯文本编辑。';
  const prose = withoutSupportedSpans(withoutCode(source));
  if (/^---\r?\n[\s\S]*?\r?\n(?:---|\.\.\.)(?:\r?\n|$)/.test(prose)
      || /^\+\+\+\r?\n/.test(prose)
      || /<(?:\/?[a-z][a-z\d-]*(?=[\s/>])[^>]*|!--[\s\S]*?--|![a-z][^>]*|\?[^>]*)>/i.test(prose)
      || /^\s*\[\^[^\]]+\]:/m.test(prose)
      || /^\s*\$\$|\\\[|\\\(/m.test(prose)
      || /^\s*:::{3,}/m.test(prose)
      || /!\[\[|(?<!\\)\[\[[^\]]+\]\]/.test(prose)
      || /^\s*\[[^\]]+\]:\s*\S+/m.test(prose)) {
    return '此文档包含扩展语法，已保留原文并使用只读预览；请切换纯文本编辑。';
  }
  return '';
}
