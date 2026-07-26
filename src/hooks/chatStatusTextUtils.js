export const PLAYFUL_PENDING_STATUSES = [
  '正在思考',
  '正在整理',
  '正在阅读上下文',
  '正在准备工具',
  '正在检查细节',
  '正在生成回复',
];

export const EXTRA_PENDING_STATUSES = [
  '正在查找资料',
  '正在梳理线索',
  '正在确认状态',
  '正在归纳信息',
  '正在处理请求',
  '正在等待模型',
];

const LOADING_DISPLAY_MIN_MS = 3000;
const LOADING_BACKSPACE_MS = 28;
const LOADING_TYPE_MS = 32;
const DISPLAY_STATUS_POOL = [...PLAYFUL_PENDING_STATUSES, ...EXTRA_PENDING_STATUSES];

export function nextLoadingStatus(seed = 0) {
  return DISPLAY_STATUS_POOL[seed % DISPLAY_STATUS_POOL.length] || DISPLAY_STATUS_POOL[0];
}

export function updateLoadingState(message, now = Date.now()) {
  const current = message?.loadingState || {
    text: nextLoadingStatus(0),
    visibleText: nextLoadingStatus(0),
    stage: 'holding',
    activeSince: now,
  };

  if (!message?.pending) return message;
  if (current.stage === 'holding' && now - current.activeSince >= LOADING_DISPLAY_MIN_MS) {
    return {
      ...message,
      loadingState: {
        ...current,
        stage: 'deleting',
        activeSince: now,
      },
    };
  }

  if (current.stage === 'deleting' && now - current.activeSince >= LOADING_BACKSPACE_MS) {
    const nextVisible = current.visibleText.slice(0, -1);
    if (nextVisible) {
      return {
        ...message,
        loadingState: {
          ...current,
          visibleText: nextVisible,
          activeSince: now,
        },
      };
    }
    const nextText = nextLoadingStatus((DISPLAY_STATUS_POOL.indexOf(current.text) + 1 + DISPLAY_STATUS_POOL.length) % DISPLAY_STATUS_POOL.length);
    return {
      ...message,
      loadingState: {
        text: nextText,
        visibleText: '',
        stage: 'typing',
        activeSince: now,
      },
    };
  }

  if (current.stage === 'typing' && now - current.activeSince >= LOADING_TYPE_MS) {
    const nextVisible = current.text.slice(0, current.visibleText.length + 1);
    if (nextVisible.length < current.text.length) {
      return {
        ...message,
        loadingState: {
          ...current,
          visibleText: nextVisible,
          activeSince: now,
        },
      };
    }
    return {
      ...message,
      loadingState: {
        ...current,
        visibleText: current.text,
        stage: 'holding',
        activeSince: now,
      },
    };
  }

  return message;
}

