import { Extension } from '@tiptap/core';
import StarterKit from '@tiptap/starter-kit';
import Link from '@tiptap/extension-link';
import { TableKit, TableCell, TableHeader } from '@tiptap/extension-table';
import TaskList from '@tiptap/extension-task-list';
import TaskItem from '@tiptap/extension-task-item';
import Image from '@tiptap/extension-image';
import CodeBlock from '@tiptap/extension-code-block';
import { safeMarkdownUrl } from '../../markdown/markdownPolicy.js';
import { PreservedSpan } from '../richText/PreservedSpan.js';
import HardBreak from '@tiptap/extension-hard-break';
import { MarkdownTableEditing } from '../richText/MarkdownTableEditing.js';
import { FootnoteDefinition, Callout } from '../richText/DocumentNodes.js';
import { DocumentClipboard } from './clipboard.js';
import { imageDisplayURL, imageSourceFromHTML } from './imagePaths.js';

const cells = type => type.extend({ addAttributes() { return { ...this.parent?.(), textAlign: {
  default: null, parseHTML: element => ['left', 'center', 'right'].includes(element.style.textAlign) ? element.style.textAlign : null,
  renderHTML: attrs => attrs.textAlign ? { style: `text-align: ${attrs.textAlign}` } : {},
} }; } });
const SourceIdentity = Extension.create({
  name: 'sourceIdentity',
  addGlobalAttributes() { return [{ types: ['paragraph', 'heading', 'bulletList', 'orderedList', 'taskList', 'blockquote', 'codeBlock', 'table', 'horizontalRule', 'blockMath', 'documentMetadata', 'footnoteDefinition', 'callout', 'preservedBlock'], attributes: { sourceId: { default: null, rendered: false } } }]; },
});

const ReferenceLink = Link.extend({
  addAttributes() { return { ...this.parent?.(), reference: { default: null, rendered: false } }; },
  renderHTML({ HTMLAttributes }) { const href = safeMarkdownUrl(HTMLAttributes.href); return ['a', { ...HTMLAttributes, href: href || undefined }, 0]; },
});
const documentImage = path => Image.extend({
  addAttributes() { const attributes = this.parent?.(); return { ...attributes, src: { ...attributes?.src, parseHTML: imageSourceFromHTML }, reference: { default: null, rendered: false } }; },
  renderHTML({ node }) {
    const src = imageDisplayURL(node.attrs.src, path);
    return ['img', { src: src || undefined, alt: node.attrs.alt || '', title: node.attrs.title, loading: 'lazy', referrerpolicy: 'no-referrer' }];
  },
}).configure({ inline: true, allowBase64: false });

export function documentExtensions(codeBlock, preserved, atoms, path = '') {
  const code = codeBlock || CodeBlock.extend({ addAttributes() { return { ...this.parent?.(), meta: { default: null, rendered: false } }; } });
  return [StarterKit.configure({ underline: false, link: false, trailingNode: false, hardBreak: false, codeBlock: false }),
    ReferenceLink.configure({ openOnClick: false, autolink: false, isAllowedUri: value => Boolean(safeMarkdownUrl(value)) }),
    TableKit.configure({ table: { resizable: false }, tableCell: false, tableHeader: false }), cells(TableCell), cells(TableHeader),
    TaskList.extend({ content: '(taskItem | listItem)+', addAttributes() { return { ordered: { default: false, rendered: false }, start: { default: 1, rendered: false } }; } }), TaskItem.configure({ nested: true }), documentImage(path), PreservedSpan, HardBreak,
    MarkdownTableEditing, ...preserved, ...atoms, FootnoteDefinition, Callout, DocumentClipboard, code, SourceIdentity];
}
