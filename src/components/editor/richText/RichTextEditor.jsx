import React from 'react';
import { createPortal } from 'react-dom';
import { EditorContent, ReactNodeViewRenderer, useEditor } from '@tiptap/react';
import StarterKit from '@tiptap/starter-kit';
import Underline from '@tiptap/extension-underline';
import Highlight from '@tiptap/extension-highlight';
import TextStyle from '@tiptap/extension-text-style';
import Color from '@tiptap/extension-color';
import TextAlign from '@tiptap/extension-text-align';
import Link from '@tiptap/extension-link';
import Placeholder from '@tiptap/extension-placeholder';
import Image from '@tiptap/extension-image';
import Table from '@tiptap/extension-table';
import TableRow from '@tiptap/extension-table-row';
import TableHeader from '@tiptap/extension-table-header';
import TableCell from '@tiptap/extension-table-cell';
import TaskList from '@tiptap/extension-task-list';
import TaskItem from '@tiptap/extension-task-item';
import { TextSelection } from '@tiptap/pm/state';
import { apiUploadFile, workspaceFileContentUrl } from '../../../api/client';
import {
  buildNoteAssetUploadPath,
  getNoteAssetReferencePath,
  resolveNoteAssetPath,
} from '../../../hooks/noteTreeUtils.js';
import { BlockSpacing, FontSize, PinnedSelection } from './noteEditorExtensions';
import RichTextHistoryPanel from './RichTextHistoryPanel';
import RichTextImageNodeView from './RichTextImageNodeView';
import RichTextSelectionToolbar from './RichTextSelectionToolbar';
import RichTextTableToolbar from './RichTextTableToolbar';
import RichTextToolbar from './RichTextToolbar';
import {
  createHiddenFloatingToolbarState,
  resolveFloatingToolbarPosition,
} from './floatingToolbarUtils';
import useFloatingToolbarViewportBehavior from './useFloatingToolbarViewportBehavior';

function sanitizeTableMarkup(root) {
  root.querySelectorAll('table').forEach((table) => {
    const rows = Array.from(table.querySelectorAll('tr'));
    if (rows.length > 0) {
      const carry = [];
      let totalColumns = 0;

      rows.forEach((row) => {
        const cells = Array.from(row.children).filter((node) =>
          ['TD', 'TH'].includes(node.tagName)
        );
        let columnIndex = 0;

        cells.forEach((cell) => {
          while (carry[columnIndex] > 0) columnIndex += 1;
          const colspan = Math.max(1, Number(cell.getAttribute('colspan')) || 1);
          const rowspan = Math.max(1, Number(cell.getAttribute('rowspan')) || 1);
          for (let offset = 0; offset < colspan; offset += 1) {
            carry[columnIndex + offset] = Math.max(carry[columnIndex + offset] || 0, rowspan);
          }
          columnIndex += colspan;
        });

        const activeColumns = carry.reduce(
          (last, span, index) => (span > 0 ? index + 1 : last),
          columnIndex
        );
        totalColumns = Math.max(totalColumns, activeColumns);

        for (let index = 0; index < carry.length; index += 1) {
          if (carry[index] > 0) carry[index] -= 1;
        }
      });

      const fillCarry = [];
      rows.forEach((row) => {
        const cells = Array.from(row.children).filter((node) =>
          ['TD', 'TH'].includes(node.tagName)
        );
        let columnIndex = 0;

        cells.forEach((cell) => {
          while (fillCarry[columnIndex] > 0) columnIndex += 1;
          const colspan = Math.max(1, Number(cell.getAttribute('colspan')) || 1);
          const rowspan = Math.max(1, Number(cell.getAttribute('rowspan')) || 1);
          for (let offset = 0; offset < colspan; offset += 1) {
            fillCarry[columnIndex + offset] = Math.max(
              fillCarry[columnIndex + offset] || 0,
              rowspan
            );
          }
          columnIndex += colspan;
        });

        while (columnIndex < totalColumns) {
          while (fillCarry[columnIndex] > 0) columnIndex += 1;
          if (columnIndex >= totalColumns) break;
          const filler = row.ownerDocument.createElement('td');
          filler.appendChild(row.ownerDocument.createElement('p'));
          row.appendChild(filler);
          fillCarry[columnIndex] = Math.max(fillCarry[columnIndex] || 0, 1);
          columnIndex += 1;
        }

        for (let index = 0; index < fillCarry.length; index += 1) {
          if (fillCarry[index] > 0) fillCarry[index] -= 1;
        }
      });
    }

    table.querySelectorAll('colgroup').forEach((node) => node.remove());
    table.style.removeProperty('border');
    table.style.removeProperty('border-color');
    table.style.removeProperty('border-style');
    table.style.removeProperty('border-width');
    table.removeAttribute('data-table-width');
    table.removeAttribute('data-table-align');
    table.removeAttribute('data-border-style');
    table.removeAttribute('data-border-color');
    table.removeAttribute('data-border-width');
    table.removeAttribute('data-background-color');
    table.querySelectorAll('th, td').forEach((cell) => {
      cell.removeAttribute('colwidth');
      cell.removeAttribute('data-cell-width');
      cell.removeAttribute('data-border-style');
      cell.removeAttribute('data-border-color');
      cell.removeAttribute('data-border-width');
      cell.removeAttribute('data-background-color');
      cell.removeAttribute('data-cell-padding');
      cell.removeAttribute('data-cell-text-align');
      cell.removeAttribute('data-cell-vertical-align');
      cell.style.removeProperty('width');
      cell.style.removeProperty('min-width');
      cell.style.removeProperty('max-width');
      cell.style.removeProperty('border');
      cell.style.removeProperty('border-color');
      cell.style.removeProperty('border-style');
      cell.style.removeProperty('border-width');
    });
    table.querySelectorAll('tr').forEach((row) => {
      row.removeAttribute('data-row-height');
      row.style.removeProperty('height');
    });
  });
}

