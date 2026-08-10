import { DOMParser as ProseMirrorDOMParser } from '@tiptap/pm/model';
import {
  getMappedEditorBlock,
  registerMappedEditorBlock,
} from './richTextBlockMapping';

const NOTE_BLOCK_TYPES = new Set([
  'heading',
  'paragraph',
  'blockquote',
  'listItem',
  'codeBlock',
  'table',
  'image',
]);

export function collectEditorBlockPositions(doc) {
  const blocks = [];
  doc.descendants((node, pos) => {
    if (!NOTE_BLOCK_TYPES.has(node.type.name)) return true;
    blocks.push({ node, pos });
    return false;
  });
  return blocks;
}

export function applyAIBlockChanges(editor, changes, options = {}) {
  if (!editor || !Array.isArray(changes) || changes.length === 0) return false;
  let transaction = editor.state.tr;
  let changed = false;
  const originalBlocks = collectEditorBlockPositions(editor.state.doc);
  const insertedBlocks = [];

  for (const change of changes) {
    const index = blockIndex(change?.block_id);
    const original =
      getMappedEditorBlock(editor, change?.block_id) ||
      (index >= 0 ? originalBlocks[index] : null);
    if (!original) continue;
    const mappedPos = transaction.mapping.map(original.pos, -1);
    const mappedNode = transaction.doc.nodeAt(mappedPos);
    const target = mappedNode ? { node: mappedNode, pos: mappedPos } : null;
    if (!target) continue;

    switch (change.operation) {
      case 'replace_text':
        changed = replaceTextInBlock(transaction, target, change.find, change.replace) || changed;
        break;
      case 'replace_html': {
        const replacement = parseReplacementNode(editor, change.html, target.node.type.name);
        if (!replacement) break;
        transaction = transaction.replaceWith(
          target.pos,
          target.pos + target.node.nodeSize,
          replacement
        );
        changed = true;
        break;
      }
      case 'replace_inner_html': {
        const replacement = parseInnerReplacementNode(editor, change.html, target.node);
        if (!replacement) break;
        transaction = transaction.replaceWith(
          target.pos + 1,
          target.pos + target.node.nodeSize - 1,
          replacement.content
        );
        changed = true;
        break;
      }
      case 'insert_before':
      case 'insert_after': {
        const replacement = parseReplacementNode(editor, change.html, target.node.type.name);
        if (!replacement) break;
        const insertAt =
          change.operation === 'insert_before'
            ? target.pos
            : target.pos + target.node.nodeSize;
        transaction = transaction.insert(insertAt, replacement);
        if (change.inserted_block_id) {
          insertedBlocks.push({
            blockID: change.inserted_block_id,
            position: insertAt,
            mappingStart: transaction.mapping.maps.length,
          });
        }
        changed = true;
        break;
      }
      case 'delete':
        if (options.final) {
          transaction = transaction.delete(target.pos, target.pos + target.node.nodeSize);
          changed = true;
        }
        break;
      case 'delete_block_range':
        if (options.final) {
          const endIndex = blockIndex(change.end_block_id);
          const endOriginal =
            getMappedEditorBlock(editor, change?.end_block_id) ||
            (endIndex >= 0 ? originalBlocks[endIndex] : null);
          if (!endOriginal || endIndex < index) break;
          const mappedEndPos = transaction.mapping.map(
            endOriginal.pos + endOriginal.node.nodeSize,
            1
          );
          transaction = transaction.delete(target.pos, mappedEndPos);
          changed = true;
        }
        break;
      default:
        break;
    }
  }

  if (!changed) return false;
  for (const inserted of insertedBlocks) {
    registerMappedEditorBlock(
      transaction,
      inserted.blockID,
      transaction.mapping.slice(inserted.mappingStart).map(inserted.position, -1)
    );
  }
  transaction.setMeta('addToHistory', false);
  transaction.setMeta('aiPreview', true);
  transaction.setMeta('transactionOrigin', options.origin || 'ai_preview');
  options.beforeDispatch?.();
  editor.view.dispatch(transaction);
  options.afterDispatch?.();
  return true;
}

export function captureEditorScrollAnchor(editor) {
  const root = editor?.view?.dom;
  if (!root) return null;
  const rootRect = root.getBoundingClientRect();
  const elements = collectEditorBlockElements(root);
  const index = elements.findIndex((element) => element.getBoundingClientRect().bottom > rootRect.top);
  const anchor = index >= 0 ? elements[index] : null;
  return {
    index,
    key: anchor ? editorBlockElementKey(anchor) : '',
    offset: anchor ? anchor.getBoundingClientRect().top - rootRect.top : 0,
    scrollTop: root.scrollTop,
    selection: {
      from: editor.state.selection.from,
      to: editor.state.selection.to,
    },
  };
}

export function restoreEditorScrollAnchor(editor, anchor) {
  const root = editor?.view?.dom;
  if (!root || !anchor) return;
  window.requestAnimationFrame(() => {
    if (anchor.index < 0) {
      root.scrollTop = anchor.scrollTop;
      restoreSelection(editor, anchor.selection);
      return;
    }
    const elements = collectEditorBlockElements(root);
    const element =
      (anchor.key && elements.find((item) => editorBlockElementKey(item) === anchor.key)) ||
      elements[anchor.index];
    if (!element) {
      root.scrollTop = anchor.scrollTop;
      restoreSelection(editor, anchor.selection);
      return;
    }
    const rootRect = root.getBoundingClientRect();
    const nextOffset = element.getBoundingClientRect().top - rootRect.top;
    root.scrollTop += nextOffset - anchor.offset;
    restoreSelection(editor, anchor.selection);
  });
}

