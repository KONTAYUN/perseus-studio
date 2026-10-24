import { unified } from 'unified';
import remarkParse from 'remark-parse';
import remarkStringify from 'remark-stringify';
import remarkGfm from 'remark-gfm';
import remarkMath from 'remark-math';
import remarkFrontmatter from 'remark-frontmatter';
import { decodeString } from 'micromark-util-decode-string';

export const markdownProcessor = unified().use(remarkParse).use(remarkGfm)
  .use(remarkMath, { singleDollarTextMath: true })
  .use(remarkFrontmatter, ['yaml', 'toml'])
  .use(remarkStringify, { bullet: '-', emphasis: '*', strong: '*', fences: true, listItemIndent: 'one', rule: '-', ruleRepetition: 3 });

export function parseMarkdownTree(source) {
  const tree = markdownProcessor.parse(source);
  // micromark consumes the leading BOM before assigning offsets. All source
  // splices use the original JS string, so restore that one-character offset.
  if (source.startsWith('\uFEFF')) {
    const visit = node => {
      for (const point of [node.position?.start, node.position?.end]) if (point) { point.offset++; if (point.line === 1) point.column++; }
      for (const child of node.children || []) visit(child);
    };
    visit(tree);
  }
  return tree;
}
export const writeMarkdownTree = tree => markdownProcessor.stringify(tree);
export const rawNode = (node, source) => source.slice(node.position?.start.offset ?? 0, node.position?.end.offset ?? source.length);

// Only plain text leaves are examined for platform-specific syntax. Code spans,
// fenced code, escaped markup and native Markdown grammar are parsed by remark.
export function splitPlatformText(node, source) {
  const raw = rawNode(node, source);
  const result = [];
  const pattern = /!?\[\[[^\]\n]+\]\]|\\\([^\n]*?\\\)|\\\[[^\n]*?\\\]/g;
  let at = 0;
  for (const match of raw.matchAll(pattern)) {
    let escapes = 0;
    for (let i = match.index - 1; i >= 0 && raw[i] === '\\'; i--) escapes++;
    if (escapes % 2) continue;
    if (match.index > at) result.push({ type: 'text', text: decodeString(raw.slice(at, match.index)) });
    result.push({ type: 'preservedInline', attrs: { raw: match[0], label: '平台专属内容' } });
    at = match.index + match[0].length;
  }
  if (!at) return [{ type: 'text', text: node.value }];
  if (at < raw.length) result.push({ type: 'text', text: decodeString(raw.slice(at)) });
  return result;
}
