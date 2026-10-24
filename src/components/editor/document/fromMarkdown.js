import { parseMarkdownTree, rawNode, splitPlatformText } from './parser.js';
import { readInlineSpan } from '../../markdown/inlineSpan.js';
import { readMarkdownBreak } from '../../markdown/markdownLineBreak.js';
import { readPreservedBlock, readPreservedInline } from '../richText/preservedMarkdownTokens.js';

const leaf = text => text ? [{ type: 'text', text }] : [];
const paragraph = content => ({ type: 'paragraph', ...(content.length ? { content } : {}) });
const preserved = (raw, inline = false, label = '特殊内容') => ({ type: inline ? 'preservedInline' : 'preservedBlock', attrs: { raw, label } });
const ALERTS = new Set(['NOTE', 'TIP', 'IMPORTANT', 'WARNING', 'CAUTION']);

export function parseDocument(source) {
  const tree = parseMarkdownTree(source);
  const definitions = new Map();
  const collect = node => {
    if (node.type === 'definition' && !definitions.has(node.identifier.toLowerCase())) definitions.set(node.identifier.toLowerCase(), node);
    for (const child of node.children || []) collect(child);
  };
  collect(tree);
  const context = { source, definitions };
  const entries = [];
  for (let i = 0; i < tree.children.length; i++) {
    const child = tree.children[i];
    const from = child.position.start.offset;
    const special = /^ {0,3}:::/.test(source.slice(from)) ? readPreservedBlock(source.slice(from)) : null;
    const to = special ? from + special.raw.length : child.position.end.offset;
    entries.push({ node: special ? preserved(special.raw, false, '平台专属段落') : block(child, context), from, to });
    while (i + 1 < tree.children.length && tree.children[i + 1].position.start.offset < to) i++;
  }
  entries.forEach((entry, index) => { entry.node.attrs = { ...entry.node.attrs, sourceId: `root-${index}` }; });
  const doc = { type: 'doc', content: entries.length ? entries.map(entry => entry.node) : [paragraph([])] };
  return { doc, entries, tree, source };
}

function inline(children, context, marks = []) {
  const { source, definitions } = context;
  const result = [];
  for (let i = 0; i < children.length; i++) {
    const node = children[i];
    let values;
    switch (node.type) {
      case 'text': values = splitPlatformText(node, source); break;
      case 'strong': case 'emphasis': case 'delete':
        result.push(...inline(node.children, context, [...marks, { type: { strong: 'bold', emphasis: 'italic', delete: 'strike' }[node.type] }])); continue;
      case 'inlineCode': values = leaf(node.value).map(value => ({ ...value, marks: [{ type: 'code' }] })); break;
      case 'break': values = [{ type: 'hardBreak' }]; break;
      case 'inlineMath': values = [{ type: 'inlineMath', attrs: { formula: node.value } }]; break;
      case 'footnoteReference': values = [{ type: 'footnoteReference', attrs: { identifier: node.identifier } }]; break;
      case 'link': case 'linkReference': {
        const definition = node.type === 'linkReference' ? definitions.get(node.identifier.toLowerCase()) : node;
        if (!definition) { values = [preserved(rawNode(node, source), true, '未解析的引用')]; break; }
        const reference = node.type === 'linkReference' ? { identifier: node.identifier, label: node.label, referenceType: node.referenceType, href: definition.url, title: definition.title || null } : null;
        result.push(...inline(node.children, context, [...marks, { type: 'link', attrs: { href: definition.url, title: definition.title || null, reference } }])); continue;
      }
      case 'image': case 'imageReference': {
        const definition = node.type === 'imageReference' ? definitions.get(node.identifier.toLowerCase()) : node;
        if (!definition) { values = [preserved(rawNode(node, source), true, '未解析的引用图片')]; break; }
        values = [{ type: 'image', attrs: { src: definition.url, alt: node.alt || '', title: definition.title || null,
          reference: node.type === 'imageReference' ? { identifier: node.identifier, label: node.label, referenceType: node.referenceType, href: definition.url, title: definition.title || null } : null } }]; break;
      }
      case 'html': {
        if (readMarkdownBreak(node.value) === node.value) { values = [{ type: 'hardBreak' }]; break; }
        const start = node.position.start.offset;
        const span = readInlineSpan(source.slice(start));
        const special = span || readPreservedInline(source.slice(start));
        const end = start + (special?.raw.length || node.value.length);
        while (i + 1 < children.length && children[i + 1].position.end.offset <= end) i++;
        if (span) {
          const innerSource = span.inner;
          const parsed = parseMarkdownTree(innerSource);
          values = [{ type: 'preservedSpan', attrs: { opening: span.opening, closing: span.closing }, content: inline(parsed.children[0]?.children || [], { ...context, source: innerSource }) }];
        } else values = [preserved(special?.raw || node.value, true, '嵌入内容')];
        break;
      }
      default: values = [preserved(rawNode(node, source), true)];
    }
    for (const value of values) {
      if (marks.length && value.type !== 'hardBreak') value.marks = [...marks, ...(value.marks || [])];
      result.push(value);
    }
  }
  return result;
}

