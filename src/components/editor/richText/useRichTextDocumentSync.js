import React from 'react';
import { decorateHtmlWithAssetUrls } from './richTextHtmlUtils';
import {
  applyAIBlockChanges,
  applyAuthoritativeEditorHTML,
  captureEditorScrollAnchor,
  restoreEditorScrollAnchor,
} from './richTextAITransactions';
import {
  beginEditorTransactionContext,
  endEditorTransactionContext,
} from './richTextTransactionOrigin';
import {
  beginEditorBlockMapping,
  endEditorBlockMapping,
} from './richTextBlockMapping';

export default function useRichTextDocumentSync({
  clearSelectionVersion,
  editor,
  filePath,
  html,
  aiPreview = null,
  onAIEditReconciled,
  lastCommittedHtmlRef,
  lastRenderedHtmlRef,
  lastSelectionToolbarPlacementRef,
  pinnedSelections,
  setSelectionToolbarState,
  createHiddenFloatingToolbarState,
}) {
  const previewTimersRef = React.useRef([]);
  const stagedPreviewTextRef = React.useRef(new Map());
  const stagedPreviewHTMLRef = React.useRef(new Map());
  const stagedInsertionsRef = React.useRef(new Map());
  const activePreviewIDRef = React.useRef('');
  const lastFilePathRef = React.useRef('');

  React.useEffect(() => {
    if (!editor) return;
    const nextHtml = html || '';
    const fileChanged = filePath !== lastFilePathRef.current;
    const reconciling = aiPreview?.status === 'reconciling';
    if (!fileChanged && nextHtml === lastRenderedHtmlRef.current) {
      if (reconciling && aiPreview?.preview_id) {
        onAIEditReconciled?.(aiPreview.preview_id);
      }
      return;
    }
    lastFilePathRef.current = filePath;
    lastRenderedHtmlRef.current = nextHtml;
    const previewActive = !!aiPreview && !aiPreview.conflict && !reconciling;
    if (!previewActive) {
      lastCommittedHtmlRef.current = nextHtml;
    }
    if (!fileChanged && previewActive) {
      lastCommittedHtmlRef.current = nextHtml;
      return;
    }
    const decoratedHtml = decorateHtmlWithAssetUrls(filePath, nextHtml);
    const anchor = captureEditorScrollAnchor(editor);
    beginEditorTransactionContext(editor, {
      origin: 'external_reload',
      documentVersion: aiPreview?.document_version || '',
      phase: 'reconciling',
    });
    try {
      const appliedIncrementally =
        !fileChanged && applyAuthoritativeEditorHTML(editor, decoratedHtml);
      if (!appliedIncrementally) {
        editor.commands.setContent(decoratedHtml, false);
      }
      restoreEditorScrollAnchor(editor, anchor);
    } finally {
      endEditorTransactionContext(editor);
    }
    if (reconciling && aiPreview?.preview_id) {
      onAIEditReconciled?.(aiPreview.preview_id);
    }
  }, [
    aiPreview?.conflict,
    aiPreview?.status,
    editor,
    filePath,
    html,
    lastCommittedHtmlRef,
    lastRenderedHtmlRef,
    onAIEditReconciled,
  ]);

  React.useEffect(() => {
    if (!editor || !aiPreview || aiPreview.conflict || aiPreview.status === 'committing') return;
    if (!Array.isArray(aiPreview.changes) || aiPreview.changes.length === 0) return;
    const previewID = aiPreview.preview_id || '';
    if (activePreviewIDRef.current !== previewID) {
      activePreviewIDRef.current = previewID;
      beginEditorBlockMapping(editor);
    }
    beginEditorTransactionContext(editor, {
      origin: 'ai_preview',
      mutationId: aiPreview.mutation_id || aiPreview.tool_call_id || '',
      documentVersion: aiPreview.document_version || '',
      phase: 'previewing',
    });
    cancelPreviewTimers(previewTimersRef);
    const prepared = aiPreview.changes.map((change) => {
      if (change?.operation === 'insert_before' || change?.operation === 'insert_after') {
        const key = `${aiPreview.preview_id}:${change.block_id}:${change.operation}`;
        const insertedBlockID = stagedInsertionsRef.current.get(key);
        if (insertedBlockID) {
          return { ...change, block_id: insertedBlockID, operation: 'replace_html' };
        }
        return {
          ...change,
          inserted_block_id: `preview:${aiPreview.preview_id}:${change.block_id}:${change.operation}`,
        };
      }
      if (change?.operation !== 'replace_text') return change;
      const key = `${aiPreview.preview_id}:${change.block_id}`;
      const staged = stagedPreviewTextRef.current.get(key);
      return staged ? { ...change, find: staged.current } : change;
    });
    const progressiveHTML = prepared.filter(
      (change) =>
        (change?.operation === 'replace_html' || change?.operation === 'replace_inner_html') &&
        visibleHTMLTextLength(change.html) >= 12
    );
    const progressiveText = prepared.filter(
      (change) =>
        change?.operation === 'replace_text' &&
        Array.from(change.replace || '').length >= 8 &&
        Array.from(change.replace || '').length >= Array.from(change.find || '').length
    );
    const immediate = prepared.filter(
      (change) => !progressiveHTML.includes(change) && !progressiveText.includes(change)
    );
    if (applyPreviewChanges(editor, immediate)) {
      for (const change of immediate) {
        if (change?.operation === 'insert_before' || change?.operation === 'insert_after') {
          const key = `${aiPreview.preview_id}:${change.block_id}:${change.operation}`;
          stagedInsertionsRef.current.set(
            key,
            change.inserted_block_id
          );
        }
        if (change?.operation !== 'replace_text') continue;
        const key = `${aiPreview.preview_id}:${change.block_id}`;
        stagedPreviewTextRef.current.set(key, {
          current: change.replace || '',
          original: stagedPreviewTextRef.current.get(key)?.original || change.find || '',
        });
      }
    }

    for (const change of prepared) {
      if (immediate.includes(change)) continue;
      if (change?.operation === 'replace_html' || change?.operation === 'replace_inner_html') {
        const key = `${aiPreview.preview_id}:${change.block_id}`;
        const stagedHTML = stagedPreviewHTMLRef.current.get(key) || '';
        const frames = buildProgressiveHTMLFrames(change.html, visibleHTMLTextLength(stagedHTML));
        frames.forEach((nextHTML, frame) => {
          const timer = window.setTimeout(() => {
            const applied = applyPreviewChanges(
              editor,
              [{ ...change, html: nextHTML }],
              null
            );
            if (applied) stagedPreviewHTMLRef.current.set(key, nextHTML);
          }, (frame + 1) * 28);
          previewTimersRef.current.push(timer);
        });
        continue;
      }
      const key = `${aiPreview.preview_id}:${change.block_id}`;
      const state = stagedPreviewTextRef.current.get(key) || {
        current: change.find || '',
        original: change.find || '',
      };
      const target = Array.from(change.replace || '');
      const startLength = Array.from(state.current).length;
      const frameCount = Math.min(120, Math.max(1, target.length - startLength));
      const chunkSize = Math.max(1, Math.ceil((target.length - startLength) / frameCount));
      for (let length = startLength + chunkSize, frame = 1; length <= target.length + chunkSize; length += chunkSize, frame += 1) {
        const next = target.slice(0, Math.min(length, target.length)).join('');
        const timer = window.setTimeout(() => {
          const latest = stagedPreviewTextRef.current.get(key) || state;
          const applied = applyPreviewChanges(
            editor,
            [{ ...change, find: latest.current, replace: next }],
            null
          );
          if (applied) {
            stagedPreviewTextRef.current.set(key, { ...latest, current: next });
          }
        }, frame * 28);
        previewTimersRef.current.push(timer);
        if (next === target.join('')) break;
      }
    }
    return () => cancelPreviewTimers(previewTimersRef);
  }, [
    aiPreview?.conflict,
    aiPreview?.preview_id,
    aiPreview?.revision,
    aiPreview?.status,
    editor,
  ]);

  React.useEffect(() => {
    if (!editor || aiPreview?.status !== 'committing' || !Array.isArray(aiPreview.changes)) return;
    cancelPreviewTimers(previewTimersRef);
    const finalChanges = aiPreview.changes.map((change) => {
      if (change?.operation === 'insert_before' || change?.operation === 'insert_after') {
        const insertionKey = `${aiPreview.preview_id}:${change.block_id}:${change.operation}`;
        const insertedBlockID = stagedInsertionsRef.current.get(insertionKey);
        return insertedBlockID
          ? { ...change, block_id: insertedBlockID, operation: 'replace_html' }
          : change;
      }
      if (change?.operation !== 'replace_text') return change;
      const key = `${aiPreview.preview_id}:${change.block_id}`;
      const staged = stagedPreviewTextRef.current.get(key);
      return staged ? { ...change, find: staged.current } : change;
    });
    beginEditorTransactionContext(editor, {
      origin: 'authoritative_sync',
      mutationId: aiPreview.mutation_id || aiPreview.tool_call_id || '',
      documentVersion: aiPreview.document_version || '',
      phase: 'committing',
    });
    applyPreviewChanges(editor, finalChanges, null, {
      final: true,
      origin: 'authoritative_sync',
    });
  }, [aiPreview?.document_version, aiPreview?.mutation_id, aiPreview?.preview_id, aiPreview?.revision, aiPreview?.status, aiPreview?.tool_call_id, editor]);

  React.useEffect(() => {
    if (!editor || aiPreview || !activePreviewIDRef.current) return;
    cancelPreviewTimers(previewTimersRef);
    activePreviewIDRef.current = '';
    stagedPreviewTextRef.current.clear();
    stagedPreviewHTMLRef.current.clear();
    stagedInsertionsRef.current.clear();
    endEditorBlockMapping(editor);
    endEditorTransactionContext(editor);
  }, [aiPreview, editor]);

  React.useEffect(
    () => () => {
      cancelPreviewTimers(previewTimersRef);
      stagedPreviewTextRef.current.clear();
      stagedPreviewHTMLRef.current.clear();
      stagedInsertionsRef.current.clear();
    },
    []
  );

  React.useEffect(() => {
    if (!editor) return;
    if (!Array.isArray(pinnedSelections) || pinnedSelections.length === 0) {
      editor.commands.clearPinnedSelections();
      return;
    }
    editor.commands.setPinnedSelections(
      pinnedSelections.map((selection) => ({
        from: selection.start_offset,
        to: selection.end_offset,
      }))
    );
  }, [editor, pinnedSelections]);

  React.useEffect(() => {
    if (!editor) return;
    editor.commands.clearPinnedSelections();
    lastSelectionToolbarPlacementRef.current = 'above';
    setSelectionToolbarState(createHiddenFloatingToolbarState({ range: null }));
  }, [
    clearSelectionVersion,
    createHiddenFloatingToolbarState,
    editor,
    lastSelectionToolbarPlacementRef,
    setSelectionToolbarState,
  ]);
}

