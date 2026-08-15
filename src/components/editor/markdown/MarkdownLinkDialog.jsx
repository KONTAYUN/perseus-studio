import React from 'react';
import { createPortal } from 'react-dom';
import {
  cancelLinkEdit$,
  linkDialogState$,
  removeLink$,
  showLinkTitleField$,
  switchFromPreviewToLinkEdit$,
  updateLink$,
  editorRootElementRef$,
} from '@mdxeditor/editor';
import { useCellValues, usePublisher } from '@mdxeditor/gurx';

function getDialogPosition(rectangle, overlayRect, viewportRect) {
  if (typeof window === 'undefined' || !rectangle) return null;

  const width = 340;
  const horizontalPadding = 12;
  const viewportLeft = Math.max(horizontalPadding, viewportRect?.left + horizontalPadding || horizontalPadding);
  const viewportRight = Math.min(
    window.innerWidth - horizontalPadding,
    viewportRect?.right - horizontalPadding || window.innerWidth - horizontalPadding
  );
  const left = Math.min(
    Math.max(viewportLeft, rectangle.left),
    Math.max(viewportLeft, viewportRight - width)
  );
  const belowTop = rectangle.top + rectangle.height + 10;
  const aboveTop = rectangle.top - 10;
  const viewportTop = Math.max(horizontalPadding, viewportRect?.top + horizontalPadding || horizontalPadding);
  const viewportBottom = Math.min(
    window.innerHeight - horizontalPadding,
    viewportRect?.bottom - horizontalPadding || window.innerHeight - horizontalPadding
  );
  const fitsBelow = belowTop + 190 <= viewportBottom;

  return {
    left: left - overlayRect.left,
    top: Math.max(viewportTop, fitsBelow ? belowTop : aboveTop) - overlayRect.top,
    transform: fitsBelow ? 'none' : 'translateY(-100%)',
  };
}

function LinkDialogSurface({ children, style, testId }) {
  return (
    <div
      className="workspace-markdown-link-dialog"
      data-testid={testId}
      role="dialog"
      style={style}
      onPointerDown={(event) => event.stopPropagation()}
      onMouseDown={(event) => event.stopPropagation()}
    >
      {children}
    </div>
  );
}

function LinkEditDialog({ state, showLinkTitleField, onSubmit, onCancel }) {
  const [draft, setDraft] = React.useState({
    url: state.url ?? '',
    title: state.title ?? '',
    text: state.text ?? '',
  });

  React.useEffect(() => {
    setDraft({
      url: state.url ?? '',
      title: state.title ?? '',
      text: state.text ?? '',
    });
  }, [state.linkNodeKey, state.url, state.title, state.text]);

  return (
    <form
      className="workspace-markdown-link-dialog-form"
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit({
          url: draft.url,
          title: draft.title,
          text: state.withAnchorText ? draft.text : undefined,
        });
      }}
      onReset={(event) => {
        event.preventDefault();
        onCancel();
      }}
      onKeyDown={(event) => {
        if (event.key === 'Escape') {
          event.preventDefault();
          onCancel();
        }
      }}
    >
      <label className="workspace-markdown-link-dialog-field">
        <span>链接地址</span>
        <input
          aria-label="链接地址"
          autoFocus
          value={draft.url}
          onChange={(event) => setDraft((current) => ({ ...current, url: event.target.value }))}
          placeholder="https://"
        />
      </label>
      {state.withAnchorText ? (
        <label className="workspace-markdown-link-dialog-field">
          <span>链接文本</span>
          <input
            aria-label="链接文本"
            value={draft.text}
            onChange={(event) => setDraft((current) => ({ ...current, text: event.target.value }))}
          />
        </label>
      ) : null}
      {showLinkTitleField ? (
        <label className="workspace-markdown-link-dialog-field">
          <span>链接标题</span>
          <input
            aria-label="链接标题"
            value={draft.title}
            onChange={(event) => setDraft((current) => ({ ...current, title: event.target.value }))}
          />
        </label>
      ) : null}
      <div className="workspace-markdown-link-dialog-actions">
        <button
          type="submit"
          aria-label="保存链接"
          className="motion-button workspace-markdown-link-dialog-primary"
        >
          保存
        </button>
        <button type="reset" aria-label="取消编辑链接" className="motion-button workspace-markdown-link-dialog-secondary">
          取消
        </button>
      </div>
    </form>
  );
}

function LinkPreviewDialog({ state, onEdit, onRemove }) {
  const [copied, setCopied] = React.useState(false);

  const copyUrl = async () => {
    try {
      await window.navigator.clipboard.writeText(state.url);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1000);
    } catch {
      setCopied(false);
    }
  };

  const external = state.url.startsWith('http');

  return (
    <div className="workspace-markdown-link-preview">
      <a
        href={state.href ?? 'about:blank'}
        target={external ? '_blank' : undefined}
        rel={external ? 'noreferrer' : undefined}
        className="workspace-markdown-link-preview-url"
      >
        {state.url}
      </a>
      <div className="workspace-markdown-link-dialog-actions">
        <button type="button" className="motion-button" aria-label="编辑链接地址" onClick={onEdit}>
          编辑
        </button>
        <button type="button" className="motion-button" aria-label="复制链接地址" onClick={copyUrl}>
          {copied ? '已复制' : '复制'}
        </button>
        <button type="button" className="motion-button" aria-label="移除链接" onClick={onRemove}>
          移除
        </button>
      </div>
    </div>
  );
}

export function MarkdownLinkDialog() {
  const [state, showLinkTitleField, editorRootElementRef] = useCellValues(
    linkDialogState$,
    showLinkTitleField$,
    editorRootElementRef$
  );
  const updateLink = usePublisher(updateLink$);
  const cancelLinkEdit = usePublisher(cancelLinkEdit$);
  const switchFromPreviewToLinkEdit = usePublisher(switchFromPreviewToLinkEdit$);
  const removeLink = usePublisher(removeLink$);

  const popupHost = editorRootElementRef?.current;
  if (typeof document === 'undefined' || state.type === 'inactive' || !popupHost) return null;
  const viewportElement = popupHost.closest?.('.workspace-markdown-editor-scroll') || popupHost;
  const position = getDialogPosition(
    state.rectangle,
    popupHost.getBoundingClientRect(),
    viewportElement.getBoundingClientRect()
  );
  if (!position) return null;

  return createPortal(
    <LinkDialogSurface style={position} testId="markdown-link-dialog">
      {state.type === 'edit' ? (
        <LinkEditDialog
          state={state}
          showLinkTitleField={showLinkTitleField}
          onSubmit={updateLink}
          onCancel={cancelLinkEdit}
        />
      ) : (
        <LinkPreviewDialog
          state={state}
          onEdit={switchFromPreviewToLinkEdit}
          onRemove={removeLink}
        />
      )}
    </LinkDialogSurface>,
    popupHost
  );
}
