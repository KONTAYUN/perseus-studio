import React from 'react';
import { createPortal } from 'react-dom';
import * as RadixToolbar from '@radix-ui/react-toolbar';
import {
  BlockTypeSelect,
  BoldItalicUnderlineToggles,
  ButtonWithTooltip,
  CreateLink,
  EMPTY_VALUE,
  getCodeBlockLanguageSelectData,
  normalizeCodeBlockLanguages,
  InsertCodeBlock,
  InsertImage,
  InsertTable,
  ListsToggle,
  Select,
  Separator,
  activeEditor$,
  addComposerChild$,
  editorRootElementRef$,
  editorWrapperElementRef$,
  editorInFocus$,
  readOnly$,
  realmPlugin,
  currentSelection$,
} from '@mdxeditor/editor';
import { useCellValues, usePublisher } from '@mdxeditor/gurx';
import { $getNodeByKey, $getRoot, getNearestEditorFromDOMNode } from 'lexical';
import { Check, Copy, Trash2 } from 'lucide-react';
import { CODE_BLOCK_LANGUAGES } from './markdownLanguages.js';

const NORMALIZED_CODE_BLOCK_LANGUAGES = normalizeCodeBlockLanguages(CODE_BLOCK_LANGUAGES);

function getElementFromNode(node) {
  if (!node) return null;
  return node.nodeType === 1 ? node : node.parentElement;
}

function findBlockElement(range, rootElement) {
  let element = getElementFromNode(range?.startContainer);
  while (element && element !== rootElement) {
    if (element.parentElement === rootElement) return element;
    element = element.parentElement;
  }
  return null;
}

function clamp(value, min, max) {
  return Math.min(Math.max(value, min), max);
}

function isSameGeometry(left, right) {
  if (!left || !right) return left === right;
  return left.hasSelection === right.hasSelection
    && left.isCodeBlock === right.isCodeBlock
    && left.isListItem === right.isListItem
    && left.selection.left === right.selection.left
    && left.selection.top === right.selection.top
    && left.selection.placement === right.selection.placement
    && left.block.left === right.block.left
    && left.block.top === right.block.top
    && left.blockToolbar.left === right.blockToolbar.left
    && left.blockToolbar.top === right.blockToolbar.top
    && left.blockToolbar.placement === right.blockToolbar.placement
    && left.codeToolbar.left === right.codeToolbar.left
    && left.codeToolbar.top === right.codeToolbar.top
    && left.codeToolbar.placement === right.codeToolbar.placement;
}

