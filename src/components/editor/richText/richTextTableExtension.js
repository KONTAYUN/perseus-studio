import Table from '@tiptap/extension-table';

const NoteTable = Table.extend({
  addAttributes() {
    return {
      ...this.parent?.(),
      caption: {
        default: '',
        parseHTML: (element) => element.getAttribute('data-caption') || '',
        renderHTML: (attributes) =>
          attributes.caption ? { 'data-caption': attributes.caption } : {},
      },
    };
  },

  renderHTML({ HTMLAttributes }) {
    const caption = HTMLAttributes['data-caption'] || '';
    return [
      'table',
      HTMLAttributes,
      ...(caption ? [['caption', { class: 'note-table-caption' }, caption]] : []),
      ['tbody', 0],
    ];
  },
});

export default NoteTable;
