// Root nodes retain document scope: pagination never reparses a text fragment.
export function documentPages(doc, target = 32000) {
  const pages = []; let from = 0, size = 0;
  (doc.content || []).forEach((node, index) => {
    size += JSON.stringify(node).length;
    if (size >= target || index - from >= 119) { pages.push({ from, to: index + 1 }); from = index + 1; size = 0; }
  });
  if (from < doc.content.length) pages.push({ from, to: doc.content.length });
  return pages.length ? pages : [{ from: 0, to: 0 }];
}
