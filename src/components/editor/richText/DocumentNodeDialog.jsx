import React, { useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { Button, Input, Textarea } from '../../ui';
import useModalFocus from '../../../hooks/useModalFocus';
import { safeMarkdownUrl } from '../../markdown/markdownPolicy.js';
import { readMetadata, updateMetadata } from '../document/metadata.js';
import { renderFormula } from './DocumentAtomView.jsx';
import FootnoteEditor from './FootnoteEditor.jsx';
import DocumentImagePicker from './DocumentImagePicker.jsx';

const titles = { inlineMath: '行内公式', blockMath: '独立公式', image: '图片', footnoteReference: '脚注', documentMetadata: '文档属性', callout: '提示段落' };

export default function DocumentNodeDialog({ editor, controller, path }) {
  const [target, setTarget] = useState(null);
  useEffect(() => {
    const open = async value => {
      if (value.kind === 'footnoteReference' && value.attrs?.identifier && controller?.view.pages > 1) {
        try {
          await controller.flush(); const page = await controller.locateNote(value.attrs.identifier);
          if (page !== null && page !== controller.view.page) { await controller.navigate(page); return; }
        } catch { return; }
      }
      setTarget({ ...value, from: editor.state.selection.from, to: editor.state.selection.to });
    };
    editor.on('documentEdit', open); return () => editor.off('documentEdit', open);
  }, [editor]);
  return target ? <NodeDialog key={`${target.kind}-${target.pos}`} editor={editor} target={target} path={path} onClose={() => setTarget(null)} /> : null;
}

function NodeDialog({ editor, target, path, onClose }) {
  const modal = useRef(null);
  const input = useRef(null);
  const attrs = target.attrs || {};
  const math = target.kind.endsWith('Math');
  const [value, setValue] = useState(attrs.formula || attrs.src || '');
  const [alt, setAlt] = useState(attrs.alt || '');
  const [kind, setKind] = useState(attrs.kind || 'NOTE');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const metadata = target.kind === 'documentMetadata' ? readMetadata(attrs.raw, attrs.format) : null;
  const [properties, setProperties] = useState(() => Object.fromEntries((metadata?.rows || []).map(row => [row.key, String(row.value ?? '')])));
  const definition = useRef(null);
  const footnoteEditor = useRef(null);
  if (target.kind === 'footnoteReference' && !definition.current) editor.state.doc.descendants((node, pos) => {
    if (!definition.current && node.type.name === 'footnoteDefinition' && node.attrs.identifier === attrs.identifier) definition.current = { pos, node };
  });
  const note = useRef(definition.current?.node.toJSON().content || [{ type: 'paragraph' }]);
  useModalFocus(modal, true, onClose, 'input,textarea,button');
  const preview = math ? renderFormula(value, target.kind === 'blockMath') : null;
  function submit(event) {
    event.preventDefault(); setError('');
    try {
      if (target.pos !== undefined) {
        const current = editor.state.doc.nodeAt(target.pos);
        if (!current || current.type.name !== target.kind || JSON.stringify(current.attrs) !== JSON.stringify(attrs)) throw new Error('内容已变化，请重新打开后编辑。');
      }
      if (math && (!value.trim() || preview.error)) throw new Error(preview.error || '请填写公式');
      if (busy) return;
      if (target.kind === 'image' && !safeMarkdownUrl(value)) throw new Error('请先上传或选择一张图片');
      const nextAttrs = math ? { ...attrs, formula: value } : target.kind === 'image' ? { ...attrs, src: value, alt } : target.kind === 'documentMetadata' ? { ...attrs, raw: updateMetadata(attrs.raw, attrs.format, properties) } : { ...attrs, kind };
      if (target.kind === 'footnoteReference') {
        if (!note.current.some(block => block.content?.length)) throw new Error('请填写脚注内容');
        const used = new Set(); editor.state.doc.descendants(node => { if (node.attrs.identifier) used.add(node.attrs.identifier); });
        let identifier = attrs.identifier;
        if (!identifier) identifier = `note-${crypto.randomUUID().slice(0, 8)}`;
        const liveContent = footnoteEditor.current?.getJSON?.().content;
        const content = Array.isArray(liveContent) ? liveContent : note.current;
        note.current = content;
        if (definition.current && !editor.state.doc.nodeAt(definition.current.pos)?.eq(definition.current.node)) throw new Error('脚注已变化，请重新打开后编辑。');
        const chain = editor.chain().focus();
        if (definition.current) chain.insertContentAt({ from: definition.current.pos, to: definition.current.pos + definition.current.node.nodeSize }, { type: 'footnoteDefinition', attrs: { identifier }, content });
        else chain.insertContentAt(editor.state.doc.content.size, { type: 'footnoteDefinition', attrs: { identifier }, content });
        if (target.pos === undefined) chain.insertContentAt({ from: target.from, to: target.to }, { type: 'footnoteReference', attrs: { identifier } });
        chain.run();
      } else if (target.pos !== undefined) editor.view.dispatch(editor.state.tr.setNodeMarkup(target.pos, undefined, nextAttrs));
      else editor.chain().focus().insertContentAt({ from: target.from, to: target.to }, target.kind === 'callout'
        ? { type: 'callout', attrs: nextAttrs, content: [{ type: 'paragraph' }] } : { type: target.kind, attrs: nextAttrs }).run();
      onClose(); editor.commands.focus();
    } catch (failure) { setError(failure.message); }
  }
  return createPortal(<div className="modal-backdrop fixed inset-0 z-[500] flex items-center justify-center bg-black/45 p-4">
    <form ref={modal} role="dialog" aria-modal="true" aria-label={titles[target.kind]} tabIndex={-1} onSubmit={submit} className="ui-overlay-surface rich-document-dialog custom-scrollbar">
      <h2>{titles[target.kind]}</h2>
      {math && <><label>公式<Textarea ref={input} aria-label="公式内容" value={value} onChange={event => setValue(event.target.value)} /></label>
        <div className="rich-formula-symbols">{['\\frac{a}{b}', 'x^{2}', '\\sqrt{x}', '\\sum_{i=1}^{n}', '\\alpha', '\\leq'].map(symbol => <Button key={symbol} type="button" variant="ghost" size="xs" onClick={() => setValue(current => current + symbol)}>{symbol}</Button>)}</div>
        {preview.html && <div className="rich-formula-preview" dangerouslySetInnerHTML={{ __html: preview.html }} />}</>}
      {target.kind === 'image' && <DocumentImagePicker value={value} onChange={setValue} alt={alt} onAltChange={setAlt} onBusyChange={setBusy} path={path} />}
      {target.kind === 'footnoteReference' && <FootnoteEditor content={note.current} onChange={content => { note.current = content; }} onReady={instance => { footnoteEditor.current = instance; }} />}
      {target.kind === 'callout' && <label>提示类型<select aria-label="提示类型" value={kind} onChange={event => setKind(event.target.value)}>{['NOTE', 'TIP', 'IMPORTANT', 'WARNING', 'CAUTION'].map(type => <option key={type}>{type}</option>)}</select></label>}
      {metadata && (metadata.editable ? metadata.rows.map(row => <label key={row.key}>{row.key}{row.type === 'boolean' ? <select aria-label={row.key} value={properties[row.key]} onChange={event => setProperties(current => ({ ...current, [row.key]: event.target.value }))}><option>true</option><option>false</option></select> : <Input aria-label={row.key} value={properties[row.key]} onChange={event => setProperties(current => ({ ...current, [row.key]: event.target.value }))} />}</label>) : <p>复杂属性已完整保留，正文仍可继续编辑。可在聊天中让助手调整文档属性。</p>)}
      {(error || preview?.error) && <p role="alert">{error || preview.error}</p>}
      <div className="rich-document-dialog-actions"><Button variant="secondary" onClick={onClose}>取消</Button><Button type="submit" disabled={busy || (target.kind === 'image' && !value) || (metadata && !metadata.editable)}>应用</Button></div>
    </form>
  </div>, document.body);
}