function getEditorGeometry(...editors) {
  if (typeof window === 'undefined') return null;
  const nativeSelection = window.getSelection?.();
  if (!nativeSelection || nativeSelection.rangeCount === 0) return null;

  const anchorNode = nativeSelection.anchorNode;
  if (!anchorNode) return null;
  const anchorElement = getElementFromNode(anchorNode);
  const domRoot = [
    anchorElement?.closest?.('.cm-content'),
    anchorElement?.closest?.('[contenteditable="true"]'),
    anchorElement?.closest?.('.workspace-markdown-editor-content'),
  ].find(Boolean);
  const editorRoots = editors
    .filter(Boolean)
    .map((editor) => editor.getRootElement?.())
    .filter(Boolean);
  const rootElement = domRoot || editorRoots.find((root) => root.contains(anchorNode));
  if (!rootElement) return null;

  const range = nativeSelection.getRangeAt(0);
  const rangeRect = range.getBoundingClientRect();
  const blockElement = findBlockElement(range, rootElement);
  // The native selection can sit inside a Lexical text span while the direct
  // block anchor is the list container. Resolve both sides of the boundary so
  // list context is a property of the document structure, not of the current
  // DOM text node.
  const listItemElement = blockElement.closest?.('li') || anchorElement?.closest?.('li');
  if (!blockElement) return null;
  const blockRect = blockElement.getBoundingClientRect();
  const viewportElement = anchorElement?.closest?.('.workspace-markdown-editor-scroll')
    || rootElement.closest?.('.workspace-markdown-editor-scroll')
    || rootElement;
  const viewportRect = viewportElement.getBoundingClientRect();
  const viewportPadding = 8;
  const viewportLeft = viewportRect.left + viewportPadding;
  const viewportRight = Math.max(viewportLeft, viewportRect.right - viewportPadding);
  const selectionToolbarWidth = Math.min(236, Math.max(180, viewportRect.width - viewportPadding * 2));
  const blockToolbarWidth = Math.min(360, Math.max(220, viewportRect.width - viewportPadding * 2));
  const selectionMin = viewportLeft + selectionToolbarWidth / 2;
  const selectionMax = Math.max(selectionMin, viewportRight - selectionToolbarWidth / 2);
  const blockToolbarMin = viewportLeft;
  const blockToolbarMax = Math.max(blockToolbarMin, viewportRight - blockToolbarWidth);
  const codeEditor = anchorElement?.closest?.('.cm-editor');
  const codeWrapper = codeEditor?.closest?.('[class*="codeMirrorWrapper"]');
  const codeBlockRect = codeWrapper?.getBoundingClientRect?.() || blockRect;
  const hasSelection = !nativeSelection.isCollapsed && (rangeRect.width > 0 || rangeRect.height > 0);
  const selectionCenter = (rangeRect.left + rangeRect.right) / 2;
  const selectionPlacement = rangeRect.top < 72 ? 'below' : 'above';
  const isCodeBlock = Boolean(codeEditor);
  const isListItem = Boolean(listItemElement && rootElement.contains(listItemElement));
  const blockToolbarHeight = 46;
  const blockToolbarGap = 10;
  const canPlaceBelow = blockRect.bottom + blockToolbarGap + blockToolbarHeight <= window.innerHeight - 8;
  const canPlaceAbove = blockRect.top - blockToolbarGap - blockToolbarHeight >= 8;
  const blockToolbarPlacement = canPlaceBelow || !canPlaceAbove ? 'below' : 'above';
  const blockTriggerTop = blockRect.top + clamp((blockRect.height - 24) / 2, 0, 12);
  const blockToolbarTop = blockToolbarPlacement === 'below'
    ? blockRect.bottom + blockToolbarGap
    : blockRect.top - blockToolbarGap - blockToolbarHeight;
  const codeToolbarWidth = Math.min(236, Math.max(190, viewportRect.width - viewportPadding * 2));
  const codeToolbarHeight = 40;
  const codeToolbarGap = 8;
  const codeToolbarViewportTop = viewportRect.top + viewportPadding;
  const codeToolbarViewportBottom = viewportRect.bottom - viewportPadding;
  const canPlaceCodeToolbarAbove = codeBlockRect.top - codeToolbarGap - codeToolbarHeight >= codeToolbarViewportTop;
  const codeToolbarPlacement = canPlaceCodeToolbarAbove ? 'above' : 'below';
  const codeToolbarTop = codeToolbarPlacement === 'above'
    ? codeBlockRect.top - codeToolbarGap - codeToolbarHeight
    : codeBlockRect.bottom + codeToolbarGap;
  const codeToolbarMin = viewportLeft;
  const codeToolbarMax = Math.max(codeToolbarMin, viewportRight - codeToolbarWidth);

  return {
    hasSelection,
    isCodeBlock,
    isListItem,
    selection: {
      left: clamp(selectionCenter, selectionMin, selectionMax),
      top: selectionPlacement === 'below' ? rangeRect.bottom + 10 : rangeRect.top - 10,
      placement: selectionPlacement,
    },
    block: {
      left: clamp(blockRect.left - 34, viewportLeft, viewportRight - 24),
      top: clamp(blockTriggerTop, 8, window.innerHeight - 34),
    },
    blockToolbar: {
      // Keep the contextual surface inside the editor viewport. The previous
      // window-wide clamp let it cross into the adjacent chat panel, where
      // the panel resizer legitimately won pointer hit-testing.
      left: clamp(blockRect.left - 6, blockToolbarMin, blockToolbarMax),
      top: clamp(blockToolbarTop, 8, window.innerHeight - blockToolbarHeight - 8),
      placement: blockToolbarPlacement,
    },
    codeToolbar: {
      left: clamp(codeBlockRect.right - codeToolbarWidth, codeToolbarMin, codeToolbarMax),
      top: clamp(codeToolbarTop, codeToolbarViewportTop, Math.max(codeToolbarViewportTop, codeToolbarViewportBottom - codeToolbarHeight)),
      placement: codeToolbarPlacement,
    },
  };
}