export function agentEventText(type, payload = {}) {
  switch (type) {
    case 'model_call_started':
    case 'model_call_attempt_started':
    case 'model_call_attempt_completed':
    case 'model_message_started':
      return '正在等待模型响应';
    case 'model_call_retrying':
      return providerRetryStatus(payload);
    case 'model_call_attempt_failed':
      return '请求失败，正在准备重试';
    case 'model_stream_recovering':
      return '响应中断，正在恢复';
    case 'stream_reconnecting':
      return streamReconnectStatus(payload);
    case 'transport_fallback':
      return '流式响应不稳定，正在切换普通响应';
    case 'provider_rate_limited':
    case 'provider_cooldown_updated':
    case 'provider_auto_resume_scheduled':
      return rateLimitStatus(payload);
    case 'provider_auto_resume_started':
      return '等待结束，正在自动继续';
    case 'tool_call_started':
      return pendingToolStatus(payload);
    case 'tool_call_completed':
      return completedToolStatus(payload);
	case 'user_input_requested':
	case 'user_choice_requested':
	case 'user_question_requested':
      return '';
    case 'tool_call_failed':
    case 'tool_result_returned':
      return payload.is_error ? '工具返回异常，正在调整' : '';
    case 'completion_rejected':
      return '正在校验回复';
    case 'agent_protocol_retry':
      return '正在继续执行';
    case 'agent_protocol_failed':
      return '模型没有选择可执行动作';
    case 'context_compaction_started':
      return payload.reason === 'auto_threshold' ? '正在自动压缩上下文' : '正在压缩上下文';
    case 'context_compaction_completed':
      if (payload.reason === 'auto_threshold') return '上下文已自动压缩';
      if (payload.reason === 'overflow_retry') return '上下文已压缩，正在重试';
      return '上下文已压缩';
    case 'context_compaction_failed':
      return '上下文压缩已跳过';
    case 'context_overflow_recovered':
      return '上下文已整理，正在继续';
    case 'tool_results_pruned':
      return '';
    case 'agent_recovery_started':
    case 'agent_recovery_retrying':
      return recoveryStatus(payload);
    case 'agent_recovery_completed':
      return '已从断点继续';
    case 'agent_recovery_failed':
      return '断点继续失败';
    case 'agent_budget_exhausted':
      return '达到轮次上限，正在总结当前进度';
    case 'agent_budget_summary_started':
      return '正在总结当前进度';
    case 'agent_budget_summary_completed':
      return '已总结当前进度，可继续本轮';
    case 'agent_budget_summary_failed':
      return '总结生成失败，已保留断点';
    case 'agent_steer_received':
      return '已收到引导，会在下一次安全边界应用';
    case 'agent_steer_applied':
      return '引导已应用到当前任务';
    case 'agent_steer_rejected':
      return payload.message || '引导未应用';
    case 'local_persistence_failed':
      return '本地状态保存失败，本轮已暂停，可继续本轮';
    case 'model_delta':
    case 'model_message_completed':
      return '';
    case 'diagnostic_created':
    case 'run_started':
      return playfulPendingStatus(type);
    case 'run_completed':
      return '执行完成';
    default:
      return '';
  }
}

function providerRetryStatus(payload = {}) {
  const attempt = Number(payload.attempt) || 1;
  const maxAttempts = Number(payload.max_attempts) || 1;
  const delaySeconds = Math.ceil((Number(payload.delay_ms) || Number(payload.retry_after_ms) || 0) / 1000);
  if (delaySeconds > 0) {
    return `请求失败，${delaySeconds} 秒后重试 ${attempt}/${maxAttempts}`;
  }
  return `请求失败，正在重试 ${attempt}/${maxAttempts}`;
}

function streamReconnectStatus(payload = {}) {
  const attempt = Number(payload.attempt) || 1;
  const maxAttempts = Number(payload.max_attempts) || 1;
  const delaySeconds = Math.ceil((Number(payload.delay_ms) || 0) / 1000);
  if (delaySeconds > 0) {
    return `响应中断，${delaySeconds} 秒后重连 ${attempt}/${maxAttempts}`;
  }
  return `响应中断，正在重连 ${attempt}/${maxAttempts}`;
}

function rateLimitStatus(payload = {}) {
  const seconds = Math.ceil((Number(payload.retry_after_ms) || 0) / 1000);
  return seconds > 0 ? `模型限流，${seconds} 秒后自动继续` : '模型限流，等待自动继续';
}

function recoveryStatus(payload = {}) {
  const attempt = Number(payload.attempt) || 1;
  const maxAttempts = Number(payload.max_attempts) || 2;
  const delaySeconds = Math.ceil((Number(payload.delay_ms) || 0) / 1000);
  if (delaySeconds > 0) {
    return `连接中断，${delaySeconds} 秒后从断点继续 ${attempt}/${maxAttempts}`;
  }
  return `连接中断，正在从断点继续 ${attempt}/${maxAttempts}`;
}

export function pendingToolStatus(input) {
  const { toolName, statusKind, target } = normalizeToolStatusInput(input);
  const byKind = pendingStatusByKind(statusKind, target);
  if (byKind) return byKind;
  if (toolName === 'find_workspace_files' || toolName === 'search_workspace_text' || toolName === 'inspect_workspace') return target ? `正在查看 ${target}` : '正在查看工作区';
  if (toolName === 'read_workspace_text' || toolName === 'read_workspace_files' || toolName === 'read_workspace_note_view') return target ? `正在读取 ${target}` : '正在读取文件';
  if (toolName === 'manage_workspace_path') return target ? `正在管理 ${target}` : '正在管理路径';
  if (toolName === 'organize_workspace_matches') return target ? `正在整理 ${target}` : '正在整理文件';
  if (toolName === 'run_workspace_command') return target ? `正在运行 ${target} 的命令` : '正在运行命令';
  if (toolName === 'perseus_search_web' || toolName === 'web_search') return target ? `正在搜索 ${target}` : '正在搜索资料';
  if (toolName === 'web_extract') return target ? `正在读取 ${target}` : '正在读取网页';
  if (toolName === 'list_skills' || toolName === 'read_skill' || toolName === 'install_skill_from_attachment' || toolName === 'delete_skill') return target ? `正在处理 ${target}` : '正在处理技能';
  return playfulPendingStatus(toolName || 'default');
}

