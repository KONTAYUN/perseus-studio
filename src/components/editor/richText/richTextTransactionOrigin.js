import { Extension } from '@tiptap/core';
import { Plugin, PluginKey } from '@tiptap/pm/state';

const transactionOriginPluginKey = new PluginKey('richTextTransactionOrigin');
const contextMetaKey = 'richTextTransactionContext';

const idleContext = Object.freeze({
  origin: 'user',
  mutationId: '',
  documentVersion: '',
  phase: 'idle',
  lastOrigin: 'user',
});

export function reduceTransactionOriginState(previous, transaction) {
  const context = transaction.context;
  if (context?.action === 'begin') {
    return {
      origin: context.origin || 'external_reload',
      mutationId: context.mutationId || '',
      documentVersion: context.documentVersion || '',
      phase: context.phase || 'reconciling',
      lastOrigin: previous.lastOrigin,
    };
  }
  if (context?.action === 'end') {
    return { ...idleContext, lastOrigin: previous.lastOrigin };
  }
  if (!transaction.docChanged) return previous;
  const origin =
    transaction.explicitOrigin ||
    (previous.phase !== 'idle' ? previous.origin : 'user');
  return { ...previous, lastOrigin: origin };
}

export const RichTextTransactionOrigin = Extension.create({
  name: 'richTextTransactionOrigin',

  addProseMirrorPlugins() {
    return [
      new Plugin({
        key: transactionOriginPluginKey,
        state: {
          init: () => ({ ...idleContext }),
          apply(transaction, previous) {
            return reduceTransactionOriginState(previous, {
              context: transaction.getMeta(contextMetaKey),
              docChanged: transaction.docChanged,
              explicitOrigin: transaction.getMeta('transactionOrigin'),
            });
          },
        },
      }),
    ];
  },
});

export function beginEditorTransactionContext(editor, context = {}) {
  if (!editor?.view) return;
  const transaction = editor.state.tr.setMeta(contextMetaKey, {
    action: 'begin',
    origin: context.origin,
    mutationId: context.mutationId,
    documentVersion: context.documentVersion,
    phase: context.phase,
  });
  transaction.setMeta('addToHistory', false);
  editor.view.dispatch(transaction);
}

export function endEditorTransactionContext(editor) {
  if (!editor?.view) return;
  const transaction = editor.state.tr.setMeta(contextMetaKey, { action: 'end' });
  transaction.setMeta('addToHistory', false);
  editor.view.dispatch(transaction);
}

export function getEditorTransactionContext(editor) {
  if (!editor?.state) return idleContext;
  return transactionOriginPluginKey.getState(editor.state) || idleContext;
}

export function getEditorTransactionOrigin(editor, transaction) {
  return (
    transaction?.getMeta?.('transactionOrigin') ||
    getEditorTransactionContext(editor).lastOrigin ||
    'user'
  );
}
