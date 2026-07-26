export function editorPositionLabels(count) {
  if (count === 1) return ['current'];
  if (count === 2) return ['left', 'right'];
  if (count === 3) return ['left', 'middle', 'right'];
  return Array.from({ length: count }, (_, index) => {
    if (index === 0) return 'left';
    if (index === count - 1) return 'right';
    return `middle-${index}`;
  });
}

export function buildEditorContextFromEntries(entries) {
  const activeEntries = entries.filter(({ editor }) => editor.currentFilePath || editor.openTabs.length > 0);
  if (activeEntries.length === 0) return null;
  const labels = editorPositionLabels(activeEntries.length);
  return {
    id: 'editor_panels_context',
    type: 'editor_panels_context',
    kind: 'editor_panels_context',
    panels: activeEntries.map(({ panel, editor }, index) => ({
      panel_id: panel.id,
      index: index + 1,
      position_label: labels[index],
      active_file_path: editor.currentFilePath,
      active_file_name: editor.currentFilePath ? editor.currentFilePath.split('/').pop() || editor.currentFilePath : '',
      open_tabs: editor.openTabs.map((tab) => tab.path),
    })),
  };
}
