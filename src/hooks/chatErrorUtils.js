export function formatChatError(error) {
  const base = error?.message || String(error || '未知错误');
  if (error?.diagnosticLogId && !base.includes('诊断日志 ID')) {
    return `${base}\n已记录诊断日志 ID：${error.diagnosticLogId}，可在 设置 → 诊断日志 中导出。`;
  }
  return base;
}

export function chatFailureMetadata(source = {}) {
  const retryAfterMs = Number(source.retryAfterMs ?? source.retry_after_ms ?? 0) || 0;
  const canResumeValue = source.canResumeNow ?? source.can_resume_now;
  return {
    failureClass: source.failureClass || source.failure_class || '',
    retryAfterMs,
    resetAt: source.resetAt || source.reset_at || '',
    providerId: source.providerId || source.provider_id || '',
    modelId: source.modelId || source.model_id || '',
    canResumeNow: canResumeValue === undefined ? true : canResumeValue !== false,
    recommendedAction: source.recommendedAction || source.recommended_action || '',
  };
}

export function applyChatFailureMetadata(target = {}, source = {}) {
  return {
    ...target,
    ...chatFailureMetadata(source),
  };
}

export function isResumeBlockedByFailure(item = {}) {
  if (!item) return false;
  if (item.canResumeNow === false) return true;
  return Number(item.retryAfterMs || 0) > 0 && isProviderRateLimitFailure(item.failureClass);
}

export function isProviderRateLimitFailure(failureClass = '') {
  const value = String(failureClass || '');
  return value === 'provider_rate_limited' || /^provider_(tpm|rpm|quota|unknown)_rate_limited$/.test(value);
}

export function formatRetryAfterLabel(retryAfterMs = 0, resetAt = '') {
  const ms = Number(retryAfterMs) || 0;
  if (ms > 0) {
    const seconds = Math.max(1, Math.ceil(ms / 1000));
    if (seconds < 60) return `${seconds} 秒后可继续`;
    return `${Math.ceil(seconds / 60)} 分钟后可继续`;
  }
  if (resetAt) return '冷却结束后可继续';
  return '稍后继续';
}
