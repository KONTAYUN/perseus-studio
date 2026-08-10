import React from 'react';
import { useEditor } from '@tiptap/react';
import { TextSelection } from '@tiptap/pm/state';
import RichTextEditorSurface from './RichTextEditorSurface';
import {
  createHiddenFloatingToolbarState,
  resolveFloatingToolbarPosition,
} from './floatingToolbarUtils';
import { decorateHtmlWithAssetUrls, getSanitizedEditorHtml } from './richTextHtmlUtils';
import NoteImage from './richTextImageExtension';
import NoteTable from './richTextTableExtension';
import {
  createHiddenBlockToolbarState,
  preserveToolbarState,
  resolveBlockTriggerLeft,
} from './richTextToolbarState';
import { createRichTextEditorExtensions } from './richTextEditorExtensions';
import useRichTextDocumentSync from './useRichTextDocumentSync';
import useRichTextImageUpload from './useRichTextImageUpload';
import useRichTextToolbarLifecycle from './useRichTextToolbarLifecycle';
import { buildRichTextSelectionContext } from './richTextSelectionContext';
import { getEditorTransactionOrigin } from './richTextTransactionOrigin';

export default function RichTextEditor({
  filePath,
  html,
  onHtmlChange,
  onSelectionChange,
  onStatusMessage,
  clearSelectionVersion = 0,
  pinnedSelections = [],
  readOnly = false,
  aiPreview = null,
  onAIEditReconciled,
}) {
  const imageInputRef = React.useRef(null);
  const isPointerSelectingRef = React.useRef(false);
  const isBlockToolbarInteractionRef = React.useRef(false);
  const lastCommittedHtmlRef = React.useRef(html || '');
  const lastRenderedHtmlRef = React.useRef(html || '');
  const readOnlyRef = React.useRef(readOnly);
  readOnlyRef.current = readOnly;
  const lastSelectionToolbarPlacementRef = React.useRef('above');
  const lastTableToolbarPlacementRef = React.useRef('above');
  const [pendingTableConfig, setPendingTableConfig] = React.useState({
    open: false,
    rows: '3',
    cols: '3',
    withHeaderRow: true,
  });
  const [selectionToolbarState, setSelectionToolbarState] = React.useState(() =>
    createHiddenFloatingToolbarState({ range: null })
  );
  const [tableToolbarState, setTableToolbarState] = React.useState(() =>
    createHiddenFloatingToolbarState({ caption: '' })
  );
  const [blockToolbarState, setBlockToolbarState] = React.useState(() =>
    createHiddenBlockToolbarState()
  );

  const syncSelectionToolbar = React.useCallback((nextEditor) => {
    const hide = () =>
      setSelectionToolbarState((current) =>
        preserveToolbarState(current, createHiddenFloatingToolbarState({ range: null }))
      );
    if (!nextEditor) {
      hide();
      return;
    }
    if (isPointerSelectingRef.current) {
      setSelectionToolbarState((current) =>
        current.visible
          ? preserveToolbarState(current, { ...current, visible: false, position: null })
          : current
      );
      return;
    }
    const { from, to, empty } = nextEditor.state.selection;
    if (!(nextEditor.state.selection instanceof TextSelection)) {
      hide();
      return;
    }
    if (empty || from === to) {
      hide();
      return;
    }
    const start = nextEditor.view.coordsAtPos(from);
    const end = nextEditor.view.coordsAtPos(to);
    const editorRect = nextEditor.view.dom.getBoundingClientRect();
    const selectionTop = Math.min(start.top, end.top);
    const selectionBottom = Math.max(start.bottom, end.bottom);
    const { placement, top } = resolveFloatingToolbarPosition({
      anchorTop: selectionTop,
      anchorBottom: selectionBottom,
      fallbackTop: selectionTop,
      previousPlacement: lastSelectionToolbarPlacementRef.current,
      viewportTop: editorRect.top,
    });

    lastSelectionToolbarPlacementRef.current = placement;
    setSelectionToolbarState((current) =>
      preserveToolbarState(current, {
        visible: true,
        position: {
          left: (start.left + end.right) / 2,
          top,
        },
        range: { from, to },
        placement,
      })
    );
  }, []);

  const syncTableToolbar = React.useCallback((nextEditor) => {
    const hide = () =>
      setTableToolbarState((current) =>
        preserveToolbarState(current, createHiddenFloatingToolbarState({ caption: '' }))
      );
    if (!nextEditor || !nextEditor.isActive('table')) {
      hide();
      return;
    }

    const domAtPos = nextEditor.view.domAtPos(nextEditor.state.selection.from);
    const baseNode =
      domAtPos.node?.nodeType === Node.TEXT_NODE ? domAtPos.node.parentElement : domAtPos.node;
    const cellElement = baseNode?.closest?.('td,th');
    const tableElement = baseNode?.closest?.('table');

    if (!tableElement) {
      hide();
      return;
    }

    const tableRect = tableElement.getBoundingClientRect();
    const anchorRect = cellElement?.getBoundingClientRect?.() || tableRect;
    const editorRect = nextEditor.view.dom.getBoundingClientRect();
    const { placement, top } = resolveFloatingToolbarPosition({
      anchorTop: tableRect.top,
      anchorBottom: anchorRect.bottom,
      fallbackTop: tableRect.top,
      previousPlacement: lastTableToolbarPlacementRef.current,
      viewportTop: editorRect.top,
    });

    lastTableToolbarPlacementRef.current = placement;
    setTableToolbarState((current) =>
      preserveToolbarState(current, {
        caption: tableElement.getAttribute('data-caption') || '',
        visible: true,
        position: {
          left: anchorRect.left + anchorRect.width / 2,
          top,
        },
        placement,
      })
    );
  }, []);

  const syncBlockToolbarTrigger = React.useCallback(
    (nextEditor) => {
      if (!nextEditor) {
        setBlockToolbarState((current) =>
          current.triggerVisible || current.toolbarVisible ? createHiddenBlockToolbarState() : current
        );
        return;
      }

      const { selection } = nextEditor.state;
      if (!selection.empty || nextEditor.isActive('table')) {
        setBlockToolbarState((current) =>
          preserveToolbarState(current, {
            ...current,
            triggerVisible: false,
            triggerPosition: null,
            toolbarVisible: false,
            toolbarPosition: null,
          })
        );
        return;
      }

      const domAtPos = nextEditor.view.domAtPos(selection.from);
      const baseNode =
        domAtPos.node?.nodeType === Node.TEXT_NODE ? domAtPos.node.parentElement : domAtPos.node;
      const blockElement = baseNode?.closest?.(
        'li, p, h1, h2, h3, h4, h5, h6, blockquote, pre'
      );

      if (!blockElement) {
        setBlockToolbarState((current) =>
          preserveToolbarState(current, {
            ...current,
            triggerVisible: false,
            triggerPosition: null,
            toolbarVisible: false,
            toolbarPosition: null,
          })
        );
        return;
      }

      const blockRect = blockElement.getBoundingClientRect();
      const editorRect = nextEditor.view.dom.getBoundingClientRect();
      if (blockRect.bottom <= editorRect.top || blockRect.top >= editorRect.bottom) {
        setBlockToolbarState((current) =>
          preserveToolbarState(current, {
            ...current,
            triggerVisible: false,
            triggerPosition: null,
            toolbarVisible: false,
            toolbarPosition: null,
          })
        );
        return;
      }

      const triggerLeft = resolveBlockTriggerLeft(blockElement, blockRect, editorRect);
      const triggerTop = Math.max(
        blockRect.top + Math.max(Math.min((blockRect.height - 24) / 2, 12), 0),
        editorRect.top + 6
      );

      setBlockToolbarState((current) =>
        preserveToolbarState(current, {
          triggerVisible: true,
          triggerPosition: {
            left: triggerLeft,
            top: triggerTop,
          },
          toolbarVisible: current.toolbarVisible,
          toolbarPosition:
            current.toolbarVisible
              ? {
                  left: triggerLeft - 2,
                  top: triggerTop + 28,
                }
              : null,
        })
      );
    },
    []
  );

  const editor = useEditor({
    content: decorateHtmlWithAssetUrls(filePath, html || ''),
    immediatelyRender: false,
    shouldRerenderOnTransaction: false,
    editable: !readOnly,
    extensions: createRichTextEditorExtensions(),
    editorProps: {
      attributes: {
        class:
          'rich-text-editor-content prose prose-neutral dark:prose-invert max-w-none custom-scrollbar h-full overflow-y-auto px-16 py-8 pb-32 outline-none max-[900px]:px-10 max-[640px]:px-6',
      },
      handlePaste: (_view, event) => {
        const items = Array.from(event?.clipboardData?.items || []);
        const imageItem = items.find((item) => item.type.startsWith('image/'));
        if (!imageItem) return false;
        const file = imageItem.getAsFile();
        if (!file) return false;
        const extension = file.type.split('/')[1] || 'png';
        const normalizedFile = file.name
          ? file
          : new File(
              [file],
              `clipboard-${new Date().toISOString().replace(/[:.]/g, '-')}.${extension}`,
              {
                type: file.type,
              }
            );
        event.preventDefault();
        void uploadImageFile(normalizedFile);
        return true;
      },
      handleDrop: (_view, event) => {
        const file = Array.from(event?.dataTransfer?.files || []).find((item) =>
          item.type?.startsWith('image/')
        );
        if (!file) return false;
        event.preventDefault();
        void uploadImageFile(file);
        return true;
      },
    },
    onUpdate({ editor: nextEditor, transaction }) {
      const transactionOrigin = getEditorTransactionOrigin(nextEditor, transaction);
      if (
        readOnlyRef.current ||
        transactionOrigin !== 'user'
      ) {
        return;
      }
      const nextHtml = getSanitizedEditorHtml(nextEditor);
      lastCommittedHtmlRef.current = nextHtml;
      lastRenderedHtmlRef.current = nextHtml;
      onHtmlChange?.(nextHtml);
      if (selectionToolbarState.range) syncSelectionToolbar(nextEditor);
      syncTableToolbar(nextEditor);
      syncBlockToolbarTrigger(nextEditor);
    },
    onCreate({ editor: nextEditor }) {
      const normalizedHtml = getSanitizedEditorHtml(nextEditor);
      lastRenderedHtmlRef.current = normalizedHtml;
      syncTableToolbar(nextEditor);
      syncBlockToolbarTrigger(nextEditor);
    },
    onSelectionUpdate({ editor: nextEditor }) {
      syncSelectionToolbar(nextEditor);
      syncTableToolbar(nextEditor);
      syncBlockToolbarTrigger(nextEditor);
    },
    onBlur() {
      if (isBlockToolbarInteractionRef.current) {
        isBlockToolbarInteractionRef.current = false;
        return;
      }
    },
  });

  const uploadImageFile = useRichTextImageUpload({ editor, filePath, onStatusMessage });

  useRichTextDocumentSync({
    clearSelectionVersion,
    editor,
    filePath,
    html,
    aiPreview,
    onAIEditReconciled,
    lastCommittedHtmlRef,
    lastRenderedHtmlRef,
    lastSelectionToolbarPlacementRef,
    pinnedSelections,
    setSelectionToolbarState,
    createHiddenFloatingToolbarState,
  });

  React.useEffect(() => {
    if (!editor) return;
    readOnlyRef.current = readOnly;
    editor.setEditable(!readOnly);
  }, [editor, readOnly]);

  React.useEffect(() => {
    if (!editor?.view?.dom) return;
    const root = editor.view.dom;
    root.toggleAttribute('data-ai-preview', !!aiPreview && !aiPreview.conflict);
    root.toggleAttribute('data-ai-preview-delete', !!aiPreview?.deleted);
    const blockNodes = collectPreviewBlockNodes(root);
    blockNodes.forEach((node) => node.removeAttribute('data-ai-preview-change'));
    for (const change of aiPreview?.changes || []) {
      const index = Number.parseInt(String(change?.block_id || '').replace(/^b/, ''), 10) - 1;
      if (index >= 0 && blockNodes[index]) {
        blockNodes[index].setAttribute('data-ai-preview-change', change.operation || 'replace_html');
      }
      if (change?.operation === 'delete_block_range') {
        const endIndex =
          Number.parseInt(String(change?.end_block_id || '').replace(/^b/, ''), 10) - 1;
        for (let rangeIndex = index; rangeIndex <= endIndex; rangeIndex += 1) {
          blockNodes[rangeIndex]?.setAttribute('data-ai-preview-change', 'delete');
        }
      }
    }
    return () => {
      root.removeAttribute('data-ai-preview');
      root.removeAttribute('data-ai-preview-delete');
      blockNodes.forEach((node) => node.removeAttribute('data-ai-preview-change'));
    };
  }, [aiPreview, editor]);

  const hideSelectionToolbar = React.useCallback(() => {
    lastSelectionToolbarPlacementRef.current = selectionToolbarState.placement || 'above';
    setSelectionToolbarState((current) =>
      current.visible ? { ...current, visible: false, position: null } : current
    );
  }, [selectionToolbarState.placement]);

  const refreshSelectionToolbar = React.useCallback(() => {
    syncSelectionToolbar(editor);
  }, [editor, syncSelectionToolbar]);

  const hideTableToolbar = React.useCallback(() => {
    lastTableToolbarPlacementRef.current = tableToolbarState.placement || 'above';
    setTableToolbarState((current) =>
      current.visible ? { ...current, visible: false, position: null } : current
    );
  }, [tableToolbarState.placement]);

  const refreshTableToolbar = React.useCallback(() => {
    syncTableToolbar(editor);
  }, [editor, syncTableToolbar]);

  const hideBlockToolbar = React.useCallback(() => {
    setBlockToolbarState((current) => ({
      ...current,
      toolbarVisible: false,
      toolbarPosition: null,
    }));
  }, []);

  const refreshBlockToolbarTrigger = React.useCallback(() => {
    syncBlockToolbarTrigger(editor);
  }, [editor, syncBlockToolbarTrigger]);

  useRichTextToolbarLifecycle({
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
  });

  const handlePinSelection = React.useCallback(() => {
    if (!editor || !selectionToolbarState.range) return;
    const currentSelections = Array.isArray(pinnedSelections) ? pinnedSelections : [];
    const alreadyPinned = currentSelections.some(
      (selection) =>
        selection.start_offset === selectionToolbarState.range.from &&
        selection.end_offset === selectionToolbarState.range.to
    );
    if (alreadyPinned) return;
    const context = buildRichTextSelectionContext(editor, selectionToolbarState.range, filePath);
    if (!context) return;
    editor.commands.addPinnedSelection(selectionToolbarState.range);
    onSelectionChange?.([...currentSelections, context]);
  }, [editor, filePath, onSelectionChange, pinnedSelections, selectionToolbarState.range]);

  const handleInsertImage = async (event) => {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file) return;
    await uploadImageFile(file);
  };

  const handleApplyTableConfig = React.useCallback(
    (nextConfig) => {
      const rows = Math.min(12, Math.max(1, Number(nextConfig.rows) || 3));
      const cols = Math.min(8, Math.max(1, Number(nextConfig.cols) || 3));
      editor?.chain().focus().insertTable({ rows, cols, withHeaderRow: !!nextConfig.withHeaderRow }).run();
      setPendingTableConfig({
        ...nextConfig,
        open: false,
        rows: String(rows),
        cols: String(cols),
      });
    },
    [editor]
  );

  const handleBlockToolbarInteract = React.useCallback(() => {
    isBlockToolbarInteractionRef.current = true;
  }, []);

  const handleBlockToolbarToggle = React.useCallback(() => {
    setBlockToolbarState((current) => ({
      ...current,
      toolbarVisible: !current.toolbarVisible,
      toolbarPosition: current.triggerPosition
        ? {
            left: current.triggerPosition.left - 2,
            top: current.triggerPosition.top + 28,
          }
        : null,
    }));
  }, []);

  return (
    <RichTextEditorSurface
      blockToolbarState={blockToolbarState}
      editor={editor}
      imageInputRef={imageInputRef}
      pendingTableConfig={pendingTableConfig}
      pinnedSelections={pinnedSelections}
      selectionToolbarState={selectionToolbarState}
      tableToolbarState={tableToolbarState}
      onApplyTableConfig={handleApplyTableConfig}
      onBlockToolbarInteract={handleBlockToolbarInteract}
      onBlockToolbarToggle={handleBlockToolbarToggle}
      onHideBlockToolbar={hideBlockToolbar}
      onHideSelectionToolbar={hideSelectionToolbar}
      onInsertImage={handleInsertImage}
      onPinSelection={handlePinSelection}
      onTableConfigChange={setPendingTableConfig}
    />
  );
}

function collectPreviewBlockNodes(root) {
  const blockTags = new Set(['H1', 'H2', 'H3', 'H4', 'H5', 'H6', 'P', 'BLOCKQUOTE', 'LI', 'PRE', 'TABLE', 'IMG']);
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
