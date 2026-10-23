import StarterKit from '@tiptap/starter-kit';
import { Markdown } from '@tiptap/markdown';
import { TableKit } from '@tiptap/extension-table';
import TaskList from '@tiptap/extension-task-list';
import TaskItem from '@tiptap/extension-task-item';
import Image from '@tiptap/extension-image';
import { safeMarkdownUrl } from '../../markdown/markdownPolicy.js';
import { PreservedSpan } from './PreservedSpan.js';

export function markdownExtensions(codeBlock) {
  return [
    StarterKit.configure({
      underline: false, trailingNode: false,
      ...(codeBlock ? { codeBlock: false } : {}),
      link: { openOnClick: false, autolink: false, isAllowedUri: url => Boolean(safeMarkdownUrl(url)) },
    }),
    ...(codeBlock ? [codeBlock] : []),
    TableKit.configure({ table: { resizable: false } }),
    TaskList, TaskItem.configure({ nested: true }),
    Image.configure({ allowBase64: false, HTMLAttributes: { loading: 'lazy', referrerpolicy: 'no-referrer' } }),
    PreservedSpan,
    Markdown.configure({ markedOptions: { gfm: true } }),
  ];
}

export function serializeMarkdown(editor, original) {
  const text = editor.getMarkdown();
  const eol = original.includes('\r\n') ? '\r\n' : '\n';
  const result = text.replace(/\r?\n/g, eol);
  return result && /\r?\n$/.test(original) && !/\r?\n$/.test(result) ? result + eol : result;
}
