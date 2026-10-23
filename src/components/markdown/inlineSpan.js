// Only balanced, single-line spans are editable. Keep their original wrappers
// as data; never inject user HTML, CSS or event attributes into the page.
const spanTag = /<span(?=[\s>])(?:"[^"\r\n]*"|'[^'\r\n]*'|[^'"<>\r\n])*>|<\/span\s*>/ig;

export function readInlineSpan(source) {
  const newline = source.search(/[\r\n]/);
  if (newline !== -1) source = source.slice(0, newline);
  const tags = new RegExp(spanTag.source, 'ig');
  const first = tags.exec(source);
  if (!first || first.index !== 0 || /^<\//.test(first[0]) || /\/\s*>$/.test(first[0])) return null;
  let depth = 1;
  const codeRanges = [...source.slice(first[0].length).matchAll(/(`+)(?!`)([^\r\n]*?[^`])\1(?!`)/g)]
    .map(match => [first[0].length + match.index, first[0].length + match.index + match[0].length]);
  let tag;
  while ((tag = tags.exec(source))) {
    if (codeRanges.some(([start, end]) => tag.index >= start && tag.index < end)) continue;
    if (/^<\//.test(tag[0])) depth--;
    else if (/\/\s*>$/.test(tag[0])) return null;
    else if (++depth > 32) return null;
    if (depth === 0) return {
      raw: source.slice(0, tags.lastIndex), opening: first[0], closing: tag[0],
      inner: source.slice(first[0].length, tag.index),
    };
  }
  return null;
}

export function withoutSupportedSpans(source) {
  const starts = /<span(?=[\s>])/ig;
  let result = '';
  let cursor = 0;
  let start;
  while ((start = starts.exec(source))) {
    const span = readInlineSpan(source.slice(start.index));
    if (!span) continue;
    result += source.slice(cursor, start.index) + span.inner.replace(spanTag, '');
    cursor = start.index + span.raw.length;
    starts.lastIndex = cursor;
  }
  return result + source.slice(cursor);
}
