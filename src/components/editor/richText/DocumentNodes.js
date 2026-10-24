import { Node } from '@tiptap/core';

export function atomNode(name, inline, attributes, tag) {
  return Node.create({
    name, group: inline ? 'inline' : 'block', inline, atom: true, selectable: true,
    addAttributes() { return Object.fromEntries(Object.entries(attributes).map(([key, value]) => [key, { default: value, rendered: false }])); },
    parseHTML() { return [{ tag: `${inline ? 'span' : 'div'}[data-document-node="${name}"]`, getAttrs: element => {
      try { const attrs = JSON.parse(element.getAttribute('data-document-attrs') || '{}'); return Object.fromEntries(Object.keys(attributes).map(key => [key, attrs[key] ?? attributes[key]])); } catch { return false; }
    } }]; },
    renderHTML({ node }) { return [inline ? 'span' : 'div', { 'data-document-node': name, 'data-document-attrs': JSON.stringify(node.attrs) }, node.attrs.formula || node.attrs.identifier || tag]; },
    renderText({ node }) { return node.attrs.formula || node.attrs.identifier || node.attrs.raw || tag; },
  });
}
export const InlineMath = atomNode('inlineMath', true, { formula: '' }, '公式');
export const BlockMath = atomNode('blockMath', false, { formula: '', meta: null }, '公式');
export const DocumentMetadata = atomNode('documentMetadata', false, { format: 'yaml', raw: '---\n---' }, '文档属性');
export const FootnoteReference = atomNode('footnoteReference', true, { identifier: '' }, '脚注');
export const FootnoteDefinition = Node.create({
  name: 'footnoteDefinition', group: 'block', content: 'block+', defining: true,
  addAttributes() { return { identifier: { default: 'note', parseHTML: element => element.getAttribute('data-footnote-definition'), renderHTML: attrs => ({ 'data-footnote-definition': attrs.identifier }) } }; },
  parseHTML() { return [{ tag: 'div[data-footnote-definition]' }]; },
  renderHTML({ HTMLAttributes }) { return ['div', { ...HTMLAttributes, class: 'rich-footnote-definition' }, 0]; },
});
export const Callout = Node.create({
  name: 'callout', group: 'block', content: 'block+', defining: true,
  addAttributes() { return { kind: { default: 'NOTE', parseHTML: element => element.getAttribute('data-callout'), renderHTML: attrs => ({ 'data-callout': ['NOTE', 'TIP', 'IMPORTANT', 'WARNING', 'CAUTION'].includes(attrs.kind) ? attrs.kind : 'NOTE' }) } }; },
  parseHTML() { return [{ tag: 'aside[data-callout]' }]; },
  renderHTML({ HTMLAttributes }) { return ['aside', { ...HTMLAttributes, class: 'rich-callout' }, 0]; },
});
