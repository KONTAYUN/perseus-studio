import React, { memo } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import remarkAutolinkBoundary from './remarkAutolinkBoundary.js';
import CodeBlock from './CodeBlock.jsx';
import { safeMarkdownUrl } from './markdownPolicy.js';

const components = {
  pre({ children }) {
    const code = React.Children.toArray(children).find(React.isValidElement);
    const language = /language-([^\s]+)/.exec(code?.props?.className || '')?.[1] || '';
    return <CodeBlock text={String(code?.props?.children || '')} language={language} />;
  },
  a({ href, children, title }) {
    return href ? <a href={href} title={title} target="_blank" rel="noopener noreferrer">{children}</a> : <span>{children}</span>;
  },
  img({ src, alt }) {
    return src ? <img src={src} alt={alt || ''} loading="lazy" referrerPolicy="no-referrer" /> : <span>{alt}</span>;
  },
  table({ children }) {
    return <div className="markdown-table-scroll custom-scrollbar"><table>{children}</table></div>;
  },
};

export default memo(function MarkdownContent({ content = '', className = '' }) {
  return (
    <div className={`markdown-content ${className}`}>
      <ReactMarkdown remarkPlugins={[remarkGfm, remarkAutolinkBoundary]} components={components} urlTransform={safeMarkdownUrl}>{content}</ReactMarkdown>
    </div>
  );
});