function FloatingSurface({ children, className = '', testId }) {
  return (
    <div
      className={`workspace-markdown-floating-surface motion-popover ${className}`.trim()}
      data-testid={testId}
    >
      {children}
    </div>
  );
}

function ContextualToolbarRoot({ children, label }) {
  return (
    <RadixToolbar.Root
      className="workspace-markdown-radix-toolbar"
      aria-label={label}
      onMouseDownCapture={(event) => {
        // Keep Lexical's native selection alive while a contextual command is
        // clicked. Select triggers are the one exception: Radix needs their
        // pointer event to open the listbox.
        const control = event.target.closest?.('[data-toolbar-item="true"], button, [role="button"]');
        if (!control) return;
        if (control.getAttribute('role') === 'combobox' || control.getAttribute('aria-haspopup') === 'listbox') return;
        event.preventDefault();
      }}
    >
      {children}
    </RadixToolbar.Root>
  );
}

function CodeBlockCopyButton({ editorInFocus }) {
  const [copied, setCopied] = React.useState(false);
  const resetTimerRef = React.useRef(null);

  React.useEffect(() => () => {
    if (resetTimerRef.current) window.clearTimeout(resetTimerRef.current);
  }, []);

  React.useEffect(() => {
    setCopied(false);
  }, [editorInFocus?.rootNode]);

  const handleCopy = React.useCallback(async () => {
    const view = editorInFocus?.editorRef?.getCodemirror?.();
    const code = view?.state?.doc?.toString?.() ?? editorInFocus?.rootNode?.getCode?.();
    if (typeof code !== 'string' || !navigator.clipboard?.writeText) return;
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      if (resetTimerRef.current) window.clearTimeout(resetTimerRef.current);
      resetTimerRef.current = window.setTimeout(() => {
        resetTimerRef.current = null;
        setCopied(false);
      }, 1400);
    } catch {
      setCopied(false);
    }
  }, [editorInFocus]);

  return (
    <ButtonWithTooltip
      type="button"
      title={copied ? '\u5df2\u590d\u5236' : '\u590d\u5236\u4ee3\u7801'}
      aria-label={copied ? '\u5df2\u590d\u5236' : '\u590d\u5236\u4ee3\u7801'}
      data-testid="markdown-code-copy"
      onMouseDown={(event) => event.preventDefault()}
      onClick={handleCopy}
    >
      {copied ? <Check size={15} aria-hidden="true" /> : <Copy size={15} aria-hidden="true" />}
    </ButtonWithTooltip>
  );
}