function decorateHtmlWithAssetUrls(notePath, html) {
  if (typeof window === 'undefined') return html;
  const template = window.document.createElement('template');
  template.innerHTML = html || '';
  sanitizeTableMarkup(template.content);
  template.content.querySelectorAll('img').forEach((image) => {
    const original = image.getAttribute('data-note-src') || image.getAttribute('src') || '';
    const resolvedPath = resolveNoteAssetPath(notePath, original);
    image.setAttribute('data-note-src', original);
    if (resolvedPath) {
      image.setAttribute('src', workspaceFileContentUrl(resolvedPath));
    }
  });
  return template.innerHTML;
}

function getSanitizedEditorHtml(editor) {
  if (typeof window === 'undefined') return editor.getHTML();
  const template = window.document.createElement('template');
  template.innerHTML = editor.getHTML();
  sanitizeTableMarkup(template.content);
  return template.innerHTML;
}

function createHiddenBlockToolbarState() {
  return {
    triggerVisible: false,
    triggerPosition: null,
    toolbarVisible: false,
    toolbarPosition: null,
  };
}

function shallowSameObject(left, right) {
  if (left === right) return true;
  if (!left || !right) return !left && !right;
  const leftKeys = Object.keys(left);
  const rightKeys = Object.keys(right);
  if (leftKeys.length !== rightKeys.length) return false;
  return leftKeys.every((key) => left[key] === right[key]);
}

function sameToolbarState(left, right) {
  if (!left || !right) return left === right;
  const keys = new Set([...Object.keys(left), ...Object.keys(right)]);
  for (const key of keys) {
    if (key === 'position' || key === 'range' || key === 'triggerPosition' || key === 'toolbarPosition') {
      if (!shallowSameObject(left[key], right[key])) return false;
      continue;
    }
    if (left[key] !== right[key]) return false;
  }
  return true;
}

function preserveToolbarState(current, next) {
  return sameToolbarState(current, next) ? current : next;
}

function clampImageWidth(value) {
  return String(Math.min(100, Math.max(20, Number(value) || 100)));
}

function normalizePixels(value, fallback = '') {
  if (value == null || value === '') return fallback;
  const numeric = Number(String(value).replace('px', ''));
  return Number.isFinite(numeric) && numeric >= 0 ? String(numeric) : fallback;
}

