import React from 'react';
import { ArrowLeft, Clipboard, Download, RefreshCw, Trash2, Users } from 'lucide-react';
import { API_BASE_URL, apiDelete, apiGet } from '../../api/client';
import { Badge, EmptyCard, formatDate } from './SettingsUI';

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
    if (isAllUsersScope) return;
    const ok = await dialog?.confirm?.({
      title: '清空诊断日志',
      message: '确认清空自己的诊断日志？',
      confirmLabel: '清空日志',
      tone: 'red',
    });
    if (!ok) return;
    try {
      await apiDelete('/api/diagnostics/logs');
      setLogs([]);
      setExpandedIds(new Set());
      setMessage('诊断日志已清空。');
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
    <section data-testid="diagnostics-view" className="settings-view-enter rounded-[24px] border border-gray-100 bg-white/55 p-5 dark:border-white/10 dark:bg-white/[0.03]">
      <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
        <button type="button" onClick={onBack} className="flex items-center gap-2 text-sm font-bold text-gray-500 hover:text-blue-500">
          <ArrowLeft size={16} /> 诊断日志
        </button>
        {isAdmin && (
          <div className="inline-flex rounded-[16px] border border-white/60 bg-white/55 p-1 shadow-sm backdrop-blur-xl dark:border-white/10 dark:bg-white/[0.06]">
            <ScopeButton active={isAllUsersScope} onClick={() => setScope('all')}>
              全部用户
            </ScopeButton>
            <ScopeButton active={!isAllUsersScope} onClick={() => setScope('mine')}>
              我的日志
            </ScopeButton>
          </div>
        )}
        <div className="flex flex-wrap gap-2">
          <button type="button" onClick={loadLogs} className="flex items-center gap-2 rounded-xl bg-gray-100 px-3 py-2 text-xs font-bold text-gray-600 hover:bg-gray-200 dark:bg-white/10 dark:text-gray-300 dark:hover:bg-white/15">
            <RefreshCw size={14} /> 刷新
          </button>
          <button type="button" onClick={copyLogs} className="flex items-center gap-2 rounded-xl bg-blue-500 px-3 py-2 text-xs font-bold text-white hover:bg-blue-600">
            <Clipboard size={14} /> {isAllUsersScope ? '复制汇总' : '复制最近日志'}
          </button>
          <button type="button" onClick={exportLogs} className="flex items-center gap-2 rounded-xl bg-blue-500 px-3 py-2 text-xs font-bold text-white hover:bg-blue-600">
            <Download size={14} /> 导出日志
          </button>
          {!isAllUsersScope && (
            <button type="button" onClick={clearLogs} className="flex items-center gap-2 rounded-xl bg-red-500 px-3 py-2 text-xs font-bold text-white hover:bg-red-600">
              <Trash2 size={14} /> 清空日志
            </button>
          )}
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
                <button type="button" onClick={() => toggleExpanded(item.id)} className="block w-full text-left">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge tone={item.level === 'error' ? 'red' : item.level === 'warn' ? 'yellow' : 'blue'}>{item.level}</Badge>
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
                  <div className="settings-expand-enter mt-4 grid gap-3 border-t border-gray-100 pt-4 text-xs dark:border-white/10 md:grid-cols-2">
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
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </section>
  );
}

function ScopeButton({ active, onClick, children }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`rounded-[12px] px-3 py-1.5 text-xs font-bold transition-all duration-200 ${
        active
          ? 'bg-white text-blue-600 shadow-sm dark:bg-white/15 dark:text-blue-200'
          : 'text-gray-500 hover:text-blue-500 dark:text-gray-400 dark:hover:text-blue-200'
      }`}
    >
      {children}
    </button>
  );
}

function DiagnosticField({ label, value, wide = false, mono = false }) {
  if (value === undefined || value === null || value === '') return null;
  return (
    <div className={wide ? 'md:col-span-2' : ''}>
      <div className="mb-1 font-bold text-gray-400">{label}</div>
      <div className={`whitespace-pre-wrap break-words rounded-xl bg-gray-50 p-3 text-gray-700 dark:bg-black/20 dark:text-gray-300 ${mono ? 'font-mono' : ''}`}>
        {value}
      </div>
    </div>
  );
}
