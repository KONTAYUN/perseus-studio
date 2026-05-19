export function formatChatError(error) {
  const base = error?.message || String(error || '未知错误');
  if (error?.diagnosticLogId && !base.includes('诊断日志 ID')) {
    return `${base}\n已记录诊断日志 ID：${error.diagnosticLogId}，可在 设置 → 诊断日志 中导出。`;
  }
  return base;
}
