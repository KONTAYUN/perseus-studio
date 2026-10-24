// Attribute-free HTML breaks have a lossless hardBreak node in the editor.
// Keep one grammar for local preservation, Markdown parsing and safe preview.

export function readMarkdownBreak(source) {
  return /^<br[ \t]*\/?>/i.exec(source)?.[0] || '';
}

export default function remarkMarkdownBreaks() {
  return tree => {
    function visit(node) {
      if (!node.children) return;
      node.children = node.children.map(child => {
        if (child.type === 'html' && readMarkdownBreak(child.value) === child.value) return { type: 'break' };
        visit(child);
        return child;
      });
    }
    visit(tree);
  };
}
