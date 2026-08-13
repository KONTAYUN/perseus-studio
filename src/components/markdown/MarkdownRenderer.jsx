import React from 'react';
import ReactMarkdown from 'react-markdown';
import rehypeHighlight from 'rehype-highlight';
import remarkBreaks from 'remark-breaks';
import remarkGfm from 'remark-gfm';
import MarkdownCodeBlock from './MarkdownCodeBlock.jsx';
import MarkdownErrorBoundary from './MarkdownErrorBoundary.jsx';
import { isLikelyBlockCode, safeMarkdownUrl } from './markdownUtils.js';

const remarkPlugins = [remarkGfm, remarkBreaks];
const rehypePlugins = [[rehypeHighlight, { detect: false, ignoreMissing: true }]];

function defaultLinkTarget(href) {
  const safeHref = safeMarkdownUrl(href);
  if (!safeHref) return null;
  return { href: safeHref, external: true };
}

function MarkdownRenderer({ content = '', className = 'chat-markdown', resolveLink, resolveImage, onWorkspaceLink }) {
  const components = React.useMemo(() => ({
    a({ href, children, node, ...props }) {
      const target = resolveLink ? resolveLink(href) : defaultLinkTarget(href);
      if (!target?.href && !target?.workspacePath) {
        return <span className="chat-markdown-unsafe-link">{children}</span>;
      }
      return (
        <a
          href={target.href || '#'}
          target={target.external ? '_blank' : undefined}
          rel={target.external ? 'noreferrer noopener' : undefined}
          onClick={target.workspacePath ? (event) => {
            event.preventDefault();
            onWorkspaceLink?.(target.workspacePath);
          } : undefined}
          {...props}
        >
          {children}
        </a>
      );
    },
    code({ inline, className: codeClassName = '', children, node, ...props }) {
      if (!inline && isLikelyBlockCode(codeClassName, children)) {
        return <MarkdownCodeBlock className={codeClassName}>{children}</MarkdownCodeBlock>;
      }
      return <code className={codeClassName} {...props}>{children}</code>;
    },
    img({ src, alt = '', title, node, ...props }) {
      const resolved = resolveImage ? resolveImage(src) : safeMarkdownUrl(src);
      if (!resolved) return alt ? <span className="chat-markdown-image-alt">{alt}</span> : null;
      return <img src={resolved} alt={alt} title={title} loading="lazy" {...props} />;
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
      return <div className="chat-markdown-table-wrapper custom-scrollbar"><table>{children}</table></div>;
    },
  }), [onWorkspaceLink, resolveImage, resolveLink]);

  return (
    <MarkdownErrorBoundary content={content}>
      <div className={className}>
        <ReactMarkdown remarkPlugins={remarkPlugins} rehypePlugins={rehypePlugins} components={components}>
          {content}
        </ReactMarkdown>
      </div>
    </MarkdownErrorBoundary>
  );
}

export default React.memo(MarkdownRenderer);
