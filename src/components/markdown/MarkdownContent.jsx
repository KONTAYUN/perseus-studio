import React, { memo } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import remarkMath from 'remark-math';
import remarkFrontmatter from 'remark-frontmatter';
import remarkDocumentPreview from './remarkDocumentPreview.js';
import { renderFormula } from './formula.js';
import 'katex/dist/katex.min.css';
import remarkAutolinkBoundary from './remarkAutolinkBoundary.js';
import remarkMarkdownBreaks from './markdownLineBreak.js';
import CodeBlock from './CodeBlock.jsx';
import { safeMarkdownUrl } from './markdownPolicy.js';

const components = {
  code({ className, children }) {
    if (!className?.includes('language-math')) return <code className={className}>{children}</code>;
    const formula = renderFormula(String(children).replace(/\n$/, ''), className.includes('math-display'));
    return formula.html ? <span className={className} dangerouslySetInnerHTML={{ __html: formula.html }} /> : <code title={formula.error}>{children}</code>;
  },
  pre({ children }) {
    const code = React.Children.toArray(children).find(React.isValidElement);
    const language = /language-([^\s]+)/.exec(code?.props?.className || '')?.[1] || '';
    if (language === 'math') return <div className="rich-formula-preview">{children}</div>;
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
      <ReactMarkdown remarkPlugins={[remarkGfm, remarkMath, [remarkFrontmatter, ['yaml', 'toml']], remarkAutolinkBoundary, remarkMarkdownBreaks, remarkDocumentPreview]} components={components} urlTransform={safeMarkdownUrl}>{content}</ReactMarkdown>
    </div>
  );
});