export function applyAuthoritativeEditorHTML(editor, html) {
  if (!editor?.schema || typeof html !== 'string') return false;
  const container = window.document.createElement('div');
  container.innerHTML = html;
  const nextDoc = ProseMirrorDOMParser.fromSchema(editor.schema).parse(container, {
    preserveWhitespace: true,
  });
  const currentBlocks = collectEditorBlockPositions(editor.state.doc);
  const nextBlocks = collectEditorBlockPositions(nextDoc);
  if (currentBlocks.length !== nextBlocks.length || currentBlocks.length === 0) return false;

  let transaction = editor.state.tr;
  let changed = false;
  for (let index = 0; index < currentBlocks.length; index += 1) {
    const current = currentBlocks[index];
    const next = nextBlocks[index];
    if (!current?.node || !next?.node || current.node.type.name !== next.node.type.name) return false;
    if (current.node.eq(next.node)) continue;
    const mappedPos = transaction.mapping.map(current.pos, -1);
    const mappedNode = transaction.doc.nodeAt(mappedPos);
    if (!mappedNode || mappedNode.type.name !== next.node.type.name) return false;
    transaction = transaction.replaceWith(mappedPos, mappedPos + mappedNode.nodeSize, next.node);
    changed = true;
  }
  if (!changed) return true;
  transaction.setMeta('addToHistory', false);
  transaction.setMeta('transactionOrigin', 'authoritative_sync');
  editor.view.dispatch(transaction);
  return true;
}

export function collectEditorBlockElements(root) {
  const blockTags = new Set([
    'H1',
    'H2',
    'H3',
    'H4',
    'H5',
    'H6',
    'P',
    'BLOCKQUOTE',
    'LI',
    'PRE',
    'TABLE',
    'IMG',
  ]);
  const nodes = [];
  const walk = (node) => {
    for (const child of Array.from(node?.children || [])) {
      if (blockTags.has(child.tagName)) {
        nodes.push(child);
      } else {
        walk(child);
      }
    }
  };
  walk(root);
  return nodes;
}

function replaceTextInBlock(transaction, target, find, replacement) {
  if (typeof find !== 'string' || find === '') return false;
  const matches = [];
  target.node.descendants((node, relativePos) => {
    if (!node.isText) return true;
    let offset = node.text.indexOf(find);
    while (offset >= 0) {
      matches.push({
        from: target.pos + 1 + relativePos + offset,
        to: target.pos + 1 + relativePos + offset + find.length,
      });
      offset = node.text.indexOf(find, offset + find.length);
    }
    return true;
  });
  for (let index = matches.length - 1; index >= 0; index -= 1) {
    transaction.insertText(replacement || '', matches[index].from, matches[index].to);
  }
  return matches.length > 0;
}

function parseReplacementNode(editor, html, expectedType) {
  if (!editor?.schema || typeof html !== 'string' || html.trim() === '') return null;
  const container = window.document.createElement('div');
  container.innerHTML = html;
  const slice = ProseMirrorDOMParser.fromSchema(editor.schema).parseSlice(container, {
    preserveWhitespace: true,
  });
  let exact = null;
  slice.content.descendants((node) => {
    if (!exact && node.type.name === expectedType) exact = node;
    return !exact;
  });
  return exact || slice.content.firstChild || null;
}

function parseInnerReplacementNode(editor, html, targetNode) {
  if (!targetNode || typeof html !== 'string') return null;
  const wrapper = wrapperHTMLForNode(targetNode, html);
  return parseReplacementNode(editor, wrapper, targetNode.type.name);
}

function wrapperHTMLForNode(node, innerHTML) {
  const attrs = node.attrs || {};
  switch (node.type.name) {
    case 'heading':
      return `<h${attrs.level || 1}>${innerHTML}</h${attrs.level || 1}>`;
    case 'paragraph':
      return `<p>${innerHTML}</p>`;
    case 'blockquote':
      return `<blockquote>${innerHTML}</blockquote>`;
    case 'listItem':
      return `<li>${innerHTML}</li>`;
    case 'codeBlock':
      return `<pre><code>${innerHTML}</code></pre>`;
    case 'table':
      return `<table>${innerHTML}</table>`;
    default:
      return `<div>${innerHTML}</div>`;
  }
}

function blockIndex(blockID) {
  const parsed = Number.parseInt(String(blockID || '').replace(/^b/, ''), 10);
  return Number.isFinite(parsed) ? parsed - 1 : -1;
}

function editorBlockElementKey(element) {
  if (!element) return '';
  const text = (element.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 96);
  return `${element.tagName}:${text}`;
}

function restoreSelection(editor, selection) {
  if (!selection) return;
  const max = editor.state.doc.content.size;
  const from = Math.min(Math.max(0, selection.from), max);
  const to = Math.min(Math.max(from, selection.to), max);
  try {
    editor.commands.setTextSelection({ from, to });
  } catch {
    // The transaction already maps valid selections; this only restores setContent fallbacks.
  }
}
