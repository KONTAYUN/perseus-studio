export function formatFileSize(size) {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / 1024 / 1024).toFixed(1)} MB`;
}

export function loadingBubbleText(message, fallbackText = '') {
  if (message?.loadingState && Object.prototype.hasOwnProperty.call(message.loadingState, 'visibleText')) {
    return message.loadingState.visibleText;
  }
  return fallbackText || '正在处理';
}

export function visibleMessageAttachments(message) {
  return (message.attachments || []).filter(
    (item) =>
      item?.tool_name !== 'ask_followup_question' &&
      item?.type !== 'phase_message' &&
      item?.kind !== 'phase_message' &&
      item?.type !== 'current_file' &&
      item?.kind !== 'current_file' &&
      item?.type !== 'editor_panels_context' &&
      item?.kind !== 'editor_panels_context'
  );
}

export function shouldReserveMessageFooter(message) {
  if (!message || message.pending) return false;
  if ((message.content || '').trim()) return true;
  if (visibleMessageAttachments(message).some((item) => !item?.tool_name)) return true;
  if (completedActionItems(message).length > 0) return true;
  if (message.error) return true;
  return false;
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

export function formatToolLabel(item) {
  if (item.tool_name === 'ask_followup_question') return '已询问用户';
  if (item.tool_name === 'find_workspace_files' || item.tool_name === 'search_workspace_text' || item.tool_name === 'inspect_workspace') return '已查看工作区';
  if (item.tool_name === 'read_workspace_text' || item.tool_name === 'read_workspace_files') return '已读取文件';
  if (item.tool_name === 'edit_workspace_text') return '已编辑文件';
  if (item.tool_name === 'save_workspace_file') return '已保存文件';
  if (item.tool_name === 'manage_workspace_path') return '已管理路径';
  if (item.tool_name === 'organize_workspace_matches') return '已整理文件';
  if (item.tool_name === 'run_workspace_command') return '已运行命令';
  if (item.tool_name === 'perseus_search_web' || item.tool_name === 'web_search') return '已搜索资料';
  if (item.tool_name === 'web_extract') return '已读取网页';
  if (item.tool_name === 'list_skills' || item.tool_name === 'read_skill' || item.tool_name === 'install_skill_from_attachment' || item.tool_name === 'delete_skill') return '已处理技能';
  return '已处理';
}

export function completedActionItems(message) {
  if (!message || message.pending || message.role !== 'assistant') return [];
  const seen = new Set();
  return visibleMessageAttachments(message)
    .filter((item) => item?.tool_name && item.tool_name !== 'ask_followup_question' && (item.status === 'success' || item.status === 'already_applied'))
    .map((item, index) => {
      const label = formatToolLabel(item);
      const key = `${item.tool_name}:${label}`;
      if (seen.has(key)) return null;
      seen.add(key);
      return {
        id: item.tool_call_id || `${key}:${index}`,
        label,
        toolName: item.tool_name,
      };
    })
    .filter(Boolean);
}

export function followupQuestionAttachment(message) {
  if (!message || message.role !== 'assistant' || message.pending) return null;
  const item = (message.attachments || []).find(
    (attachment) => attachment?.tool_name === 'ask_followup_question' && attachment?.status === 'success'
  );
  if (!item || !(item.question || '').trim()) return null;
  const seen = new Set();
  const options = (item.options || [])
    .map((option) => String(option || '').trim())
    .filter((option) => {
      if (!option || seen.has(option)) return false;
      seen.add(option);
      return true;
    })
    .slice(0, 4);
  return {
    question: item.question.trim(),
    options,
    allowFreeform: item.allow_freeform !== false,
  };
}

export function pendingFollowupQuestion(messages) {
  for (let index = (messages || []).length - 1; index >= 0; index -= 1) {
    const message = messages[index];
    if (!message || message.error) continue;
    if (message.role === 'user') return null;
    if (message.role !== 'assistant') continue;
    const question = followupQuestionAttachment(message);
    if (!question) continue;
    return {
      ...question,
      messageId: message.id,
      createdAt: message.created_at || '',
    };
  }
  return null;
}
