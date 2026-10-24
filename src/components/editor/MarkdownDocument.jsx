import React, { lazy, Suspense, useLayoutEffect, useRef, useState } from 'react';
import DocumentEditorBoundary from './DocumentEditorBoundary.jsx';
const RichTextEditor = lazy(() => import('./richText/RichTextEditor.jsx'));

export default function MarkdownDocument({ content, onChange, path, userId }) {
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
  const change = next => { echoed.current = next; onChange(next); };
  return <DocumentEditorBoundary key={revision} content={content} onChange={change}>
    <Suspense fallback={<div className="rich-document-loading">正在打开文档…</div>}>
      <RichTextEditor content={content} onChange={change} path={path} userId={userId} />
    </Suspense>
  </DocumentEditorBoundary>;
}
