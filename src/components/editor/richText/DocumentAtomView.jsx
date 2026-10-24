import React, { useMemo } from 'react';
import { NodeViewWrapper, ReactNodeViewRenderer } from '@tiptap/react';
import 'katex/dist/katex.min.css';
import { InlineMath, BlockMath, DocumentMetadata, FootnoteReference } from './DocumentNodes.js';
import { readMetadata } from '../document/metadata.js';
import { Button } from '../../ui';
import { renderFormula } from '../../markdown/formula.js';
export { renderFormula } from '../../markdown/formula.js';

function DocumentAtomView({ node, editor, getPos }) {
  const inline = node.type.isInline;
  const type = node.type.name;
  const formula = useMemo(() => type.endsWith('Math') ? renderFormula(node.attrs.formula, !inline) : null, [type, node.attrs.formula, inline]);
  const edit = () => editor.emit('documentEdit', { kind: type, pos: getPos(), attrs: node.attrs });
  const metadata = type === 'documentMetadata' ? readMetadata(node.attrs.raw, node.attrs.format) : null;
  return <NodeViewWrapper as={inline ? 'span' : 'div'} contentEditable={false} className={`rich-document-atom ${inline ? 'is-inline' : ''}`} data-testid={`document-${type}`}>
    {type === 'footnoteReference' ? <button type="button" className="rich-footnote-reference motion-button" onClick={edit} title="编辑脚注">[{node.attrs.identifier}]</button>
      : metadata ? <div className="rich-document-properties"><div className="rich-preserved-header"><span>文档属性</span><Button size="xs" variant="ghost" onClick={edit}>查看属性</Button></div>
        {metadata.rows.slice(0, 8).map(row => <div key={row.key}><strong>{row.key}</strong>：{row.simple ? String(row.value) : '已保留复杂属性'}</div>)}
        {!metadata.editable && <span>原有属性已完整保留</span>}</div>
        : <button type="button" className="rich-formula-button motion-button" title="编辑公式" onClick={edit}>
          {formula?.html ? <span dangerouslySetInnerHTML={{ __html: formula.html }} /> : <span>{node.attrs.formula || '点击填写公式'}</span>}
          {formula?.error && <span className="rich-document-node-error">{formula.error}</span>}
        </button>}
  </NodeViewWrapper>;
}
const view = () => ReactNodeViewRenderer(DocumentAtomView);
export const RichDocumentAtoms = [InlineMath, BlockMath, DocumentMetadata, FootnoteReference].map(extension => extension.extend({ addNodeView: view }));
