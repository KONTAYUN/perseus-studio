import React from 'react';
import { Activity, Trash2, X } from 'lucide-react';
import useMotionPresence from '../../hooks/useMotionPresence';
import { Badge, Button, IconButton } from '../ui';

export default function TaskPanel({ tasks }) {
  const { closeWithPresence, exiting, present } = useMotionPresence(tasks.isTaskPanelOpen);
  const closePanel = React.useCallback(() => {
    closeWithPresence(() => tasks.setIsTaskPanelOpen(false));
  }, [closeWithPresence, tasks]);

  if (!present) return null;

  return (
    <div
      className={`motion-fade-reveal fixed inset-0 z-[140] bg-black/30 backdrop-blur-sm ${
        exiting ? 'opacity-0' : 'animate-in fade-in opacity-100'
      }`}
      onClick={closePanel}
    >
      <aside
        className={`ui-overlay-surface motion-modal absolute right-4 top-16 bottom-4 flex w-[min(920px,calc(100vw-2rem))] flex-col overflow-hidden border border-gray-200 bg-white dark:border-white/10 dark:bg-[#1C1C1E] ${
          exiting ? 'motion-presence-exit' : ''
        }`}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex h-14 items-center justify-between border-b border-gray-100 px-4 dark:border-white/10">
          <div className="flex items-center gap-2">
            <Activity size={17} className="text-blue-500" />
            <div>
              <div className="text-sm font-bold text-gray-800 dark:text-gray-100">任务</div>
              <div className="text-[11px] text-gray-400">{tasks.activeTaskCount} 个进行中</div>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <IconButton
              onClick={closePanel}
              title="关闭任务面板"
            >
              <X size={16} />
            </IconButton>
          </div>
        </div>

        {tasks.taskError && (
          <div className="mx-4 mt-3 rounded-xl border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-600 dark:border-red-500/20 dark:bg-red-500/10 dark:text-red-300">
            {tasks.taskError}
          </div>
        )}

        <div className="grid min-h-0 flex-1 grid-cols-[minmax(240px,280px)_minmax(0,1fr)]">
          <div className="min-h-0 overflow-y-auto border-r border-gray-100 p-3 custom-scrollbar dark:border-white/10">
            {tasks.tasks.length === 0 ? (
              <div className="p-4 text-sm text-gray-400">暂无任务</div>
            ) : (
              tasks.tasks.map((task) => {
                const canDelete = task.status !== 'running' && task.status !== 'pending';
                return (
                <div
                  key={task.id}
                  onClick={() => tasks.setSelectedTaskId(task.id)}
                  role="button"
                  tabIndex={0}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault();
                      tasks.setSelectedTaskId(task.id);
                    }
                  }}
                  className={`motion-row task-item-row group/task relative mb-2 grid w-full grid-cols-[minmax(0,1fr)_64px] gap-2 overflow-hidden rounded-xl border px-3 py-2 text-left ${
                    tasks.selectedTaskId === task.id
                      ? 'border-blue-300 bg-blue-50 dark:border-blue-500/40 dark:bg-blue-500/10'
                      : 'border-gray-100 bg-gray-50/70 hover:bg-gray-100 dark:border-white/10 dark:bg-white/[0.03] dark:hover:bg-white/[0.06]'
                  }`}
                >
                  <div className="min-w-0">
                    <div className="truncate text-sm font-bold text-gray-800 dark:text-gray-100">{task.title}</div>
                    <div className="mt-1 truncate text-[11px] text-gray-400">{task.type}</div>
                    <div className="mt-2 text-[11px] text-gray-500 dark:text-gray-400">
                      <div className="truncate">{task.progress_stage || 'idle'}</div>
                      <div className="truncate">{task.progress_message || '-'}</div>
                      <div>{formatDate(task.created_at)}</div>
                    </div>
                  </div>
                  <div className="flex h-full min-w-0 flex-col items-end justify-between gap-2 overflow-hidden">
                    <StatusBadge status={task.status} />
                    {canDelete ? (
                      <IconButton
                        onClick={(e) => {
                          e.stopPropagation();
                          tasks.deleteTask(task.id);
                        }}
                        variant="danger"
                        size="xs"
                        className="h-7 w-7 opacity-0 group-hover/task:opacity-100 focus:opacity-100"
                        title="删除任务记录"
                      >
                        <Trash2 size={13} />
                      </IconButton>
                    ) : (
                      <span className="h-7 w-7" />
                    )}
                  </div>
                </div>
                );
              })
            )}
          </div>

          <div className="min-w-0 min-h-0 p-4">
            {!tasks.selectedTask ? (
              <div className="flex h-full items-center justify-center text-sm text-gray-400">选择一个任务查看日志</div>
            ) : (
              <TaskDetail tasks={tasks} />
            )}
          </div>
        </div>
      </aside>
    </div>
  );
}

