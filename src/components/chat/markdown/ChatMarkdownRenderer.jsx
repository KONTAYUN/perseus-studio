import React from 'react';
import ReactMarkdown from 'react-markdown';
import rehypeHighlight from 'rehype-highlight';
import remarkBreaks from 'remark-breaks';
import remarkGfm from 'remark-gfm';
import ChatCodeBlock from './ChatCodeBlock';
import ChatMarkdownErrorBoundary from './ChatMarkdownErrorBoundary';
import { isLikelyBlockCode, safeMarkdownUrl } from './chatMarkdownUtils';

const remarkPlugins = [remarkGfm, remarkBreaks];
const rehypePlugins = [[rehypeHighlight, { detect: false, ignoreMissing: true }]];

const markdownComponents = {
  a({ href, children, node, ...props }) {
    const safeHref = safeMarkdownUrl(href);
    if (!safeHref) {
      return <span className="chat-markdown-unsafe-link">{children}</span>;
    }
    return (
      <a href={safeHref} target="_blank" rel="noreferrer noopener" {...props}>
        {children}
      </a>
    );
  },
  code({ inline, className = '', children, node, ...props }) {
    if (!inline && isLikelyBlockCode(className, children)) {
      return <ChatCodeBlock className={className}>{children}</ChatCodeBlock>;
    }
    return (
      <code className={className} {...props}>
        {children}
      </code>
    );
  },
  img({ src, alt = '', title, node, ...props }) {
    const safeSrc = safeMarkdownUrl(src);
    if (!safeSrc) {
      return alt ? <span className="chat-markdown-image-alt">{alt}</span> : null;
    }
    return <img src={safeSrc} alt={alt} title={title} loading="lazy" {...props} />;
  },
  input({ type, checked, node, ...props }) {
    if (type === 'checkbox') {
      return <input type="checkbox" checked={Boolean(checked)} disabled readOnly {...props} />;
    }
    return <input type={type} {...props} />;
  },
  pre({ children }) {
    return <>{children}</>;
  },
  table({ children }) {
    return (
      <div className="chat-markdown-table-wrapper custom-scrollbar">
        <table>{children}</table>
      </div>
    );
  },
};

function ChatMarkdownRenderer({ content = '' }) {
  return (
    <ChatMarkdownErrorBoundary content={content}>
      <div className="chat-markdown">
        <ReactMarkdown
          remarkPlugins={remarkPlugins}
          rehypePlugins={rehypePlugins}
          components={markdownComponents}
        >
          {content}
        </ReactMarkdown>
      </div>
    </ChatMarkdownErrorBoundary>
  );
}

export default React.memo(ChatMarkdownRenderer);
