export function buildRichTextSelectionContext(nextEditor, range, filePath) {
  if (!filePath || !nextEditor || !range || range.from >= range.to) return null;
  const selectedText = nextEditor.state.doc.textBetween(range.from, range.to, '\n\n', '\n').trim();
  if (!selectedText) return null;
  const contextRadius = 160;
  const beforeStart = Math.max(0, range.from - contextRadius);
  const afterEnd = Math.min(nextEditor.state.doc.content.size, range.to + contextRadius);
  return {
    id: 'text_selection',
    type: 'text_selection',
    kind: 'text_selection',
    path: filePath,
    file_type: 'text',
    selected_text: selectedText,
    text_before: nextEditor.state.doc.textBetween(beforeStart, range.from, '\n\n', '\n').trim(),
    text_after: nextEditor.state.doc.textBetween(range.to, afterEnd, '\n\n', '\n').trim(),
    start_offset: range.from,
    end_offset: range.to,
  };
}
