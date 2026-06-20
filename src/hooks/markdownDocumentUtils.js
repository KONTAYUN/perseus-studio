import { marked } from 'marked';
import TurndownService from 'turndown';

marked.setOptions({
  breaks: true,
  gfm: true,
});

const turndown = new TurndownService({
  bulletListMarker: '-',
  codeBlockStyle: 'fenced',
  emDelimiter: '*',
  headingStyle: 'atx',
  strongDelimiter: '**',
});

turndown.addRule('strikethrough', {
  filter: ['del', 's'],
  replacement(content) {
    return `~~${content}~~`;
  },
});

turndown.addRule('taskListItems', {
  filter(node) {
    return node.nodeName === 'LI' && node.getAttribute('data-type') === 'taskItem';
  },
  replacement(content, node, options) {
    const checkbox = node.querySelector('input[type="checkbox"]');
    const checked = checkbox?.checked ? 'x' : ' ';
    const normalized = content.replace(/\n+/g, ' ').trim();
    return `${options.bulletListMarker} [${checked}] ${normalized}\n`;
  },
});

turndown.addRule('noteImages', {
  filter(node) {
    return node.nodeName === 'IMG';
  },
  replacement(_content, node) {
    return node.outerHTML || '';
  },
});

turndown.addRule('styledInlineNodes', {
  filter(node) {
    if (!['SPAN', 'EM', 'STRONG', 'U'].includes(node.nodeName)) return false;
    const style = node.getAttribute('style') || '';
    return /font-size|color|background-color/.test(style);
  },
  replacement(_content, node) {
    return node.outerHTML || node.textContent || '';
  },
});

turndown.addRule('styledBlockNodes', {
  filter(node) {
    if (!['P', 'H1', 'H2', 'H3', 'H4', 'BLOCKQUOTE'].includes(node.nodeName)) return false;
    const style = node.getAttribute('style') || '';
    return /line-height|margin-bottom|text-align/.test(style);
  },
  replacement(_content, node) {
    return `\n\n${node.outerHTML || ''}\n\n`;
  },
});

turndown.addRule('richTables', {
  filter(node) {
    return node.nodeName === 'TABLE';
  },
  replacement(_content, node) {
    return `\n\n${node.outerHTML || ''}\n\n`;
  },
});

export function markdownToHtml(markdown = '') {
  return marked.parse(markdown || '');
}

export function htmlToMarkdown(html = '') {
  return turndown
    .turndown(html || '')
    .replace(/\u00a0/g, ' ')
    .replace(/\n{3,}/g, '\n\n')
    .trim();
}
