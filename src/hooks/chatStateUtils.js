const PLAYFUL_PENDING_STATUSES = ['正在烹饪', '正在裁剪', '正在编写大纲', '正在绘制图纸', '正在打磨细节', '正在铺开画布'];
const EXTRA_PENDING_STATUSES = ['正在翻阅底稿', '正在装订成册', '正在检查火候', '正在归档素材', '正在搜罗线索', '正在摘录资料', '正在准备工具箱', '正在重新调味'];
const LOADING_DISPLAY_MIN_MS = 3000;
const LOADING_BACKSPACE_MS = 28;
const LOADING_TYPE_MS = 32;
const DISPLAY_STATUS_POOL = [...PLAYFUL_PENDING_STATUSES, ...EXTRA_PENDING_STATUSES];

export function buildAssistantRunShell({ id, conversationId, createdAt, content = '', attachments = [] } = {}) {
  return {
    id,
    conversation_id: conversationId || 'pending',
    role: 'assistant',
    content,
    attachments,
    pending: true,
    created_at: createdAt || new Date().toISOString(),
    progressItems: [],
    loadingState: {
      text: PLAYFUL_PENDING_STATUSES[0],
      visibleText: PLAYFUL_PENDING_STATUSES[0],
      stage: 'holding',
      activeSince: Date.now(),
    },
    activeMessageId: '',
  };
}

export function appendUniqueProgressItem(items, nextItem) {
  if (!nextItem?.id) return items;
  if ((items || []).some((item) => item.id === nextItem.id)) return items;
  return [...(items || []), nextItem];
}

export function nextLoadingStatus(seed = 0) {
  return DISPLAY_STATUS_POOL[seed % DISPLAY_STATUS_POOL.length] || DISPLAY_STATUS_POOL[0];
}

export function buildProgressItem(type, payload = {}, fallbackText = '') {
  const text = agentEventText(type, payload) || payload.message || fallbackText || '';
  if (!text) return null;
  return {
    id: payload.event_id || `${type}:${payload.tool_call_id || payload.round || payload.path || payload.message || payload.reason || text}`,
    type,
    text,
    toolName: payload.tool_name || '',
    status: payload.status || '',
  };
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

export function normalizeConversationMessage(message) {
  if (!message) return message;
  const next = {
    attachments: [],
    ...message,
  };
  if (next.role === 'assistant' && next.status === 'streaming') {
    next.pending = true;
    next.activeMessageId = next.id;
    next.loadingState = next.loadingState || {
      text: nextLoadingStatus(0),
      visibleText: nextLoadingStatus(0),
      stage: 'holding',
      activeSince: Date.now(),
    };
  }
  return next;
}

export function normalizeConversationMessages(items) {
  return (items || []).map((item) => normalizeConversationMessage(item));
}

export function hasStreamingAssistantMessage(items) {
  return (items || []).some((item) => item?.role === 'assistant' && (item?.status === 'streaming' || item?.activeMessageId));
}

export function mapPendingAssistant(current, thinkingId, transform) {
  let updated = false;
  const next = current.map((item) => {
    const isPendingTarget =
      item.id === thinkingId ||
      (item.role === 'assistant' && item.pending);
    if (!isPendingTarget || updated) return item;
    updated = true;
    return transform(item);
  });
  return next;
}

export function agentEventText(type, payload = {}) {
  switch (type) {
    case 'model_call_started':
      return playfulPendingStatus(type);
    case 'model_call_attempt_started':
      return playfulPendingStatus(type);
    case 'model_call_retrying':
      return playfulPendingStatus(type);
    case 'model_call_attempt_completed':
      return playfulPendingStatus(type);
    case 'model_call_attempt_failed':
      return playfulPendingStatus(type);
    case 'model_stream_recovering':
      return playfulPendingStatus(type);
    case 'tool_call_started':
      return friendlyToolStatus(payload.tool_name, 'started');
    case 'tool_call_completed':
      return friendlyToolStatus(payload.tool_name, 'completed');
    case 'tool_call_failed':
      return '正在重新调味';
    case 'tool_result_returned':
      return payload.is_error ? '正在重新调味' : '';
    case 'completion_rejected':
      return '正在检查火候';
    case 'model_message_started':
      return playfulPendingStatus(type);
    case 'model_delta':
      return '';
    case 'model_message_completed':
      return '';
    case 'diagnostic_created':
      return playfulPendingStatus(type);
    case 'run_started':
      return playfulPendingStatus(type);
    case 'run_completed':
      return '执行完成';
    default:
      return '';
  }
}

export function friendlyToolStatus(toolName, phase) {
  if (toolName === 'inspect_workspace') return '正在铺开画布';
  if (toolName === 'read_workspace_text') return '正在翻阅底稿';
  if (toolName === 'save_workspace_file') return phase === 'completed' ? '正在检查火候' : '正在装订成册';
  if (toolName === 'manage_workspace_path') return phase === 'completed' ? '正在检查火候' : '正在裁剪';
  if (toolName === 'organize_workspace_matches') return phase === 'completed' ? '正在检查火候' : '正在归档素材';
  if (toolName === 'run_workspace_command') return '正在测算';
  if (toolName === 'perseus_search_web' || toolName === 'web_search') return '正在搜罗线索';
  if (toolName === 'web_extract') return '正在摘录资料';
  if (toolName === 'list_skills' || toolName === 'read_skill' || toolName === 'install_skill_from_attachment') return '正在准备工具箱';
  return playfulPendingStatus(toolName || 'default');
}

export function playfulPendingStatus(seed = '') {
  const text = String(seed || '');
  let hash = 0;
  for (let index = 0; index < text.length; index += 1) {
    hash = (hash * 31 + text.charCodeAt(index)) % PLAYFUL_PENDING_STATUSES.length;
  }
  return PLAYFUL_PENDING_STATUSES[hash] || PLAYFUL_PENDING_STATUSES[0];
}

export function isPendingStatusText(text = '') {
  const value = text.trim();
  if (!value) return true;
  if (value === '已开始执行') return true;
  if (PLAYFUL_PENDING_STATUSES.includes(value) || EXTRA_PENDING_STATUSES.includes(value)) return true;
  if (value.startsWith('正在') && value.length <= 12) return true;
  return false;
}
