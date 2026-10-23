import { Node } from '@tiptap/core';
import { readInlineSpan } from '../../markdown/inlineSpan.js';

// An inline container keeps code/bold/link children editable without losing
// wrappers when a code mark excludes other marks. Original HTML stays inert.
export const PreservedSpan = Node.create({
  name: 'preservedSpan',
  group: 'inline', inline: true, content: 'inline*',
  addAttributes() {
    return {
      opening: { default: '<span>', rendered: false, parseHTML: element => {
        const value = element.getAttribute('data-span-opening');
        const span = value && readInlineSpan(`${value}</span>`);
        return span?.opening === value ? value : '<span>';
      } },
      closing: { default: '</span>', rendered: false, parseHTML: element => {
        const value = element.getAttribute('data-span-closing');
        return /^<\/span\s*>$/i.test(value || '') ? value : '</span>';
      } },
    };
  },
  parseHTML() { return [{ tag: 'span[data-preserved-span]' }]; },
  renderHTML({ node }) {
    return ['span', { 'data-preserved-span': '', 'data-span-opening': node.attrs.opening, 'data-span-closing': node.attrs.closing }, 0];
  },
  markdownTokenizer: {
    name: 'preservedSpan', level: 'inline',
    start: source => source.search(/<span(?=[\s>])/i),
    tokenize(source, _tokens, helpers) {
      const span = readInlineSpan(source);
      if (!span) return undefined;
      return { type: 'preservedSpan', ...span, tokens: helpers.inlineTokens(span.inner) };
    },
  },
  parseMarkdown(token, helpers) {
    return helpers.createNode('preservedSpan', { opening: token.opening, closing: token.closing }, helpers.parseInline(token.tokens));
  },
  renderMarkdown(node, helpers) {
    return `${node.attrs.opening}${helpers.renderChildren(node.content || [])}${node.attrs.closing}`;
  },
});
