export function formatFileSize(size) {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / 1024 / 1024).toFixed(1)} MB`;
}

export function visibleMessageAttachments(message) {
  return (message.attachments || []).filter(
    (item) =>
      item?.type !== 'phase_message' &&
      item?.kind !== 'phase_message' &&
      item?.type !== 'current_file' &&
      item?.kind !== 'current_file' &&
      item?.type !== 'editor_panels_context' &&
      item?.kind !== 'editor_panels_context'
  );
}

export function getSlashSkillQuery(value) {
  const match = value.match(/(?:^|\s)\/([^\s/]*)$/);
  return match ? match[1] : null;
}

export function removeSlashSkillQuery(value) {
  return value.replace(/(?:^|\s)\/([^\s/]*)$/, (match) => (match.startsWith(' ') ? ' ' : ''));
}

export function formatDate(value) {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return date.toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  });
}

export function summarizeCommandOutput(stdout, stderr) {
  const text = stderr || stdout;
  if (!text) return '';
  const lines = text.trim().split(/\r?\n/);
  return lines.slice(0, 8).join('\n');
}

export function formatToolLabel(item) {
  if (item.tool_name === 'inspect_workspace') return '正在铺开画布';
  if (item.tool_name === 'read_workspace_text') return '正在翻阅底稿';
  if (item.tool_name === 'save_workspace_file') return '正在装订成册';
  if (item.tool_name === 'manage_workspace_path') return '正在裁剪';
  if (item.tool_name === 'organize_workspace_matches') return '正在归档素材';
  if (item.tool_name === 'run_workspace_command') return '正在测算';
  if (item.tool_name === 'perseus_search_web' || item.tool_name === 'web_search') return '正在搜罗线索';
  if (item.tool_name === 'web_extract') return '正在摘录资料';
  return '正在烹饪';
}
