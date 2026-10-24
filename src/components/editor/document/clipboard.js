import { Extension } from '@tiptap/core';
import { Plugin } from '@tiptap/pm/state';
import { DOMParser as PMDOMParser } from '@tiptap/pm/model';
import { parseDocument } from './fromMarkdown.js';
import { selectedTable } from '../richText/tableActions.js';

const allowed = new Set('a b strong em i s del strike code pre p br hr h1 h2 h3 h4 h5 h6 ul ol li blockquote table thead tbody tfoot tr th td img span div aside input label'.split(' '));
function clearSourceIdentity(node) {
  if (node.attrs) delete node.attrs.sourceId;
  for (const child of node.content || []) clearSourceIdentity(child);
  return node;
}
export function pastedDocument(data, schema) {
  const html = data.getData('text/html');
  if (!html) { const text = data.getData('text/plain'); return text ? clearSourceIdentity(parseDocument(text).doc) : null; }
  const parsed = new DOMParser().parseFromString(html, 'text/html');
  const unsafe = [...parsed.body.querySelectorAll('*')].some(node => !allowed.has(node.tagName.toLowerCase()))
    || parsed.querySelector('script, iframe, object, embed, svg, math')
    || parsed.querySelector('table [colspan]:not([colspan="1"]), table [rowspan]:not([rowspan="1"]), table table, td ul, td ol, th ul, th ol, td pre, th pre');
  if (unsafe) return { type: 'doc', content: [{ type: 'preservedBlock', attrs: { raw: html, label: '保留的粘贴内容' } }] };
  for (const cell of parsed.querySelectorAll('td,th')) {
    const paragraphs = cell.querySelectorAll(':scope > p, :scope > div');
    paragraphs.forEach((paragraph, index) => {
      if (index) paragraph.before(parsed.createElement('br'));
      paragraph.replaceWith(...paragraph.childNodes);
    });
    const alignment = cell.getAttribute('align'); if (['left', 'center', 'right'].includes(alignment)) cell.style.textAlign = alignment;
  }
  const doc = PMDOMParser.fromSchema(schema).parse(parsed.body).toJSON();
  function normalize(node) {
    if (node.attrs) delete node.attrs.sourceId;
    if (node.type === 'table') node.content.forEach((row, index) => row.content.forEach(cell => { cell.type = index ? 'tableCell' : 'tableHeader'; }));
    for (const child of node.content || []) normalize(child);
  }
  normalize(doc); return doc;
}

export const DocumentClipboard = Extension.create({
  name: 'documentClipboard',
  addProseMirrorPlugins() {
    const editor = this.editor;
    function insert(data) {
      if (!data || data.types.includes('workspace_path') || data.types.includes('editor_tab_path')) return false;
      if (editor.isActive('codeBlock')) {
        const text = data.getData('text/plain');
        if (!text) return false;
        editor.view.dispatch(editor.state.tr.insertText(text)); return true;
      }
      const doc = pastedDocument(data, editor.schema); if (!doc) return false;
      const table = selectedTable(editor);
      if (table) {
        if (doc.content.every(block => ['paragraph', 'heading'].includes(block.type))) {
          const content = doc.content.flatMap((block, index) => [...(index ? [{ type: 'hardBreak' }] : []), ...(block.content || []).flatMap(node => {
            if (node.type !== 'text' || !node.text.includes('\n')) return [node];
            return node.text.split(/\r?\n/).flatMap((text, line) => [...(line ? [{ type: 'hardBreak' }] : []), ...(text ? [{ ...node, text }] : [])]);
          })]);
          editor.commands.insertContent(content); return true;
        }
        editor.commands.insertContentAt(table.pos + table.node.nodeSize, doc.content);
        editor.emit('documentNotice', '粘贴内容包含独立结构，已放在表格后，原内容完整保留。'); return true;
      }
      editor.commands.insertContent(doc.content); return true;
    }
    return [new Plugin({ props: {
      handlePaste(_view, event) { return insert(event.clipboardData); },
      handleDrop(_view, event, _slice, moved) { return !moved && !event.dataTransfer?.files.length && insert(event.dataTransfer); },
    } })];
  },
});