function buildImageStyle(attributes = {}) {
  const align = attributes.align || 'center';
  const widthPercent = clampImageWidth(attributes.widthPercent || '100');
  const margin =
    align === 'left'
      ? 'margin-right:auto;margin-left:0;'
      : align === 'right'
        ? 'margin-left:auto;margin-right:0;'
        : 'margin-left:auto;margin-right:auto;';
  const borderStyle =
    attributes.borderStyle && attributes.borderStyle !== 'none' ? attributes.borderStyle : 'none';
  const borderWidth = normalizePixels(attributes.borderWidth, '0');
  const borderColor = attributes.borderColor || 'transparent';
  const backgroundColor = attributes.backgroundColor || 'transparent';
  const borderRadius = normalizePixels(attributes.borderRadius, '18');
  return [
    `width:${widthPercent}%`,
    'display:block',
    'max-width:100%',
    margin,
    `border-style:${borderStyle}`,
    `border-width:${borderWidth}px`,
    `border-color:${borderColor}`,
    `background-color:${backgroundColor}`,
    `border-radius:${borderRadius}px`,
  ].join(';');
}

const NoteImage = Image.extend({
  addNodeView() {
    return ReactNodeViewRenderer(RichTextImageNodeView);
  },

  addAttributes() {
    return {
      ...this.parent?.(),
      src: { default: null },
      alt: { default: null },
      title: { default: null },
      'data-note-src': { default: null },
      align: {
        default: 'center',
        parseHTML: (element) => element.getAttribute('data-align') || 'center',
        renderHTML: (attributes) => ({
          'data-align': attributes.align || 'center',
        }),
      },
      widthPercent: {
        default: '100',
        parseHTML: (element) =>
          clampImageWidth(
            element.getAttribute('data-width') || element.style.width?.replace('%', '') || '100'
          ),
        renderHTML: (attributes) => ({
          'data-width': clampImageWidth(attributes.widthPercent || '100'),
          style: buildImageStyle(attributes),
        }),
      },
      borderStyle: {
        default: 'none',
        parseHTML: (element) =>
          element.style.borderStyle || element.getAttribute('data-border-style') || 'none',
        renderHTML: (attributes) => ({
          'data-border-style': attributes.borderStyle || 'none',
        }),
      },
      borderColor: {
        default: '#cbd5e1',
        parseHTML: (element) =>
          element.style.borderColor || element.getAttribute('data-border-color') || '#cbd5e1',
        renderHTML: (attributes) => ({
          'data-border-color': attributes.borderColor || '#cbd5e1',
        }),
      },
      borderWidth: {
        default: '0',
        parseHTML: (element) =>
          normalizePixels(element.style.borderWidth || element.getAttribute('data-border-width'), '0'),
        renderHTML: (attributes) => ({
          'data-border-width': normalizePixels(attributes.borderWidth, '0'),
        }),
      },
      backgroundColor: {
        default: 'transparent',
        parseHTML: (element) =>
          element.style.backgroundColor ||
          element.getAttribute('data-background-color') ||
          'transparent',
        renderHTML: (attributes) => ({
          'data-background-color': attributes.backgroundColor || 'transparent',
        }),
      },
      borderRadius: {
        default: '18',
        parseHTML: (element) =>
          normalizePixels(
            element.style.borderRadius || element.getAttribute('data-border-radius'),
            '18'
          ),
        renderHTML: (attributes) => ({
          'data-border-radius': normalizePixels(attributes.borderRadius, '18'),
        }),
      },
    };
  },
});

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

