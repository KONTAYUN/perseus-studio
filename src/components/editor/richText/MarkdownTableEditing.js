import { Extension } from '@tiptap/core';
import { Plugin } from '@tiptap/pm/state';
import { paragraphOutsideTable, selectedTable } from './tableActions.js';

export const MarkdownTableEditing = Extension.create({
  name: 'markdownTableEditing',
  priority: 1100,
  addKeyboardShortcuts() {
    return {
      // A GFM cell is inline content. Use a real break rather than creating
      // paragraphs whose structure cannot be represented in a Markdown cell.
      Enter: () => selectedTable(this.editor) ? this.editor.commands.setHardBreak() : false,
      'Mod-Enter': () => paragraphOutsideTable(this.editor),
    };
  },
  addProseMirrorPlugins() {
    return [new Plugin({
      appendTransaction(transactions, _previous, state) {
        if (!transactions.some(transaction => transaction.docChanged)) return null;
        const tr = state.tr;
        // Every edit path, including clipboard paste and undo, must preserve
        // GFM's single header row. Keep text, alignment and cell contents.
        state.doc.descendants((node, pos) => {
          if (node.type.name !== 'table') return;
          node.forEach((row, rowOffset, index) => {
            const type = state.schema.nodes[index === 0 ? 'tableHeader' : 'tableCell'];
            row.forEach((cell, cellOffset) => {
              if (cell.type !== type) tr.setNodeMarkup(pos + 2 + rowOffset + cellOffset, type, cell.attrs);
            });
          });
        });
        return tr.docChanged ? tr : null;
      },
    })];
  },
});
