import StarterKit from '@tiptap/starter-kit';
import Underline from '@tiptap/extension-underline';
import Highlight from '@tiptap/extension-highlight';
import TextStyle from '@tiptap/extension-text-style';
import Color from '@tiptap/extension-color';
import TextAlign from '@tiptap/extension-text-align';
import Link from '@tiptap/extension-link';
import Placeholder from '@tiptap/extension-placeholder';
import TableRow from '@tiptap/extension-table-row';
import TableHeader from '@tiptap/extension-table-header';
import TableCell from '@tiptap/extension-table-cell';
import TaskList from '@tiptap/extension-task-list';
import TaskItem from '@tiptap/extension-task-item';
import { BlockSpacing, FontSize, PinnedSelection } from './noteEditorExtensions';
import NoteImage from './richTextImageExtension';
import NoteTable from './richTextTableExtension';

export function createRichTextEditorExtensions() {
  return [
    StarterKit,
    Underline,
    Highlight.configure({ multicolor: true }),
    TextStyle,
    FontSize,
    BlockSpacing,
    PinnedSelection,
    Color,
    TaskList,
    TaskItem.configure({ nested: true }),
    NoteTable.configure({ resizable: false }),
    TableRow,
    TableHeader,
    TableCell,
    TextAlign.configure({ types: ['heading', 'paragraph', 'blockquote'] }),
    Link.configure({ openOnClick: false }),
    NoteImage,
    Placeholder.configure({
      placeholder: '在这里写下你的笔记，或让 AI 接着整理它。',
    }),
  ];
}