export function completedToolStatus(input) {
  const { toolName, statusKind, target } = normalizeToolStatusInput(input);
  if (toolName === 'request_user_input' || toolName === 'request_user_choice' || toolName === 'ask_followup_question') return '';
  const byKind = completedStatusByKind(statusKind, target);
  if (byKind) return byKind;
  if (toolName === 'find_workspace_files' || toolName === 'search_workspace_text' || toolName === 'inspect_workspace') return target ? `已查看 ${target}` : '已查看工作区';
  if (toolName === 'read_workspace_text' || toolName === 'read_workspace_files' || toolName === 'read_workspace_note_view') return target ? `已读取 ${target}` : '已读取文件';
  if (toolName === 'manage_workspace_path') return target ? `已管理 ${target}` : '已管理路径';
  if (toolName === 'organize_workspace_matches') return target ? `已整理 ${target}` : '已整理文件';
  if (toolName === 'run_workspace_command') return target ? `已运行 ${target} 的命令` : '已运行命令';
  if (toolName === 'perseus_search_web' || toolName === 'web_search') return target ? `已搜索 ${target}` : '已搜索资料';
  if (toolName === 'web_extract') return target ? `已读取 ${target}` : '已读取网页';
  if (toolName === 'list_skills' || toolName === 'read_skill' || toolName === 'install_skill_from_attachment' || toolName === 'delete_skill') return target ? `已处理 ${target}` : '已处理技能';
  return '已处理';
}

function normalizeToolStatusInput(input) {
  if (typeof input === 'string') {
    return { toolName: input, statusKind: '', target: '' };
  }
  return {
    toolName: input?.tool_name || input?.toolName || '',
    statusKind: input?.status_kind || input?.statusKind || '',
    target: String(input?.target_summary || input?.targetSummary || '').trim(),
  };
}

function pendingStatusByKind(statusKind = '', target = '') {
  switch (statusKind) {
    case 'workspace_search':
      return target ? `正在查看 ${target}` : '正在查看工作区';
    case 'workspace_read':
      return target ? `正在读取 ${target}` : '正在读取文件';
    case 'workspace_edit':
      return target ? `正在编辑 ${target}` : '正在编辑文件';
    case 'workspace_path':
      return target ? `正在整理 ${target}` : '正在整理文件';
    case 'command':
      return target ? `正在运行 ${target} 的命令` : '正在运行命令';
    case 'web':
      return target ? `正在搜索 ${target}` : '正在搜索资料';
    case 'skill':
      return target ? `正在处理 ${target}` : '正在处理技能';
    case 'task':
      return '正在启动后台任务';
    default:
      return '';
  }
}

function completedStatusByKind(statusKind = '', target = '') {
  switch (statusKind) {
    case 'workspace_search':
      return target ? `已查看 ${target}` : '已查看工作区';
    case 'workspace_read':
      return target ? `已读取 ${target}` : '已读取文件';
    case 'workspace_edit':
      return target ? `已编辑 ${target}` : '已编辑文件';
    case 'workspace_path':
      return target ? `已整理 ${target}` : '已整理文件';
    case 'command':
      return target ? `已运行 ${target} 的命令` : '已运行命令';
    case 'web':
      return target ? `已搜索 ${target}` : '已搜索资料';
    case 'skill':
      return target ? `已处理 ${target}` : '已处理技能';
    case 'task':
      return '已启动后台任务';
    default:
      return '';
  }
}

export function playfulPendingStatus(seed = '') {
  const text = String(seed || '');
  let hash = 0;
  for (let index = 0; index < text.length; index += 1) {
    hash = (hash * 31 + text.charCodeAt(index)) % PLAYFUL_PENDING_STATUSES.length;
  }
  return PLAYFUL_PENDING_STATUSES[hash] || PLAYFUL_PENDING_STATUSES[0];
}
