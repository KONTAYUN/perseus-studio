import { Node } from '@tiptap/core';

function preservedNode(inline) {
  const name = inline ? 'preservedInline' : 'preservedBlock';
  return Node.create({
    name, group: inline ? 'inline' : 'block', inline, atom: true, selectable: true,
    addAttributes() {
      return {
        raw: { default: '', parseHTML: element => element.getAttribute('data-preserved-raw') || '', rendered: false },
        label: { default: '特殊内容', rendered: false },
      };
    },
    parseHTML() { return [{ tag: `${inline ? 'span' : 'div'}[data-preserved-markdown="${name}"]` }]; },
    renderHTML({ node }) {
      return [inline ? 'span' : 'div', { 'data-preserved-markdown': name, 'data-preserved-raw': node.attrs.raw }, node.attrs.raw];
    },
    renderText({ node }) { return node.attrs.raw; },
  });
}
export const PreservedBlock = preservedNode(false);
export const PreservedInline = preservedNode(true);