function MarkdownCodeLanguageSelect({ activeEditor, editorInFocus }) {
  const codeBlockNode = editorInFocus?.rootNode;
  const nodeKey = codeBlockNode?.getKey?.();
  const [rawLanguage, setRawLanguage] = React.useState('');

  React.useEffect(() => {
    if (!activeEditor || !nodeKey) {
      setRawLanguage('');
      return undefined;
    }
    const readLanguage = (editorState) => {
      editorState.read(() => {
        const currentNode = $getNodeByKey(nodeKey);
        setRawLanguage(currentNode?.getLanguage?.() || '');
      });
    };
    readLanguage(activeEditor.getEditorState());
    return activeEditor.registerUpdateListener(({ editorState }) => readLanguage(editorState));
  }, [activeEditor, nodeKey]);

  if (!activeEditor || !codeBlockNode || !nodeKey) return null;

  const { value, items } = getCodeBlockLanguageSelectData(NORMALIZED_CODE_BLOCK_LANGUAGES, rawLanguage);
  const handleChange = (language) => {
    activeEditor.update(() => {
      const currentNode = $getNodeByKey(nodeKey);
      if (!currentNode || currentNode.getType?.() !== 'codeblock') return;
      currentNode.setLanguage(language === EMPTY_VALUE ? '' : language);
    });
  };

  return (
    <Select
      value={value || EMPTY_VALUE}
      onChange={handleChange}
      triggerTitle="选择代码语言"
      placeholder="代码语言"
      items={items}
    />
  );
}

/**
 * MDXEditor attaches its CodeMirror focus listener asynchronously. A code
 * editor recreated by setMarkdown can therefore receive the user's first
 * focus event before editorInFocus$ is updated. The integration owns this
 * bridge so contextual tools follow the actual Lexical decorator node rather
 * than a timing window in the nested editor implementation.
 */
function CodeMirrorFocusBridge({ editorRootElementRef, activeEditor }) {
  const activeEditorRef = React.useRef(activeEditor);
  activeEditorRef.current = activeEditor;
  const publishActiveEditor = usePublisher(activeEditor$);
  const publishEditorInFocus = usePublisher(editorInFocus$);

  const findFocusedCodeBlock = React.useCallback(
    (domElement, editor) => {
      let focusedNode = null;
      const visit = (node) => {
        if (!node || focusedNode) return;
        if (node.getType?.() === 'codeblock') {
          const decoratorElement = editor.getElementByKey(node.getKey());
          if (decoratorElement?.contains(domElement)) {
            focusedNode = node;
            return;
          }
        }
        node.getChildren?.().forEach(visit);
      };

      editor.getEditorState().read(() => visit($getRoot()));
      return focusedNode;
    },
    []
  );

  React.useEffect(() => {
    const handleFocusIn = (event) => {
      const target = event.target;
      const rootElement = editorRootElementRef?.current;
      if (!rootElement || !target?.closest) return;

      const codeContent = target.closest('.cm-content');
      if (!codeContent) return;

      const editor = activeEditorRef.current || getNearestEditorFromDOMNode(codeContent);
      if (!editor) return;
      const editorDomRoot = editor.getRootElement?.();
      if (!editorDomRoot?.contains(codeContent)) return;
      const codeBlockNode = findFocusedCodeBlock(codeContent, editor);

      if (!codeBlockNode || codeBlockNode.getType?.() !== 'codeblock') return;

      publishActiveEditor(editor);
      publishEditorInFocus({
        editorType: 'codeblock',
        rootNode: codeBlockNode,
        editorRef: {
          getCodemirror: () => null,
        },
      });
    };

    document.addEventListener('focusin', handleFocusIn, true);
    return () => document.removeEventListener('focusin', handleFocusIn, true);
  }, [editorRootElementRef, findFocusedCodeBlock, publishActiveEditor, publishEditorInFocus]);

  return null;
}

function CodeBlockDeleteButton({ activeEditor, editorInFocus }) {
  const handleDelete = React.useCallback((event) => {
    event.preventDefault();
    const node = editorInFocus?.rootNode;
    if (!activeEditor || !node) return;
    activeEditor.update(() => {
      node.getLatest?.().remove?.();
    });
  }, [activeEditor, editorInFocus]);

  return (
    <ButtonWithTooltip
      type="button"
      title="\u5220\u9664\u4ee3\u7801\u5757"
      aria-label="\u5220\u9664\u4ee3\u7801\u5757"
      data-testid="markdown-code-delete"
      onMouseDown={(event) => event.preventDefault()}
      onClick={handleDelete}
    >
      <Trash2 size={15} aria-hidden="true" />
    </ButtonWithTooltip>
  );
}

