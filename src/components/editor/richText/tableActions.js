export function selectedTable(editor) {
  const { $from } = editor.state.selection;
  for (let depth = $from.depth; depth > 0; depth--) {
    if ($from.node(depth).type.name === 'table') {
      return { node: $from.node(depth), pos: $from.before(depth) };
    }
  }
  return null;
}

export function paragraphOutsideTable(editor, before = false) {
  const table = selectedTable(editor);
  if (!table) return false;
  const position = before ? table.pos : table.pos + table.node.nodeSize;
  return editor.chain().focus().insertContentAt(position, { type: 'paragraph' })
    .setTextSelection(position + 1).scrollIntoView().run();
}

export function runTableAction(editor, action) {
  return editor.chain().focus()[action]().run();
}

export function alignTableColumn(editor, alignment) {
  const selected = selectedTable(editor);
  if (!selected) return false;
  const { $from } = editor.state.selection;
  let depth = $from.depth;
  while (depth > 0 && $from.node(depth).type.name !== 'tableRow') depth--;
  const column = depth > 0 ? $from.index(depth) : 0;
  const tr = editor.state.tr;
  selected.node.forEach((row, offset) => row.forEach((cell, cellOffset, index) => {
    if (index === column) tr.setNodeMarkup(selected.pos + offset + cellOffset + 2, undefined, { ...cell.attrs, textAlign: alignment });
  }));
  editor.view.dispatch(tr); editor.commands.focus(); return true;
}

export function tableActionGroups(table) {
  return {
    rows: [
      ['在上方插入行', 'addRowBefore'], ['在下方插入行', 'addRowAfter'],
      ['删除当前行', 'deleteRow', table.childCount <= 1],
    ],
    columns: [
      ['在左侧插入列', 'addColumnBefore'], ['在右侧插入列', 'addColumnAfter'],
      ['删除当前列', 'deleteColumn', table.firstChild.childCount <= 1],
    ],
  };
}