function block(node, context) {
  const content = () => (node.children || []).map(child => block(child, context));
  switch (node.type) {
    case 'paragraph': return paragraph(inline(node.children, context));
    case 'html': return readMarkdownBreak(node.value) === node.value ? paragraph([{ type: 'hardBreak' }]) : preserved(rawNode(node, context.source), false, '嵌入内容');
    case 'heading': return { type: 'heading', attrs: { level: node.depth }, content: inline(node.children, context) };
    case 'thematicBreak': return { type: 'horizontalRule' };
    case 'code': return { type: 'codeBlock', attrs: { language: node.lang || null, meta: node.meta || null }, ...(node.value ? { content: leaf(node.value) } : {}) };
    case 'math': return { type: 'blockMath', attrs: { formula: node.value, meta: node.meta || null } };
    case 'yaml': case 'toml': return { type: 'documentMetadata', attrs: { format: node.type, raw: rawNode(node, context.source) } };
    case 'definition': return preserved(rawNode(node, context.source), false, '链接定义');
    case 'footnoteDefinition': return { type: 'footnoteDefinition', attrs: { identifier: node.identifier }, content: content() };
    case 'blockquote': {
      const blocks = content();
      const first = blocks[0]?.content?.[0];
      const alert = first?.type === 'text' && /^\[!([A-Z]+)\](?:\n|$)/.exec(first.text);
      if (alert && ALERTS.has(alert[1])) {
        first.text = first.text.slice(alert[0].length);
        if (!first.text) blocks[0].content.shift();
        if (!blocks[0].content.length && blocks.length > 1) blocks.shift();
        return { type: 'callout', attrs: { kind: alert[1] }, content: blocks };
      }
      return { type: 'blockquote', content: blocks };
    }
    case 'list': {
      const tasks = node.children.some(child => child.checked !== null && child.checked !== undefined);
      return { type: tasks ? 'taskList' : node.ordered ? 'orderedList' : 'bulletList',
        ...(node.ordered ? { attrs: { start: node.start || 1, ...(tasks ? { ordered: true } : {}) } } : {}),
        content: node.children.map(child => ({ type: child.checked !== null && child.checked !== undefined ? 'taskItem' : 'listItem', ...(child.checked !== null && child.checked !== undefined ? { attrs: { checked: Boolean(child.checked) } } : {}), content: (child.children || []).map(value => block(value, context)) })) };
    }
    case 'table': return { type: 'table', content: node.children.map((row, rowIndex) => ({ type: 'tableRow', content: row.children.map((cell, index) => ({
      type: rowIndex === 0 ? 'tableHeader' : 'tableCell', attrs: { textAlign: node.align[index] || null }, content: [paragraph(inline(cell.children, context))],
    })) })) };
    default: return preserved(rawNode(node, context.source));
  }
}
