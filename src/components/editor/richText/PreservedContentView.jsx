import React, { useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { NodeViewWrapper, ReactNodeViewRenderer } from '@tiptap/react';
import { Button } from '../../ui';
import useModalFocus from '../../../hooks/useModalFocus';
import { PreservedBlock, PreservedInline } from './PreservedMarkdown.js';
import { CodeCopyButton } from '../../markdown/CodeBlock.jsx';

function PreservedContentView({ node, editor, getPos }) {
  const inline = node.type.name === 'preservedInline';
  const [asking, setAsking] = useState(false), modal = useRef(null);
  useModalFocus(modal, asking, () => setAsking(false), 'button');
  function continueWriting(before) {
    const pos = getPos(); if (typeof pos !== 'number') return;
    const at = before ? pos : pos + node.nodeSize;
    if (inline) editor.chain().focus().setTextSelection(at).insertContent(' ').run();
    else editor.chain().focus().insertContentAt(at, { type: 'paragraph' }).setTextSelection(at + 1).run();
  }
  function remove() {
    const pos = getPos(); if (typeof pos === 'number') editor.chain().focus().deleteRange({ from: pos, to: pos + node.nodeSize }).run();
  }
  const definition = node.attrs.label === '链接定义';
  return <NodeViewWrapper as={inline ? 'span' : 'div'} contentEditable={false}
    className={inline ? 'rich-preserved-inline' : 'rich-preserved-block'} data-testid={inline ? 'preserved-inline' : 'preserved-block'}>
    {inline ? <button type="button" className="rich-preserved-inline-button motion-row" title="这部分已原样保留；可让助手调整" onClick={() => setAsking(true)}>{node.attrs.raw}</button> : <>
      <div className="rich-preserved-header"><span>{definition ? '链接地址定义 · 自动保留' : `${node.attrs.label || '特殊内容'} · 原样保留，正文可继续编辑`}</span><CodeCopyButton text={node.attrs.raw} /></div>
      <details><summary>查看保留内容</summary><pre className="rich-preserved-source custom-scrollbar">{node.attrs.raw}</pre></details>
      {!definition && <div className="rich-preserved-actions">
        <Button variant="ghost" size="xs" onClick={() => continueWriting(true)}>在前面写</Button>
        <Button variant="ghost" size="xs" onClick={() => continueWriting(false)}>在后面写</Button>
        <Button variant="ghost" size="xs" onClick={() => setAsking(true)}>让助手调整</Button>
        <Button variant="ghost" size="xs" onClick={remove}>删除（可撤销）</Button>
      </div>}
    </>}
    {asking && createPortal(<div className="modal-backdrop fixed inset-0 z-[500] flex items-center justify-center bg-black/45 p-4">
      <div ref={modal} role="dialog" aria-modal="true" aria-label="让助手调整特殊内容" tabIndex={-1} className="ui-overlay-surface rich-document-dialog">
        <h2>让助手调整这部分内容？</h2><p>将请当前助手提出通用格式的修改建议。原稿会保留，可能影响正文的建议需你确认后采用。</p>
        <div className="rich-document-dialog-actions"><Button variant="secondary" onClick={() => setAsking(false)}>取消</Button>
          <Button onClick={() => { editor.emit('documentAssist', { raw: node.attrs.raw }); setAsking(false); }}>提交给助手</Button></div>
      </div>
    </div>, document.body)}
  </NodeViewWrapper>;
}
const nodeView = () => ReactNodeViewRenderer(PreservedContentView);
export const RichPreservedBlock = PreservedBlock.extend({ addNodeView: nodeView });
export const RichPreservedInline = PreservedInline.extend({ addNodeView: nodeView });
