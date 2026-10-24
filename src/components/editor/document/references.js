import { parseMarkdownTree } from './parser.js';

// Removing a shared definition must not break untouched uses of that address.
// Convert only affected uses to ordinary inline links/images; keep their URL,
// title and text. This expands the save range to the real dependency roots.
export function prepareReferences(doc) {
  const definitions = new Map();
  const collect = node => {
    if (node.type === 'preservedBlock' && node.attrs?.label === '链接定义') {
      for (const value of parseMarkdownTree(node.attrs.raw).children) {
        if (value.type === 'definition' && !definitions.has(value.identifier.toLowerCase())) definitions.set(value.identifier.toLowerCase(), value);
      }
    }
    for (const child of node.content || []) collect(child);
  };
  collect(doc);
  const affected = new Set();
  const content = doc.content.map((root, index) => {
    const visit = node => {
      const ref = node.attrs?.reference;
      const definition = ref && definitions.get(ref.identifier.toLowerCase());
      let result = node;
      if (ref && (!definition || definition.url !== ref.href || (definition.title || null) !== (ref.title || null))) {
        affected.add(index); result = { ...node, attrs: { ...node.attrs, reference: null } };
      }
      if (node.marks) result = { ...result, marks: node.marks.map(visit) };
      if (node.content) result = { ...result, content: node.content.map(visit) };
      return result;
    };
    return visit(root);
  });
  return { doc: { ...doc, content }, affected };
}
