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
  if (failureClass === 'provider_rejected_required_tool_choice') return '上游明确拒绝 required 工具调用';
  if (failureClass === 'provider_rejected_unsupported_model_params') return '上游拒绝不兼容模型参数';
  if (failureClass === 'provider_bad_request_with_reasoning') return '上游拒绝推理参数请求';
  if (failureClass === 'provider_incomplete_tool_arguments') return '上游工具参数流被截断';
  if (failureClass === 'provider_bad_request_with_tools') return '上游拒绝工具调用请求';
  if (failureClass === 'provider_bad_request_without_tools') return '上游拒绝普通请求';
  if (failureClass === 'user_or_client_canceled') return '用户或浏览器取消请求';
  if (failureClass === 'provider_transient_http') return '上游临时 HTTP 故障';
  if (failureClass === 'provider_transient_transport') return '上游连接临时中断';
  if (failureClass === 'provider_transport_eof') return '上游连接提前断开';
  if (failureClass === 'provider_connection_interrupted') return '上游连接中断';
  if (failureClass === 'provider_connection_refused') return '上游连接被拒绝';
  if (failureClass === 'provider_transport_timeout') return '上游连接超时';
  if (failureClass === 'provider_tls_interrupted') return '上游 TLS 连接中断';
  if (failureClass === 'provider_transport_protocol_error') return '上游连接协议异常';
  if (failureClass === 'provider_dns_lookup_failed') return '上游域名解析失败';
  if (failureClass === 'provider_http_error') return '上游 HTTP 错误';
  if (item?.http_status === 400 && Number(summary?.tools_count || 0) > 0) return '上游拒绝工具请求';
  if (String(item?.error_message || '').toLowerCase().includes('context canceled')) return '用户或浏览器取消请求';
  return '';
}
