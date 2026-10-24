import { writeMarkdownTree } from './parser.js';
import { DocumentFormatError } from './profile.js';

const text = value => ({ type: 'text', value });
const reference = attrs => attrs.reference && attrs.reference.href === (attrs.href ?? attrs.src)
  && (attrs.reference.title || null) === (attrs.title || null) ? attrs.reference : null;
const order = { link: 0, bold: 1, italic: 2, strike: 3, code: 4 };

export function inlineToTree(nodes = [], table = false) {
  const result = [];
  let active = [];
  let parents = [result];
  for (const node of nodes) {
    const marks = [...(node.marks || [])].sort((a, b) => (order[a.type] ?? 10) - (order[b.type] ?? 10));
    const containers = marks.filter(mark => mark.type !== 'code');
    let common = 0;
    while (common < active.length && common < containers.length && JSON.stringify(active[common]) === JSON.stringify(containers[common])) common++;
    active = active.slice(0, common); parents = parents.slice(0, common + 1);
    for (const mark of containers.slice(common)) {
      const attrs = mark.attrs || {};
      const ref = reference(attrs);
      let wrapper;
      if (mark.type === 'link') wrapper = ref ? { type: 'linkReference', identifier: ref.identifier, label: ref.label, referenceType: ref.referenceType, children: [] }
        : { type: 'link', url: attrs.href || '', title: attrs.title || null, children: [] };
      else if (['bold', 'italic', 'strike'].includes(mark.type)) wrapper = { type: { bold: 'strong', italic: 'emphasis', strike: 'delete' }[mark.type], children: [] };
      else throw new DocumentFormatError(`暂时无法保存文字格式：${mark.type}`);
      parents.at(-1).push(wrapper); parents.push(wrapper.children); active.push(mark);
    }
    let value;
    const attrs = node.attrs || {};
    switch (node.type) {
      case 'text': value = marks.some(mark => mark.type === 'code') ? { type: 'inlineCode', value: node.text } : text(node.text); break;
      case 'hardBreak': value = table ? { type: 'html', value: '<br>' } : { type: 'break' }; break;
      case 'inlineMath': value = { type: 'inlineMath', value: attrs.formula || '' }; break;
      case 'footnoteReference': value = { type: 'footnoteReference', identifier: attrs.identifier, label: attrs.identifier }; break;
      case 'image': {
        const ref = reference(attrs);
        value = ref ? { type: 'imageReference', identifier: ref.identifier, label: ref.label, referenceType: ref.referenceType, alt: attrs.alt || '' }
          : { type: 'image', url: attrs.src || '', alt: attrs.alt || '', title: attrs.title || null }; break;
      }
      case 'preservedInline': value = { type: 'html', value: attrs.raw || '' }; break;
      case 'preservedSpan': value = { type: 'html', value: `${attrs.opening}${writeInline(node.content)}${attrs.closing}` }; break;
      default: throw new DocumentFormatError(`暂时无法保存行内结构：${node.type}`);
    }
    parents.at(-1).push(value);
  }
  return result;
}

function writeInline(content) {
  return writeMarkdownTree({ type: 'root', children: [{ type: 'paragraph', children: inlineToTree(content) }] }).replace(/\n$/, '');
}

export function blockToTree(node) {
  const attrs = node.attrs || {};
  const blocks = () => (node.content || []).map(blockToTree);
  switch (node.type) {
    case 'paragraph': return node.content?.length ? { type: 'paragraph', children: inlineToTree(node.content) } : { type: 'html', value: '<br>' };
    case 'heading': return { type: 'heading', depth: attrs.level || 1, children: inlineToTree(node.content) };
    case 'horizontalRule': return { type: 'thematicBreak' };
    case 'codeBlock': return { type: 'code', lang: attrs.language || null, meta: attrs.meta || null, value: (node.content || []).map(child => child.text || '').join('') };
    case 'blockMath': return { type: 'math', value: attrs.formula || '', meta: attrs.meta || null };
    case 'blockquote': return { type: 'blockquote', children: blocks() };
    case 'callout': {
      const children = blocks();
      const marker = text(`[!${attrs.kind || 'NOTE'}]\n`);
      if (children[0]?.type === 'paragraph') children[0].children.unshift(marker);
      else children.unshift({ type: 'paragraph', children: [text(`[!${attrs.kind || 'NOTE'}]`)] });
      return { type: 'blockquote', children };
    }
    case 'bulletList': case 'orderedList': case 'taskList': return { type: 'list', ordered: node.type === 'orderedList' || Boolean(attrs.ordered), start: node.type === 'orderedList' || attrs.ordered ? (attrs.start || 1) : null,
      spread: (node.content || []).some(item => item.content?.length > 1), children: (node.content || []).map(item => ({ type: 'listItem', checked: item.type === 'taskItem' ? Boolean(item.attrs?.checked) : null, spread: item.content?.length > 1, children: (item.content || []).map(blockToTree) })) };
    case 'table': {
      const width = node.content?.[0]?.content?.length || 0;
      if (!width) throw new DocumentFormatError('表格至少需要一行一列');
      return { type: 'table', align: node.content[0].content.map(cell => cell.attrs?.textAlign || null), children: node.content.map(row => {
        if (row.content?.length !== width) throw new DocumentFormatError('表格行列结构不一致');
        return { type: 'tableRow', children: row.content.map(cell => {
          if ((cell.attrs?.colspan || 1) !== 1 || (cell.attrs?.rowspan || 1) !== 1 || cell.content?.length !== 1 || cell.content[0].type !== 'paragraph') throw new DocumentFormatError('表格中出现无法用 Markdown 保存的结构');
          return { type: 'tableCell', children: inlineToTree(cell.content[0].content, true) };
        }) };
      }) };
    }
    case 'footnoteDefinition': return { type: 'footnoteDefinition', identifier: attrs.identifier, label: attrs.identifier, children: blocks() };
    case 'documentMetadata': case 'preservedBlock': return { type: 'html', value: attrs.raw || '' };
    default: throw new DocumentFormatError(`暂时无法保存文稿结构：${node.type}`);
  }
}

export function serializeBlock(node) {
  if (node.type === 'preservedBlock' || node.type === 'documentMetadata') return node.attrs.raw;
  return writeMarkdownTree({ type: 'root', children: [blockToTree(node)] }).replace(/\n$/, '');
}