function applyPreviewChanges(editor, changes, _unused, options = {}) {
  if (!Array.isArray(changes) || changes.length === 0) return false;
  const anchor = captureEditorScrollAnchor(editor);
  return applyAIBlockChanges(editor, changes, {
    final: options.final,
    origin: options.origin || 'ai_preview',
    beforeDispatch: undefined,
    afterDispatch: () => {
      restoreEditorScrollAnchor(editor, anchor);
    },
  });
}

function cancelPreviewTimers(previewTimersRef) {
  for (const timer of previewTimersRef.current) window.clearTimeout(timer);
  previewTimersRef.current = [];
}

function visibleHTMLTextLength(html = '') {
  if (!html || typeof window === 'undefined') return 0;
  const container = window.document.createElement('div');
  container.innerHTML = html;
  return Array.from(container.textContent || '').length;
}

function buildProgressiveHTMLFrames(html = '', startLength = 0) {
  if (!html || typeof window === 'undefined') return [];
  const source = window.document.createElement('div');
  source.innerHTML = html;
  const total = Array.from(source.textContent || '').length;
  if (total <= startLength) return [html];
  const frameCount = Math.min(100, Math.max(1, total - startLength));
  const chunkSize = Math.max(1, Math.ceil((total - startLength) / frameCount));
  const frames = [];
  for (let length = startLength + chunkSize; length < total; length += chunkSize) {
    const clone = source.cloneNode(true);
    revealTextPrefix(clone, length);
    frames.push(clone.innerHTML);
  }
  frames.push(html);
  return frames;
}

function revealTextPrefix(root, length) {
  const walker = window.document.createTreeWalker(root, window.NodeFilter.SHOW_TEXT);
  let remaining = length;
  let node = walker.nextNode();
  while (node) {
    const characters = Array.from(node.nodeValue || '');
    if (remaining <= 0) {
      node.nodeValue = '';
    } else if (characters.length > remaining) {
      node.nodeValue = characters.slice(0, remaining).join('');
      remaining = 0;
    } else {
      remaining -= characters.length;
    }
    node = walker.nextNode();
  }
}
