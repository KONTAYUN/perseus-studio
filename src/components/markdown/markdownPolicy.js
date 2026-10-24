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
