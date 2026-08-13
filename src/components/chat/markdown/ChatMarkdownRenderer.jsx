import React from 'react';
import MarkdownRenderer from '../../markdown/MarkdownRenderer.jsx';

function ChatMarkdownRenderer({ content = '' }) {
  return <MarkdownRenderer content={content} />;
}

export default React.memo(ChatMarkdownRenderer);