function getEditorTooltipTarget(target, roots) {
  const button = target?.closest?.('button');
  if (!button || !roots.some((root) => root?.contains?.(button))) return null;
  const isTableControl = Boolean(button.closest('table [data-tool-cell="true"]'));
  const isTablePopoverControl = Boolean(button.closest('[role="dialog"], [data-radix-popper-content-wrapper], [data-state="open"]'));
  if (!isTableControl && !isTablePopoverControl) return null;
  const title = button.dataset.markdownTooltip || button.getAttribute('title') || '';
  if (!title.trim()) return null;
  return { button, title: title.trim() };
}

/**
 * The table plugin exposes native `title` attributes instead of MDXEditor's
 * TooltipWrap primitive. Convert that public DOM contract once at the editor
 * boundary so table controls use the same accessible, themeable tooltip as
 * the rest of the contextual UI. This avoids changing plugin internals or
 * relying on browser-specific native tooltip rendering.
 */
function MarkdownEditorTooltipLayer({ observeRoot, renderHost }) {
  const [tooltip, setTooltip] = React.useState(null);
  const tooltipTargetRef = React.useRef(null);

  React.useLayoutEffect(() => {
    const roots = [observeRoot, renderHost].filter(Boolean).filter((root, index, list) => list.indexOf(root) === index);
    if (roots.length === 0) return undefined;

    const normalizeTargets = () => {
      roots.forEach((root) => {
        const targets = root.querySelectorAll(
          'table [data-tool-cell="true"] button[title], [role="dialog"] button[title], [data-radix-popper-content-wrapper] button[title]'
        );
        targets.forEach((button) => {
          const title = button.getAttribute('title')?.trim();
          if (!title || button.dataset.markdownTooltip) return;
          button.dataset.markdownTooltip = title;
          if (!button.getAttribute('aria-label')) button.setAttribute('aria-label', title);
          button.removeAttribute('title');
        });
      });
    };

    normalizeTargets();
    const observer = new MutationObserver(normalizeTargets);
    roots.forEach((root) => observer.observe(root, {
      childList: true,
      subtree: true,
      attributes: true,
      attributeFilter: ['title'],
    }));

    const showForEvent = (event) => {
      const target = getEditorTooltipTarget(event.target, roots);
      if (!target) return;
      if (!target.button.dataset.markdownTooltip) {
        target.button.dataset.markdownTooltip = target.title;
        if (!target.button.getAttribute('aria-label')) target.button.setAttribute('aria-label', target.title);
        target.button.removeAttribute('title');
      }
      const rect = target.button.getBoundingClientRect();
      tooltipTargetRef.current = target.button;
      setTooltip({ label: target.title, left: rect.left + rect.width / 2, top: rect.top });
    };
    const hideForEvent = (event) => {
      const relatedTarget = event.relatedTarget;
      const currentTarget = event.target?.closest?.('button');
      if (relatedTarget && currentTarget?.contains?.(relatedTarget)) return;
      tooltipTargetRef.current = null;
      setTooltip(null);
    };
    const refreshPosition = () => {
      setTooltip((current) => {
        if (!current) return current;
        const target = tooltipTargetRef.current;
        if (!target || !roots.some((root) => root.contains(target))) {
          tooltipTargetRef.current = null;
          return null;
        }
        const rect = target.getBoundingClientRect();
        return { ...current, left: rect.left + rect.width / 2, top: rect.top };
      });
    };

    roots.forEach((root) => {
      root.addEventListener('pointerover', showForEvent, true);
      root.addEventListener('focusin', showForEvent, true);
      root.addEventListener('pointerout', hideForEvent, true);
      root.addEventListener('focusout', hideForEvent, true);
    });
    window.addEventListener('resize', refreshPosition);
    document.addEventListener('scroll', refreshPosition, true);
    return () => {
      observer.disconnect();
      roots.forEach((root) => {
        root.removeEventListener('pointerover', showForEvent, true);
        root.removeEventListener('focusin', showForEvent, true);
        root.removeEventListener('pointerout', hideForEvent, true);
        root.removeEventListener('focusout', hideForEvent, true);
      });
      window.removeEventListener('resize', refreshPosition);
      document.removeEventListener('scroll', refreshPosition, true);
      tooltipTargetRef.current = null;
    };
  }, [observeRoot, renderHost]);

  if (!tooltip || !renderHost) return null;
  const hostRect = renderHost.getBoundingClientRect();
  return (
    <div
      className="workspace-markdown-editor-tooltip"
      role="tooltip"
      style={{
        left: tooltip.left - hostRect.left,
        top: tooltip.top - hostRect.top - 8,
      }}
    >
      {tooltip.label}
    </div>
  );
}

