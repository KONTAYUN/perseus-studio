const LANGUAGE_LABELS = {
  bash: 'Bash', sh: 'Shell', shell: 'Shell', zsh: 'Shell',
  powershell: 'PowerShell', ps1: 'PowerShell',
  javascript: 'JavaScript', js: 'JavaScript', jsx: 'JSX',
  typescript: 'TypeScript', ts: 'TypeScript', tsx: 'TSX',
  json: 'JSON', html: 'HTML', css: 'CSS', scss: 'SCSS', sql: 'SQL',
  yaml: 'YAML', yml: 'YAML', markdown: 'Markdown', md: 'Markdown',
  python: 'Python', py: 'Python', go: 'Go', golang: 'Go',
  text: 'Text', txt: 'Text',
};

const SAFE_URL_PATTERN = /^(https?:|mailto:|tel:)/i;
const UNSAFE_URL_PATTERN = /^\s*(javascript:|data:|vbscript:)/i;

export function languageFromClassName(className = '') {
  const match = /(?:^|\s)language-([^\s]+)/i.exec(className || '');
  return match?.[1]?.toLowerCase() || '';
}

export function displayLanguageName(language = '') {
  const normalized = String(language || '').trim().toLowerCase();
  if (!normalized) return LANGUAGE_LABELS.text;
  if (LANGUAGE_LABELS[normalized]) return LANGUAGE_LABELS[normalized];
  return normalized
    .split(/[-_]/)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ') || LANGUAGE_LABELS.text;
}

export function reactNodeToPlainText(node) {
  if (node == null || typeof node === 'boolean') return '';
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map(reactNodeToPlainText).join('');
  if (typeof node === 'object' && node.props) return reactNodeToPlainText(node.props.children);
  return '';
}

export function normalizeCodeForCopy(code = '') {
  return String(code || '').replace(/\n+$/g, '');
}

export function safeMarkdownUrl(url = '') {
  const raw = String(url || '').trim();
  if (!raw || UNSAFE_URL_PATTERN.test(raw)) return '';
  if (SAFE_URL_PATTERN.test(raw)) return raw;
  if (/^(#|\/|\?|\.\.?\/)/.test(raw)) return raw;
  if (!/^[a-z][a-z0-9+.-]*:/i.test(raw)) return raw;
  return '';
}

export function isLikelyBlockCode(className = '', children) {
  const text = reactNodeToPlainText(children);
  return Boolean(languageFromClassName(className) || /\n$/.test(text) || text.includes('\n'));
}
