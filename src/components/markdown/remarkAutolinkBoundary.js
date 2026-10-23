// GFM treats CJK punctuation as part of bare URLs. Restrict this adjustment
// to parser-created bare autolinks; explicit Markdown links remain literal.
export default function remarkAutolinkBoundary() {
  return (tree, file) => {
    const source = String(file.value);
    function visit(parent) {
      if (!parent.children) return;
      parent.children = parent.children.flatMap((node) => {
        if (node.type === 'link' && node.children?.length === 1 && node.children[0].type === 'text') {
          const original = source.slice(node.position?.start.offset, node.position?.end.offset);
          const label = node.children[0].value;
          if (original === label && /^(?:https?:\/\/|www\.)/i.test(original)) {
            const boundary = label.search(/[，。；：！？、（）【】《》“”‘’]/u);
            if (boundary > 0) {
              const head = label.slice(0, boundary);
              return [{ ...node, url: head.startsWith('www.') ? `http://${head}` : head,
                children: [{ type: 'text', value: head }] }, { type: 'text', value: label.slice(boundary) }];
            }
          }
        }
        visit(node);
        return [node];
      });
    }
    visit(tree);
  };
}
