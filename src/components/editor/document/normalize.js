// Semantic projection ignores editor defaults and source spelling, never text,
// URLs, formula payloads, code, table alignment or inert source.
export function normalizeDocument(node, inTable = false) {
  const attrs = node.attrs || {};
  const output = { type: node.type };
  const fields = {
    heading: ['level'], orderedList: ['start'], taskList: ['ordered', 'start'], taskItem: ['checked'], codeBlock: ['language', 'meta'],
    image: ['src', 'alt', 'title'], link: ['href', 'title'], tableCell: ['textAlign', 'colspan', 'rowspan'], tableHeader: ['textAlign', 'colspan', 'rowspan'],
    preservedInline: ['raw'], preservedBlock: ['raw'], preservedSpan: ['opening', 'closing'],
    inlineMath: ['formula'], blockMath: ['formula', 'meta'], footnoteReference: ['identifier'], footnoteDefinition: ['identifier'],
    documentMetadata: ['format', 'raw'], callout: ['kind'],
  }[node.type] || [];
  if (fields.length) output.attrs = Object.fromEntries(fields.map(key => [key, attrs[key] ?? (['colspan', 'rowspan', 'start'].includes(key) ? 1 : key === 'ordered' ? false : null)]));
  if (node.type === 'text') output.text = node.text.replace(/\r\n?/g, '\n');
  if (node.marks?.length) output.marks = node.marks.map(mark => normalizeDocument(mark)).sort((a, b) => a.type.localeCompare(b.type));
  let content = (node.content || []).map(child => normalizeDocument(child, inTable || node.type === 'table'));
  const merged = [];
  for (const child of content) {
    const previous = merged.at(-1);
    if (child.type === 'text' && previous?.type === 'text' && JSON.stringify(child.marks) === JSON.stringify(previous.marks)) previous.text += child.text;
    else merged.push(child);
  }
  content = merged;
  if (!inTable && node.type === 'paragraph' && content.length === 1 && content[0].type === 'hardBreak') content = [];
  if (content.length) output.content = content;
  return output;
}
export const semanticKey = node => JSON.stringify(normalizeDocument(node));
