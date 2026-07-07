import React from 'react';
import useFloatingToolbarViewportBehavior from './useFloatingToolbarViewportBehavior';

export default function useRichTextToolbarLifecycle({
  blockToolbarState,
  editor,
  hideBlockToolbar,
  hideSelectionToolbar,
  hideTableToolbar,
  isPointerSelectingRef,
  refreshBlockToolbarTrigger,
  refreshSelectionToolbar,
  refreshTableToolbar,
  selectionToolbarState,
  syncBlockToolbarTrigger,
  syncSelectionToolbar,
  tableToolbarState,
}) {
  React.useEffect(() => {
    if (!editor || typeof ResizeObserver === 'undefined') return undefined;
    const editorDom = editor.view.dom;
    const scroller = editorDom.parentElement;
    let frame = 0;
    const scheduleRefresh = () => {
      if (frame) cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        frame = 0;
        refreshSelectionToolbar();
        refreshTableToolbar();
        refreshBlockToolbarTrigger();
      });
    };
    const observer = new ResizeObserver(scheduleRefresh);
    observer.observe(editorDom);
    if (scroller) observer.observe(scroller);
    return () => {
      if (frame) cancelAnimationFrame(frame);
      observer.disconnect();
    };
  }, [editor, refreshBlockToolbarTrigger, refreshSelectionToolbar, refreshTableToolbar]);

  useFloatingToolbarViewportBehavior({
    visible: selectionToolbarState.visible,
    onScroll: hideSelectionToolbar,
    onResize: refreshSelectionToolbar,
  });

  useFloatingToolbarViewportBehavior({
    visible: tableToolbarState.visible,
    onScroll: hideTableToolbar,
    onResize: refreshTableToolbar,
  });

  useFloatingToolbarViewportBehavior({
    visible: blockToolbarState.triggerVisible || blockToolbarState.toolbarVisible,
    onScroll: () => {
      hideBlockToolbar();
      refreshBlockToolbarTrigger();
    },
    onResize: refreshBlockToolbarTrigger,
  });

  React.useEffect(() => {
    if (!editor) return undefined;
    const scroller = editor.view.dom.parentElement;
    if (!scroller) return undefined;
    const handleScroll = () => {
      hideBlockToolbar();
      hideSelectionToolbar();
      hideTableToolbar();
      syncBlockToolbarTrigger(editor);
    };
    scroller.addEventListener('scroll', handleScroll, { passive: true });
    return () => scroller.removeEventListener('scroll', handleScroll);
  }, [editor, hideBlockToolbar, hideSelectionToolbar, hideTableToolbar, syncBlockToolbarTrigger]);

  React.useEffect(() => {
    if (!editor) return undefined;
    const editorDom = editor.view.dom;
    const handlePointerDown = () => {
      isPointerSelectingRef.current = true;
      hideSelectionToolbar();
    };
    const handlePointerUp = () => {
      if (!isPointerSelectingRef.current) return;
      isPointerSelectingRef.current = false;
      requestAnimationFrame(() => {
        syncSelectionToolbar(editor);
        syncBlockToolbarTrigger(editor);
      });
    };
    editorDom.addEventListener('mousedown', handlePointerDown);
    window.addEventListener('mouseup', handlePointerUp);
    return () => {
      editorDom.removeEventListener('mousedown', handlePointerDown);
      window.removeEventListener('mouseup', handlePointerUp);
    };
  }, [
    editor,
    hideSelectionToolbar,
    isPointerSelectingRef,
    syncBlockToolbarTrigger,
    syncSelectionToolbar,
  ]);
}
