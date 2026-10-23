import React, { lazy, Suspense, useLayoutEffect, useRef, useState } from 'react';
import MarkdownContent from '../markdown/MarkdownContent.jsx';
import { richEditingRestriction } from '../markdown/markdownPolicy.js';
const RichTextEditor = lazy(() => import('./richText/RichTextEditor.jsx'));

export default function MarkdownDocument({ content, onChange }) {
  const echoed = useRef(content);
  const [revision, setRevision] = useState(0);
  useLayoutEffect(() => {
    if (content !== echoed.current) {
      echoed.current = content;
      // External file replacement starts a fresh undo history. Undo must not
      // restore a previous agent/disk version after conflict resolution.
      setRevision(value => value + 1);
    }
  }, [content]);
  const restriction = richEditingRestriction(content);
  if (restriction) return (
    <div className="markdown-document-preview custom-scrollbar">
      <p className="rich-document-notice" role="status">{restriction}</p>
      <MarkdownContent content={content} />
    </div>
  );
  return <Suspense fallback={<div className="rich-document-loading">正在打开文档…</div>}>
    <RichTextEditor key={revision} content={content} onChange={next => { echoed.current = next; onChange(next); }} />
  </Suspense>;
}
