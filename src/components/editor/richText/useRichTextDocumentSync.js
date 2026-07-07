import React from 'react';
import { decorateHtmlWithAssetUrls } from './richTextHtmlUtils';

export default function useRichTextDocumentSync({
  clearSelectionVersion,
  editor,
  filePath,
  html,
  lastCommittedHtmlRef,
  lastSelectionToolbarPlacementRef,
  pinnedSelections,
  setSelectionToolbarState,
  createHiddenFloatingToolbarState,
}) {
  React.useEffect(() => {
    if (!editor) return;
    const nextHtml = html || '';
    if (nextHtml === lastCommittedHtmlRef.current) return;
    lastCommittedHtmlRef.current = nextHtml;
    editor.commands.setContent(decorateHtmlWithAssetUrls(filePath, nextHtml), false);
  }, [editor, filePath, html, lastCommittedHtmlRef]);

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
