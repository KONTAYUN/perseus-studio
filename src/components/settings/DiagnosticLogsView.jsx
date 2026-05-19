import React from 'react';
import { ArrowLeft, Clipboard, Download, RefreshCw, Trash2, Users, X } from 'lucide-react';
import { API_BASE_URL, apiDelete, apiGet } from '../../api/client';
import { Button, EmptyCard, SegmentedControl, SettingsSection, formatDate } from './SettingsUI';
import { diagnosticShortJudgement, formatRequestSummary } from './diagnosticLogUtils';

export default function DiagnosticLogsView({ dialog, onBack, currentUser }) {
  const isAdmin = currentUser?.role === 'admin';
  const [scope, setScope] = React.useState(isAdmin ? 'all' : 'mine');
  const [logs, setLogs] = React.useState([]);
  const [usersById, setUsersById] = React.useState({});
  const [message, setMessage] = React.useState('');
  const [isLoading, setIsLoading] = React.useState(false);
  const [expandedIds, setExpandedIds] = React.useState(new Set());
  const isAllUsersScope = isAdmin && scope === 'all';

  React.useEffect(() => {
    setScope(isAdmin ? 'all' : 'mine');
  }, [isAdmin]);

  const loadLogs = React.useCallback(async () => {
    setIsLoading(true);
    setMessage('');
    try {
      const data = await apiGet(isAllUsersScope ? '/api/admin/diagnostics/logs?limit=200' : '/api/diagnostics/logs?limit=100');
      setLogs(data.logs || []);
      setUsersById(isAllUsersScope ? data.users || {} : {});
    } catch (error) {
      setMessage(error.message);
    } finally {
      setIsLoading(false);
    }
  }, [isAllUsersScope]);

  React.useEffect(() => {
    loadLogs();
  }, [loadLogs]);

  const fetchExportText = async () => {
    const path = isAllUsersScope ? '/api/admin/diagnostics/export?limit=200' : '/api/diagnostics/export?limit=100';
    const response = await fetch(`${API_BASE_URL}${path}`, {
      credentials: 'include',
    });
    const text = await response.text();
    if (!response.ok) {
      throw new Error(text || '导出失败');
    }
    return text;
  };

  const copyLogs = async () => {
    try {
      const text = await fetchExportText();
      await navigator.clipboard.writeText(text);
      setMessage(isAllUsersScope ? '已复制全部用户最近诊断日志。' : '已复制最近诊断日志。');
    } catch (error) {
      setMessage(error.message);
    }
  };

  const exportLogs = async () => {
    try {
      const text = await fetchExportText();
      const blob = new Blob([text], { type: 'text/plain;charset=utf-8' });
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = isAllUsersScope ? 'perseus-diagnostic-logs-all-users.txt' : 'perseus-diagnostic-logs.txt';
      link.click();
      URL.revokeObjectURL(url);
      setMessage('已导出诊断日志。');
    } catch (error) {
      setMessage(error.message);
    }
  };

  const clearLogs = async () => {
    const ok = await dialog?.confirm?.({
      title: isAllUsersScope ? '清空全部诊断日志' : '清空诊断日志',
      message: isAllUsersScope
        ? '确认清空所有用户的诊断日志？此操作不可恢复，但不会影响会话、消息、workspace 文件或 run checkpoint。'
        : '确认清空自己的诊断日志？',
      confirmLabel: isAllUsersScope ? '清空全部日志' : '清空日志',
      tone: 'red',
    });
    if (!ok) return;
    try {
      await apiDelete(isAllUsersScope ? '/api/admin/diagnostics/logs' : '/api/diagnostics/logs');
      setLogs([]);
      setExpandedIds(new Set());
      setMessage(isAllUsersScope ? '全部用户诊断日志已清空。' : '诊断日志已清空。');
    } catch (error) {
      setMessage(error.message);
    }
  };

  const deleteLog = async (item) => {
    const ok = await dialog?.confirm?.({
      title: '删除这条诊断日志',
      message: `确认删除 ${item.id}？此操作不可恢复。`,
      confirmLabel: '删除',
      tone: 'red',
    });
    if (!ok) return;
    try {
      await apiDelete(`${isAllUsersScope ? '/api/admin/diagnostics/logs' : '/api/diagnostics/logs'}/${encodeURIComponent(item.id)}`);
      setLogs((current) => current.filter((entry) => entry.id !== item.id));
      setExpandedIds((current) => {
        const next = new Set(current);
        next.delete(item.id);
        return next;
      });
      setMessage('已删除该条诊断日志。');
    } catch (error) {
      setMessage(error.message);
    }
  };

  const toggleExpanded = (id) => {
    setExpandedIds((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  return (
    <SettingsSection data-testid="diagnostics-view" className="settings-view-enter">
      <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 flex-wrap items-center gap-3">
          <Button type="button" variant="ghost" size="sm" onClick={onBack}>
            <ArrowLeft size={16} /> 诊断日志
          </Button>
          {isAdmin && (
            <SegmentedControl
              value={scope}
              onChange={setScope}
              options={[
                { value: 'all', label: '全部用户' },
                { value: 'mine', label: '我的日志' },
              ]}
            />
          )}
        </div>
        <div className="flex flex-wrap gap-2">
          <Button type="button" variant="secondary" size="sm" onClick={loadLogs}>
            <RefreshCw size={14} /> 刷新
          </Button>
          <Button type="button" size="sm" onClick={copyLogs}>
            <Clipboard size={14} /> {isAllUsersScope ? '复制汇总' : '复制最近日志'}
          </Button>
          <Button type="button" size="sm" onClick={exportLogs}>
            <Download size={14} /> 导出日志
          </Button>
          <Button type="button" variant="danger" size="sm" onClick={clearLogs}>
            <Trash2 size={14} /> {isAllUsersScope ? '清空全部' : '清空日志'}
          </Button>
        </div>
      </div>

      {message && (
        <div className="settings-expand-enter mb-4 rounded-2xl border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-700 dark:border-blue-500/20 dark:bg-blue-500/10 dark:text-blue-200">
          {message}
        </div>
      )}

      {isLoading ? (
        <div className="settings-card rounded-2xl border border-gray-100 bg-white/70 p-6 text-center text-sm text-gray-400 dark:border-white/10 dark:bg-[#252526]/80">
          正在读取诊断日志...
        </div>
      ) : logs.length === 0 ? (
        <EmptyCard text={isAllUsersScope ? '还没有用户诊断日志。' : '还没有诊断日志。模型调用或工具执行出错时会自动记录最近摘要。'} />
      ) : (
        <div className="space-y-3">
          {logs.map((item) => {
            const expanded = expandedIds.has(item.id);
            const user = usersById[item.user_id];
            return (
              <div key={item.id} className="settings-card rounded-2xl border border-gray-100 bg-white/80 p-4 dark:border-white/10 dark:bg-[#252526]/80">
                <button type="button" onClick={() => toggleExpanded(item.id)} className="motion-row block w-full text-left">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <DiagnosticLevelPill level={item.level} />
                        <DiagnosticSummaryPill item={item} />
                        <span className="truncate text-sm font-bold text-gray-800 dark:text-gray-100">{item.stage || 'unknown'}</span>
                      </div>
                      <div className="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">
                        {[item.provider_name, item.model_id || item.model_name].filter(Boolean).join(' · ') || '未记录模型'}
                      </div>
                      {isAllUsersScope && (
                        <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                          <span className="inline-flex items-center gap-1 rounded-full bg-blue-50 px-2 py-1 font-bold text-blue-600 dark:bg-blue-500/10 dark:text-blue-200">
                            <Users size={12} /> {user?.username || item.user_id || '未知用户'}
                          </span>
                          {user?.role && (
                            <span className="rounded-full bg-gray-100 px-2 py-1 font-bold text-gray-500 dark:bg-white/10 dark:text-gray-300">
                              {user.role === 'admin' ? '管理员' : '用户'}
                            </span>
                          )}
                          {user?.disabled && (
                            <span className="rounded-full bg-red-50 px-2 py-1 font-bold text-red-500 dark:bg-red-500/10 dark:text-red-200">
                              已禁用
                            </span>
                          )}
                        </div>
                      )}
                      <div className="mt-1 line-clamp-2 text-xs text-gray-400">{item.error_message || '无错误摘要'}</div>
                    </div>
                    <div className="shrink-0 text-right text-xs text-gray-400">
                      <div>{formatDate(item.created_at)}</div>
                      {item.http_status > 0 && <div className="mt-1 font-bold text-red-500">HTTP {item.http_status}</div>}
                    </div>
                  </div>
                </button>

                {expanded && (
                  <div className="settings-expand-enter mt-4 border-t border-gray-100 pt-4 text-xs dark:border-white/10">
                    <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                      <div className="font-bold text-gray-500 dark:text-gray-300">{diagnosticShortJudgement(item)}</div>
                      <Button type="button" variant="danger" size="xs" onClick={() => deleteLog(item)}>
                        <X size={13} /> 删除单条
                      </Button>
                    </div>
                    <div className="grid gap-3 md:grid-cols-2">
                      {isAllUsersScope && <DiagnosticField label="user_id" value={item.user_id} />}
                      {isAllUsersScope && <DiagnosticField label="username" value={user?.username} />}
                      <DiagnosticField label="id" value={item.id} />
                      <DiagnosticField label="conversation_id" value={item.conversation_id} />
                      <DiagnosticField label="message_id" value={item.message_id} />
                      <DiagnosticField label="task_id" value={item.task_id} />
                      <DiagnosticField label="model_provider_id" value={item.model_provider_id} />
                      <DiagnosticField label="base_url_host" value={item.base_url_host} />
                      <DiagnosticField label="final_url_path" value={item.final_url_path} />
                      <DiagnosticField label="content_type" value={item.content_type} />
                      <DiagnosticField label="duration_ms" value={item.duration_ms ? String(item.duration_ms) : ''} />
                      <DiagnosticField label="error_message" value={item.error_message} wide />
                      <DiagnosticField label="response_snippet" value={item.response_snippet} wide mono />
                      <DiagnosticField label="request_summary" value={formatRequestSummary(item.request_summary)} wide mono />
                    </div>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </SettingsSection>
  );
}

function DiagnosticLevelPill({ level }) {
  const normalized = String(level || 'info').toLowerCase();
  const styles = {
    error: {
      label: '错误',
      dot: 'bg-red-500',
      text: 'text-red-600 dark:text-red-300',
      bg: 'bg-red-50 dark:bg-red-500/10',
    },
    warn: {
      label: '警告',
      dot: 'bg-amber-500',
      text: 'text-amber-600 dark:text-amber-300',
      bg: 'bg-amber-50 dark:bg-amber-500/10',
    },
    info: {
      label: '信息',
      dot: 'bg-blue-500',
      text: 'text-blue-600 dark:text-blue-300',
      bg: 'bg-blue-50 dark:bg-blue-500/10',
    },
  };
  const tone = styles[normalized] || styles.info;
  return (
    <span className={`inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[11px] font-bold ${tone.bg} ${tone.text}`}>
      <span className={`h-1.5 w-1.5 rounded-full ${tone.dot}`} />
      {tone.label}
    </span>
  );
}

function DiagnosticSummaryPill({ item }) {
  const label = diagnosticShortJudgement(item);
  if (!label) return null;
  return (
    <span className="rounded-full bg-gray-100 px-2 py-0.5 text-[11px] font-bold text-gray-500 dark:bg-white/10 dark:text-gray-300">
      {label}
    </span>
  );
}

function DiagnosticField({ label, value, wide = false, mono = false }) {
  if (value === undefined || value === null || value === '') return null;
  return (
    <div className={wide ? 'md:col-span-2' : ''}>
      <div className="mb-1 font-bold text-gray-400">{label}</div>
      <div className={`whitespace-pre-wrap break-words [overflow-wrap:anywhere] rounded-xl bg-gray-50 p-3 text-gray-700 dark:bg-black/20 dark:text-gray-300 ${mono ? 'font-mono' : ''}`}>
        {value}
      </div>
    </div>
  );
}
