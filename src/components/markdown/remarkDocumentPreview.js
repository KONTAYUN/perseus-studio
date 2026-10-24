export default function remarkDocumentPreview() {
  return tree => {
    function visit(node) {
      if (node.type === 'yaml' || node.type === 'toml') { node.lang = node.type; node.type = 'code'; }
      if (node.type === 'blockquote' && node.children[0]?.type === 'paragraph') {
        const first = node.children[0].children[0];
        const match = first?.type === 'text' && /^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\](?:\n|$)/.exec(first.value);
        if (match) {
          first.value = first.value.slice(match[0].length);
          node.data = { hName: 'aside', hProperties: { 'data-callout': match[1], className: 'rich-callout' } };
        }
      }
      if (node.type === 'html') { node.type = 'text'; }
      for (const child of node.children || []) visit(child);
    }
    visit(tree);
  };
}
