import React from 'react';
import CodeBlock from '@tiptap/extension-code-block';
import { NodeViewContent, NodeViewWrapper, ReactNodeViewRenderer } from '@tiptap/react';
import { CodeCopyButton } from '../../markdown/CodeBlock.jsx';

function CodeBlockView({ node }) {
  return (
    <NodeViewWrapper className="code-fence rich-code-fence">
      <div className="code-fence-header" contentEditable={false}>
        <span>{node.attrs.language || '纯文本'}</span>
        <CodeCopyButton text={node.textContent} />
      </div>
      <pre className="custom-scrollbar"><NodeViewContent as="code" /></pre>
    </NodeViewWrapper>
  );
}

export const RichCodeBlock = CodeBlock.extend({
  addNodeView() { return ReactNodeViewRenderer(CodeBlockView); },
});
