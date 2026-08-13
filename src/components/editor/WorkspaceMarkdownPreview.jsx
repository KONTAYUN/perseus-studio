import React from 'react';
import MarkdownRenderer from '../markdown/MarkdownRenderer.jsx';
import {
  resolveWorkspaceMarkdownImage,
  resolveWorkspaceMarkdownLink,
} from './workspaceMarkdownUtils.js';

export default function WorkspaceMarkdownPreview({ path, content, scrollTop = 0, onScrollTopChange, onOpenFile }) {
  const scrollRef = React.useRef(null);

  React.useLayoutEffect(() => {
    if (scrollRef.current) scrollRef.current.scrollTop = scrollTop;
  }, [path]);

  const resolveLink = React.useCallback(
    (href) => resolveWorkspaceMarkdownLink(path, href),
    [path]
  );
  const resolveImage = React.useCallback(
    (src) => resolveWorkspaceMarkdownImage(path, src),
    [path]
  );

  return (
    <div
      ref={scrollRef}
      data-testid="markdown-preview"
      className="workspace-markdown-preview custom-scrollbar h-full overflow-y-auto"
      onScroll={(event) => onScrollTopChange?.(event.currentTarget.scrollTop)}
    >
      <MarkdownRenderer
        content={content}
        className="chat-markdown workspace-markdown-document"
        resolveLink={resolveLink}
        resolveImage={resolveImage}
        onWorkspaceLink={onOpenFile}
      />
    </div>
  );
}
