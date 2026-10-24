import { Lexer } from 'marked';
import { readInlineSpan } from '../../markdown/inlineSpan.js';
import { readMarkdownBreak } from '../../markdown/markdownLineBreak.js';

// CommonMark's HTML grammar identifies inert raw ranges; it never executes HTML.
function htmlToken(source, block = false) {
  const lexer = new Lexer({ gfm: true });
  return block ? lexer.tokenizer.html(source) : lexer.tokenizer.tag(source);
}
export const specialBlockStart = source => {
  // Marked asks for hints on source.slice(1). A match at zero could split an
  // inline code span or escaped tag into a new HTML block. Only hint real lines.
  const end = source.indexOf('\n\n');
  const index = source.slice(0, end < 0 ? source.length : end).search(/\n {0,3}(?:---|\+\+\+|\$\$|\\\[|:{3,}|\[\^|<)/);
  return index < 0 ? -1 : index + 1;
};
export const specialInlineStart = source => source.search(/<|!?\[\[|\[\^|\$\$|\\[([]/);

function delimitedBlock(source, opening, closing) {
  if (!opening.test(source.slice(0, source.indexOf('\n') < 0 ? source.length : source.indexOf('\n')))) return null;
  const lines = source.split('\n');
  let depth = 1;
  let fence = null;
  const container = /^ {0,3}:{3,}/.test(lines[0]);
  for (let i = 1; i < lines.length; i++) {
    if (container) {
      const marker = /^ {0,3}(`{3,}|~{3,})(.*)$/.exec(lines[i]);
      if (fence) {
        if (marker && marker[1][0] === fence[0] && marker[1].length >= fence.length && !marker[2].trim()) fence = null;
        continue;
      }
      if (marker) { fence = marker[1]; continue; }
    }
    if (closing.test(lines[i])) {
      if (--depth === 0) return lines.slice(0, i + 1).join('\n');
    } else if (container && /^ {0,3}:{3,}\s*\S/.test(lines[i])) depth++;
  }
  // An incomplete delimiter protects only its line, never the rest of a file.
  return lines[0];
}

export function readPreservedBlock(source) {
  if (source.startsWith('\uFEFF')) {
    const value = readPreservedBlock(source.slice(1));
    return value ? { ...value, raw: '\uFEFF' + value.raw } : null;
  }
  let raw = delimitedBlock(source, /^ {0,3}---\s*$/, /^ {0,3}(?:---|\.\.\.)\s*$/);
  if (raw && !raw.includes('\n')) raw = null; // Ordinary horizontal rule.
  raw ||= delimitedBlock(source, /^ {0,3}\+\+\+\s*$/, /^ {0,3}\+\+\+\s*$/);
  raw ||= delimitedBlock(source, /^ {0,3}\$\$\s*$/, /^ {0,3}\$\$\s*$/);
  raw ||= delimitedBlock(source, /^ {0,3}\\\[\s*$/, /^ {0,3}\\\]\s*$/);
  raw ||= delimitedBlock(source, /^ {0,3}:{3,}(?:[ \t]*\S.*)?$/, /^ {0,3}:{3,}\s*$/);
  if (raw) return { raw, label: '特殊段落' };
  const footnote = /^ {0,3}\[\^[^\]\n]+\]:[^\n]*(?:\n(?:(?: {4}|\t)[^\n]*|(?=\n(?: {4}|\t))))*/.exec(source);
  if (footnote) return { raw: footnote[0], label: '脚注' };
  if (readInlineSpan(source) || readMarkdownBreak(source)) return null;
  const html = htmlToken(source, true);
  if (html) return { raw: html.raw.replace(/\n+$/, ''), label: '嵌入内容' };
  // A multiline inline tag can be split by the block parser at a closing `>`
  // line (a quote marker). Capture it before that block-level interpretation.
  const tag = htmlToken(source);
  return tag?.raw.includes('\n') ? { raw: tag.raw, label: '嵌入内容' } : null;
}

export function readPreservedInline(source) {
  const special = /^(?:!?\[\[[^\]\n]+\]\]|\[\^[^\]\n]+\]|\$\$[^\n]*?\$\$|\\\([^\n]*?\\\)|\\\[[^\n]*?\\\])/.exec(source);
  if (special) return { raw: special[0], label: '特殊内容' };
  if (readInlineSpan(source) || readMarkdownBreak(source)) return null;
  const token = htmlToken(source);
  if (!token) return null;
  const opening = /^<([a-z][\w-]*)(?=[\s/>])/i.exec(token.raw);
  let raw = token.raw;
  if (opening && !/\/>$/.test(raw) && !/^(?:br|hr|img|input|meta|link|wbr)$/i.test(opening[1])) {
    let depth = 1;
    let offset = raw.length;
    while (offset < source.length && source[offset] !== '\n') {
      if (source[offset] !== '<') { offset++; continue; }
      const next = htmlToken(source.slice(offset));
      if (!next) { offset++; continue; }
      const tag = /^<(\/?)([a-z][\w-]*)(?=[\s/>])/i.exec(next.raw);
      if (tag && tag[2].toLowerCase() === opening[1].toLowerCase()) {
        depth += tag[1] ? -1 : /\/>$/.test(next.raw) ? 0 : 1;
        if (!depth) { raw = source.slice(0, offset + next.raw.length); break; }
      }
      offset += next.raw.length;
    }
  }
  return { raw, label: '嵌入内容' };
}

export const isReferenceToken = token => /^!?\[/.test(token.raw || '') && !/^!?\[(?:\\.|[^\]\\])*\]\(/.test(token.raw);

export function preservationTokenizers() {
  return [
    { name: 'preservedBlock', level: 'block', start: specialBlockStart, tokenizer(source) {
      const value = readPreservedBlock(source);
      return value ? { type: 'preservedBlock', ...value } : undefined;
    } },
    { name: 'preservedInline', level: 'inline', start: specialInlineStart, tokenizer(source) {
      const value = readPreservedInline(source);
      return value ? { type: 'preservedInline', ...value } : undefined;
    } },
  ];
}
