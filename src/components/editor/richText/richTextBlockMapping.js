import { Extension } from '@tiptap/core';
import { Plugin, PluginKey } from '@tiptap/pm/state';

const blockMappingPluginKey = new PluginKey('richTextBlockMapping');
const blockMappingMetaKey = 'richTextBlockMapping';
const noteBlockTypes = new Set([
  'heading',
  'paragraph',
  'blockquote',
  'listItem',
  'codeBlock',
  'table',
  'image',
]);

export const RichTextBlockMapping = Extension.create({
  name: 'richTextBlockMapping',

  addProseMirrorPlugins() {
    return [
      new Plugin({
        key: blockMappingPluginKey,
        state: {
          init: () => ({ active: false, positions: {} }),
          apply(transaction, previous) {
            const command = transaction.getMeta(blockMappingMetaKey);
            if (command?.action === 'begin') {
              return { active: true, positions: { ...command.positions } };
            }
            if (command?.action === 'end') {
              return { active: false, positions: {} };
            }
            if (!previous.active) return previous;
            const positions = {};
            for (const [blockID, position] of Object.entries(previous.positions)) {
              positions[blockID] = transaction.mapping.map(position, -1);
            }
            for (const registration of command?.registrations || []) {
              if (registration.blockID) {
                positions[registration.blockID] = registration.position;
              }
            }
            return { active: true, positions };
          },
        },
      }),
    ];
  },
});

export function beginEditorBlockMapping(editor) {
  if (!editor?.view) return;
  const positions = {};
  collectBlockPositions(editor.state.doc).forEach((item, index) => {
    positions[`b${String(index + 1).padStart(4, '0')}`] = item.pos;
  });
  const transaction = editor.state.tr.setMeta(blockMappingMetaKey, {
    action: 'begin',
    positions,
  });
  transaction.setMeta('addToHistory', false);
  editor.view.dispatch(transaction);
}

export function endEditorBlockMapping(editor) {
  if (!editor?.view) return;
  const transaction = editor.state.tr.setMeta(blockMappingMetaKey, { action: 'end' });
  transaction.setMeta('addToHistory', false);
  editor.view.dispatch(transaction);
}

export function registerMappedEditorBlock(transaction, blockID, position) {
  const current = transaction.getMeta(blockMappingMetaKey);
  transaction.setMeta(blockMappingMetaKey, {
    action: 'register',
    registrations: [
      ...(current?.registrations || []),
      { blockID, position },
    ],
  });
}

export function getMappedEditorBlock(editor, blockID) {
  const state = blockMappingPluginKey.getState(editor?.state);
  if (!state?.active) return null;
  const pos = state.positions?.[blockID];
  if (!Number.isFinite(pos)) return null;
  const node = editor.state.doc.nodeAt(pos);
  return node ? { node, pos } : null;
}

function collectBlockPositions(doc) {
  const blocks = [];
  doc.descendants((node, pos) => {
    if (!noteBlockTypes.has(node.type.name)) return true;
    blocks.push({ node, pos });
    return false;
  });
  return blocks;
}