export default function RichTextEditor({
  historyOpen = false,
  onClearHistory,
  filePath,
  onHistoryOpenChange,
  html,
  onHtmlChange,
  onDeleteVersion,
  onSelectionChange,
  onStatusMessage,
  onRestoreVersion,
  clearSelectionVersion = 0,
  pinnedSelections = [],
}) {
  const imageInputRef = React.useRef(null);
  const isPointerSelectingRef = React.useRef(false);
  const isBlockToolbarInteractionRef = React.useRef(false);
  const lastCommittedHtmlRef = React.useRef(html || '');
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

  const buildSelectionContext = React.useCallback(
    (nextEditor, range) => {
      if (!filePath || !nextEditor || !range || range.from >= range.to) return null;
      const selectedText = nextEditor.state.doc.textBetween(range.from, range.to, '\n\n', '\n').trim();
      if (!selectedText) return null;
      const contextRadius = 160;
      const beforeStart = Math.max(0, range.from - contextRadius);
      const afterEnd = Math.min(nextEditor.state.doc.content.size, range.to + contextRadius);
      return {
        id: 'text_selection',
        type: 'text_selection',
        kind: 'text_selection',
        path: filePath,
        file_type: 'text',
        selected_text: selectedText,
        text_before: nextEditor.state.doc.textBetween(beforeStart, range.from, '\n\n', '\n').trim(),
        text_after: nextEditor.state.doc.textBetween(range.to, afterEnd, '\n\n', '\n').trim(),
        start_offset: range.from,
        end_offset: range.to,
      };
    },
    [filePath]
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
      if (!nextEditor || historyOpen) {
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

      const triggerLeft = Math.max(editorRect.left + 22, 20);
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
    [historyOpen]
  );

  const editor = useEditor({
    content: decorateHtmlWithAssetUrls(filePath, html || ''),
    immediatelyRender: false,
    shouldRerenderOnTransaction: false,
    extensions: [
      StarterKit,
      Underline,
      Highlight.configure({ multicolor: true }),
      TextStyle,
      FontSize,
      BlockSpacing,
      PinnedSelection,
      Color,
      TaskList,
      TaskItem.configure({ nested: true }),
      NoteTable.configure({ resizable: false }),
      TableRow,
      TableHeader,
      TableCell,
      TextAlign.configure({ types: ['heading', 'paragraph', 'blockquote'] }),
      Link.configure({ openOnClick: false }),
      NoteImage,
      Placeholder.configure({
        placeholder: '在这里写下你的笔记，或让 AI 接着整理它。',
      }),
    ],
    editorProps: {
      attributes: {
        class:
          'rich-text-editor-content custom-scrollbar h-full overflow-y-auto px-16 py-8 pb-32 outline-none max-[900px]:px-10 max-[640px]:px-6',
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
    onUpdate({ editor: nextEditor }) {
      const nextHtml = getSanitizedEditorHtml(nextEditor);
      lastCommittedHtmlRef.current = nextHtml;
      onHtmlChange?.(nextHtml);
      if (selectionToolbarState.range) syncSelectionToolbar(nextEditor);
      syncTableToolbar(nextEditor);
      syncBlockToolbarTrigger(nextEditor);
    },
    onCreate({ editor: nextEditor }) {
      const normalizedHtml = getSanitizedEditorHtml(nextEditor);
      if (normalizedHtml !== lastCommittedHtmlRef.current) {
        lastCommittedHtmlRef.current = normalizedHtml;
        onHtmlChange?.(normalizedHtml);
      }
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

  const uploadImageFile = React.useCallback(
    async (file) => {
      if (!file || !editor || !filePath) return false;
      const uploadPath = buildNoteAssetUploadPath(filePath, file.name);
      const referencePath = getNoteAssetReferencePath(filePath, uploadPath);
      try {
        onStatusMessage?.('正在上传图片...');
        await apiUploadFile(uploadPath, file, false);
        editor
          .chain()
          .focus()
          .setImage({
            alt: file.name,
            src: workspaceFileContentUrl(uploadPath),
            'data-note-src': referencePath,
            align: 'center',
            widthPercent: '100',
          })
          .run();
        onStatusMessage?.('图片已插入笔记');
        return true;
      } catch (error) {
        console.error('insert image failed', error);
        onStatusMessage?.(`图片上传失败：${error.message}`);
        return false;
      }
    },
    [editor, filePath, onStatusMessage]
  );

  React.useEffect(() => {
    if (!editor) return;
    const nextHtml = html || '';
    if (nextHtml === lastCommittedHtmlRef.current) return;
    lastCommittedHtmlRef.current = nextHtml;
    editor.commands.setContent(decorateHtmlWithAssetUrls(filePath, nextHtml), false);
  }, [editor, filePath, html]);

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
  }, [clearSelectionVersion, editor]);

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
  }, [editor, hideSelectionToolbar, syncBlockToolbarTrigger, syncSelectionToolbar]);

  React.useEffect(() => {
    if (!historyOpen) return;
    lastSelectionToolbarPlacementRef.current = 'above';
    lastTableToolbarPlacementRef.current = 'above';
    setSelectionToolbarState(createHiddenFloatingToolbarState({ range: null }));
    setTableToolbarState(createHiddenFloatingToolbarState({ caption: '' }));
    setBlockToolbarState(createHiddenBlockToolbarState());
  }, [historyOpen]);

  const handlePinSelection = React.useCallback(() => {
    if (!editor || !selectionToolbarState.range) return;
    const currentSelections = Array.isArray(pinnedSelections) ? pinnedSelections : [];
    const alreadyPinned = currentSelections.some(
      (selection) =>
        selection.start_offset === selectionToolbarState.range.from &&
        selection.end_offset === selectionToolbarState.range.to
    );
    if (alreadyPinned) return;
    const context = buildSelectionContext(editor, selectionToolbarState.range);
    if (!context) return;
    editor.commands.addPinnedSelection(selectionToolbarState.range);
    onSelectionChange?.([...currentSelections, context]);
  }, [buildSelectionContext, editor, onSelectionChange, pinnedSelections, selectionToolbarState.range]);

  const handleInsertImage = async (event) => {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file) return;
    await uploadImageFile(file);
  };

  return (
    <div className="flex h-full min-h-0 flex-col bg-[#FAFAFA] text-gray-800 dark:bg-[#1E1E1E] dark:text-gray-200">
      <RichTextSelectionToolbar
        editor={editor}
        pinnedSelections={pinnedSelections}
        placement={selectionToolbarState.placement}
        position={selectionToolbarState.position}
        range={selectionToolbarState.range}
        visible={selectionToolbarState.visible && !historyOpen && !tableToolbarState.visible}
        onPinSelection={handlePinSelection}
        onRequestClose={hideSelectionToolbar}
      />
      <RichTextTableToolbar
        caption={tableToolbarState.caption}
        editor={editor}
        placement={tableToolbarState.placement}
        position={tableToolbarState.position}
        visible={tableToolbarState.visible && !historyOpen}
      />
      <RichTextToolbar
        editor={editor}
        floating
        pendingTableConfig={pendingTableConfig}
        position={blockToolbarState.toolbarPosition}
        visible={blockToolbarState.toolbarVisible && !historyOpen && !tableToolbarState.visible}
        onApplyTableConfig={(nextConfig) => {
          const rows = Math.min(12, Math.max(1, Number(nextConfig.rows) || 3));
          const cols = Math.min(8, Math.max(1, Number(nextConfig.cols) || 3));
          editor?.chain().focus().insertTable({ rows, cols, withHeaderRow: !!nextConfig.withHeaderRow }).run();
          setPendingTableConfig({
            ...nextConfig,
            open: false,
            rows: String(rows),
            cols: String(cols),
          });
        }}
        onInsertImage={() => imageInputRef.current?.click()}
        onInteractWithin={() => {
          isBlockToolbarInteractionRef.current = true;
        }}
        onRequestClose={hideBlockToolbar}
        onTableConfigChange={setPendingTableConfig}
      />
      <input
        ref={imageInputRef}
        type="file"
        accept="image/png,image/jpeg,image/webp,image/gif,image/svg+xml"
        className="hidden"
        onChange={handleInsertImage}
      />
      <div className="relative min-h-0 flex-1 overflow-hidden">
        {blockToolbarState.triggerVisible &&
          blockToolbarState.triggerPosition &&
          !historyOpen &&
          !tableToolbarState.visible &&
          typeof document !== 'undefined' &&
          createPortal(
            <button
              type="button"
              className={`rich-text-block-trigger fixed z-[1130] ${
                blockToolbarState.toolbarVisible ? 'is-active' : ''
              }`}
              style={{
                left: blockToolbarState.triggerPosition.left,
                top: blockToolbarState.triggerPosition.top,
              }}
              onMouseDown={(event) => {
                isBlockToolbarInteractionRef.current = true;
                event.preventDefault();
              }}
              onClick={() =>
                setBlockToolbarState((current) => ({
                  ...current,
                  toolbarVisible: !current.toolbarVisible,
                  toolbarPosition: current.triggerPosition
                    ? {
                        left: current.triggerPosition.left - 2,
                        top: current.triggerPosition.top + 28,
                      }
                    : null,
                }))
              }
              aria-label="打开行内工具栏"
              title="打开行内工具栏"
            >
              {Array.from({ length: 6 }).map((_, index) => (
                <span key={`dot-${index}`} className="rich-text-block-trigger-dot" />
              ))}
            </button>,
            document.body
          )}
        <EditorContent editor={editor} className="rich-text-editor-host h-full min-h-0" />
        <RichTextHistoryPanel
          currentHtml={html}
          filePath={filePath}
          open={historyOpen}
          onClearHistory={onClearHistory}
          onClose={() => onHistoryOpenChange?.(false)}
          onDeleteVersion={onDeleteVersion}
          onRestoreVersion={onRestoreVersion}
        />
      </div>
    </div>
  );
}