function TaskDetail({ tasks }) {
  const task = tasks.selectedTask;
  const canCancel = task.status === 'running' || task.status === 'pending';

  return (
    <div className="flex h-full min-w-0 min-h-0 flex-col overflow-hidden">
      <div className="grid min-w-0 grid-cols-[minmax(0,1fr)_76px] items-start gap-3">
        <div className="min-w-0 overflow-hidden">
          <div className="truncate text-base font-bold text-gray-800 dark:text-gray-100">{task.title}</div>
          <div className="mt-1 flex flex-wrap gap-2 text-[11px] text-gray-400">
            <span>{task.type}</span>
            <span>{task.progress_stage || 'idle'}</span>
            <span>created {formatDate(task.created_at)}</span>
            <span>updated {formatDate(task.finished_at || task.started_at || task.created_at)}</span>
          </div>
          <div className="mt-1 text-xs text-gray-500 dark:text-gray-400">{task.progress_message || '-'}</div>
        </div>
        <div className="flex min-w-0 flex-col items-end gap-2 overflow-hidden">
          <StatusBadge status={task.status} />
          {canCancel && (
            <Button
              onClick={() => tasks.cancelTask(task.id)}
              variant="dangerSubtle"
              size="xs"
              fullWidth
            >
              取消
            </Button>
          )}
          {!canCancel && (
            <IconButton
              onClick={() => tasks.deleteTask(task.id)}
              variant="danger"
              title="删除任务记录"
            >
              <Trash2 size={13} />
            </IconButton>
          )}
        </div>
      </div>

      <div className="motion-card mt-4 min-h-0 min-w-0 flex-1 overflow-hidden rounded-xl border border-gray-100 bg-gray-950 dark:border-white/10">
        <div className="h-full w-full overflow-y-auto overflow-x-hidden p-3 font-mono text-[12px] text-gray-200 custom-scrollbar">
        {tasks.taskLogs.length === 0 ? (
          <div className="text-gray-500">暂无日志</div>
        ) : (
          tasks.taskLogs.map((log) => (
            <div key={log.id || `${log.task_id}-${log.seq}`} className="mb-1 whitespace-pre-wrap break-words [overflow-wrap:anywhere]">
              <span className="text-gray-500">{formatTime(log.created_at)}</span>{' '}
              <span className={log.level === 'error' ? 'text-red-300' : log.level === 'warn' ? 'text-amber-300' : 'text-blue-300'}>
                [{log.level}]
              </span>{' '}
              <span>{log.message}</span>
            </div>
          ))
        )}
        </div>
      </div>
    </div>
  );
}

function StatusBadge({ status }) {
  const tone =
    status === 'done'
      ? 'green'
      : status === 'failed'
      ? 'red'
      : status === 'canceled'
      ? 'gray'
      : 'blue';
  return <Badge tone={tone}>{status}</Badge>;
}

function formatDate(value) {
  if (!value) return '-';
  return new Date(value).toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  });
}

function formatTime(value) {
  if (!value) return '--:--:--';
  return new Date(value).toLocaleTimeString('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}