function MarkdownContextualTools() {
  const [activeEditor, currentSelection, editorInFocus, readOnly, editorRootElementRef, editorWrapperElementRef] = useCellValues(
    activeEditor$,
    currentSelection$,
    editorInFocus$,
    readOnly$,
    editorRootElementRef$,
    editorWrapperElementRef$
  );
  const [geometry, setGeometry] = React.useState(null);
  const [blockMenuOpen, setBlockMenuOpen] = React.useState(false);
  const blockMenuRef = React.useRef(null);
  const blockTriggerRef = React.useRef(null);
  const selectionToolbarRef = React.useRef(null);
  const codeToolbarRef = React.useRef(null);
  const geometryRef = React.useRef(null);
  const lastCodeContextRef = React.useRef(null);
  const lastCodeEditorRef = React.useRef(null);
  const codeInteractionRef = React.useRef(false);
  const blockMenuOpenRef = React.useRef(false);
  blockMenuOpenRef.current = blockMenuOpen;

  if (editorInFocus?.editorType === 'codeblock') {
    lastCodeContextRef.current = editorInFocus;
    lastCodeEditorRef.current = activeEditor;
  }

  const isToolbarInteraction = React.useCallback((target) => {
    if (!target || typeof target.closest !== 'function') return false;
    if (blockMenuRef.current?.contains(target)) return true;
    if (blockTriggerRef.current?.contains(target)) return true;
    if (selectionToolbarRef.current?.contains(target)) return true;
    if (codeToolbarRef.current?.contains(target)) return true;
    if (editorRootElementRef?.current?.contains(target)) return true;
    return false;
  }, [editorRootElementRef]);

  const hasOpenEditorPopup = React.useCallback(
    () => Boolean(editorRootElementRef?.current?.querySelector?.('[data-state="open"], [role="dialog"], [role="listbox"]')),
    [editorRootElementRef]
  );

  React.useLayoutEffect(() => {
    const updateGeometry = (event) => {
      if (isToolbarInteraction(event?.target)) return;
      const next = getEditorGeometry(activeEditor, editorInFocus);
      if (next) {
        if (!isSameGeometry(geometryRef.current, next)) {
          geometryRef.current = next;
          setGeometry(next);
        }
        return;
      }

      // Radix controls move focus into the editor's popup container. Keep the
      // last valid anchor while that interaction is active instead of treating
      // the temporary native-selection change as an editor reset.
      const preserveCodeSurface = codeInteractionRef.current
        && geometryRef.current?.isCodeBlock;
      if (blockMenuOpenRef.current || isToolbarInteraction(document.activeElement) || hasOpenEditorPopup() || preserveCodeSurface) return;
      geometryRef.current = null;
      setGeometry(null);
      if (blockMenuOpenRef.current) setBlockMenuOpen(false);
    };

    updateGeometry();
    window.addEventListener('resize', updateGeometry);
    document.addEventListener('scroll', updateGeometry, true);
    document.addEventListener('selectionchange', updateGeometry);
    document.addEventListener('focusin', updateGeometry);
    document.addEventListener('pointerup', updateGeometry);
    const rememberCodePointer = (event) => {
      const target = event.target;
      if (target?.closest?.('.cm-content')) {
        codeInteractionRef.current = true;
        return;
      }
      if (codeToolbarRef.current?.contains(target)) {
        codeInteractionRef.current = true;
        return;
      }
      const codePopup = target?.closest?.(
        '[role="listbox"], [data-editor-dropdown="true"], [data-radix-popper-content-wrapper]'
      );
      if (codePopup && geometryRef.current?.isCodeBlock) {
        codeInteractionRef.current = true;
        return;
      }
      if (isToolbarInteraction(target)) {
        codeInteractionRef.current = false;
        return;
      }
      codeInteractionRef.current = false;
    };
    document.addEventListener('pointerdown', rememberCodePointer, true);
    return () => {
      window.removeEventListener('resize', updateGeometry);
      document.removeEventListener('scroll', updateGeometry, true);
      document.removeEventListener('selectionchange', updateGeometry);
      document.removeEventListener('focusin', updateGeometry);
      document.removeEventListener('pointerup', updateGeometry);
      document.removeEventListener('pointerdown', rememberCodePointer, true);
    };
  }, [activeEditor, editorInFocus, hasOpenEditorPopup, isToolbarInteraction]);

  React.useEffect(() => {
    if (!blockMenuOpen) return undefined;
    const closeOnOutsidePointer = (event) => {
      if (isToolbarInteraction(event.target)) return;
      setBlockMenuOpen(false);
    };
    const closeOnEscape = (event) => {
      if (event.key === 'Escape') setBlockMenuOpen(false);
    };
    document.addEventListener('pointerdown', closeOnOutsidePointer);
    document.addEventListener('keydown', closeOnEscape);
    return () => {
      document.removeEventListener('pointerdown', closeOnOutsidePointer);
      document.removeEventListener('keydown', closeOnEscape);
    };
  }, [blockMenuOpen, isToolbarInteraction]);

  const popupHost = editorRootElementRef?.current;
  const focusBridge = (
    <CodeMirrorFocusBridge
      editorRootElementRef={editorRootElementRef}
      activeEditor={activeEditor}
    />
  );
  if (typeof document === 'undefined' || !popupHost) return focusBridge;

  const editorContentHost = editorWrapperElementRef?.current;
  const tooltipLayer = (
    <MarkdownEditorTooltipLayer
      observeRoot={editorContentHost}
      renderHost={popupHost}
    />
  );
  if (readOnly || !geometry) {
    return createPortal(
      <>{focusBridge}{tooltipLayer}</>,
      popupHost
    );
  }

  // MDXEditor owns a fixed overlay host. The contextual surfaces are children
  // of that host, so their viewport geometry must be converted to the host's
  // local coordinate system before rendering. Mixing viewport `fixed` values
  // with this nested containing block shifts the toolbar into the next panel.
  const overlayRect = popupHost.getBoundingClientRect();
  const toOverlayPoint = ({ left, top }) => ({
    left: left - overlayRect.left,
    top: top - overlayRect.top,
  });
  const selectionPosition = toOverlayPoint(geometry.selection);
  const blockPosition = toOverlayPoint(geometry.block);
  const blockToolbarPosition = toOverlayPoint(geometry.blockToolbar);
  const codeToolbarPosition = toOverlayPoint(geometry.codeToolbar);

  // A nested CodeMirror editor legitimately blurs when its language menu takes
  // focus. The code block node remains the authoritative context until the
  // geometry moves to another block, so a popup cannot unmount its own owner.
  const codeContext = editorInFocus?.editorType === 'codeblock'
    ? editorInFocus
    : lastCodeContextRef.current;
  const codeEditor = activeEditor || lastCodeEditorRef.current;
  // The DOM block is the stable identity for showing the contextual surface.
  // `editorInFocus$` is intentionally transient and is cleared by MDXEditor
  // when a nested editor hands focus to a popup. It must not control whether
  // the popup's owner exists.
  const isCodeBlock = Boolean(geometry.isCodeBlock);
  const selectionToolbar = geometry.hasSelection && !isCodeBlock ? (
    <div
      ref={selectionToolbarRef}
      className="workspace-markdown-selection-toolbar"
      style={{
        left: selectionPosition.left,
        top: selectionPosition.top,
        transform: geometry.selection.placement === 'below' ? 'translate(-50%, 0)' : 'translate(-50%, -100%)',
      }}
    >
      <FloatingSurface testId="markdown-selection-toolbar">
        <ContextualToolbarRoot label="文字格式">
          <div className="workspace-markdown-toolbar-content">
            <BoldItalicUnderlineToggles />
            <Separator />
            <CreateLink />
          </div>
        </ContextualToolbarRoot>
      </FloatingSurface>
    </div>
  ) : null;

  const blockTrigger = !isCodeBlock ? (
    <button
      type="button"
      className={`motion-button workspace-markdown-block-trigger is-current ${blockMenuOpen ? 'is-active' : ''}`}
      style={{ left: blockPosition.left, top: blockPosition.top }}
      aria-label="打开当前段落工具"
      title="当前段落工具"
      data-testid="markdown-block-trigger"
      ref={blockTriggerRef}
      onMouseDown={(event) => event.preventDefault()}
      onClick={() => setBlockMenuOpen((current) => !current)}
    >
      {Array.from({ length: 6 }).map((_, index) => (
        <span key={`markdown-dot-${index}`} aria-hidden="true" />
      ))}
    </button>
  ) : null;

  const blockToolbar = blockMenuOpen && !isCodeBlock ? (
    <div
      ref={blockMenuRef}
      className={`workspace-markdown-block-toolbar ${geometry.isListItem ? 'is-list-context' : ''}`}
      data-list-context={geometry.isListItem ? 'true' : 'false'}
      data-placement={geometry.blockToolbar.placement}
      style={{ left: blockToolbarPosition.left, top: blockToolbarPosition.top }}
    >
      <FloatingSurface testId="markdown-block-toolbar">
        <ContextualToolbarRoot label="块级工具">
          <div className="workspace-markdown-toolbar-content workspace-markdown-block-toolbar-content">
            {!geometry.isListItem && <BlockTypeSelect />}
            <ListsToggle />
            <Separator />
            <InsertImage />
            <InsertTable />
            <InsertCodeBlock />
          </div>
        </ContextualToolbarRoot>
      </FloatingSurface>
    </div>
  ) : null;

  const codeToolbar = isCodeBlock ? (
    <div
      ref={codeToolbarRef}
      className="workspace-markdown-code-toolbar"
      data-placement={geometry.codeToolbar.placement}
      style={{ left: codeToolbarPosition.left, top: codeToolbarPosition.top }}
      data-testid="markdown-code-toolbar"
    >
      <FloatingSurface>
        <ContextualToolbarRoot label="代码块工具">
          <div className="workspace-markdown-toolbar-content workspace-markdown-code-toolbar-content">
            <MarkdownCodeLanguageSelect
              activeEditor={codeEditor}
              editorInFocus={codeContext}
            />
            <Separator />
            <CodeBlockCopyButton editorInFocus={codeContext} />
            <CodeBlockDeleteButton activeEditor={codeEditor} editorInFocus={codeContext} />
          </div>
        </ContextualToolbarRoot>
      </FloatingSurface>
    </div>
  ) : null;

  return createPortal(
    <>
      {focusBridge}
      {selectionToolbar}
      {blockTrigger}
      {blockToolbar}
      {codeToolbar}
      {tooltipLayer}
    </>,
    popupHost
  );
}

export const markdownContextualToolsPlugin = realmPlugin({
  init(realm) {
    realm.pub(addComposerChild$, MarkdownContextualTools);
  },
});
