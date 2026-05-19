export function parseRequestSummary(value) {
  if (!value) return null;
  try {
    return JSON.parse(value);
  } catch {
    return null;
  }
}

export function formatRequestSummary(value) {
  const parsed = parseRequestSummary(value);
  if (!parsed) return value || '';
  return JSON.stringify(parsed, null, 2);
}

export function diagnosticShortJudgement(item) {
  const summary = parseRequestSummary(item?.request_summary);
  const failureClass = summary?.failure_class || '';
  if (failureClass === 'provider_rejected_required_tool_choice') return '上游拒绝强制工具调用';
  if (failureClass === 'provider_bad_request_with_tools') return '上游拒绝工具调用请求';
  if (failureClass === 'provider_bad_request_without_tools') return '上游拒绝普通请求';
  if (failureClass === 'user_or_client_canceled') return '用户或浏览器取消请求';
  if (failureClass === 'provider_transient_http') return '上游临时 HTTP 故障';
  if (failureClass === 'provider_transient_transport') return '上游连接临时中断';
  if (failureClass === 'provider_http_error') return '上游 HTTP 错误';
  if (item?.http_status === 400 && Number(summary?.tools_count || 0) > 0) return '上游拒绝工具请求';
  if (String(item?.error_message || '').toLowerCase().includes('context canceled')) return '用户或浏览器取消请求';
  return '';
}
