import { useEffect, useMemo, useRef, useState } from 'react';
import { API_BASE_URL, apiDelete, apiGet, apiPost } from '../api/client';

export default function useTasks(config = {}) {
  const [tasks, setTasks] = useState([]);
  const [isTaskPanelOpen, setIsTaskPanelOpen] = useState(false);
  const [selectedTaskId, setSelectedTaskId] = useState('');
  const [taskLogs, setTaskLogs] = useState([]);
  const [taskError, setTaskError] = useState('');
  const [hasUnreadFinishedTask, setHasUnreadFinishedTask] = useState(false);
  const eventSourceRef = useRef(null);
  const tasksRef = useRef([]);
  const configRef = useRef(config);

  useEffect(() => {
    configRef.current = config;
  }, [config]);

  const activeTaskCount = useMemo(
    () => tasks.filter((task) => task.status === 'running' || task.status === 'pending').length,
    [tasks]
  );
  const selectedTask = tasks.find((task) => task.id === selectedTaskId) || null;

  useEffect(() => {
    if (!config.enabled) return undefined;
    loadTasks();
    const timer = window.setInterval(loadTasks, 4000);
    return () => window.clearInterval(timer);
  }, [config.enabled]);

  useEffect(() => {
    eventSourceRef.current?.close();
    eventSourceRef.current = null;
    setTaskLogs([]);

    if (!config.enabled || !selectedTaskId) return undefined;
    if (selectedTask?.status === 'running' || selectedTask?.status === 'pending') {
      let disposed = false;
      let cursor = 0;
      let finalSeq = 0;
      let pendingTask = null;
      let reconnectTimer = null;
      const open = () => {
        if (disposed) return;
        const suffix = cursor ? `?after_seq=${encodeURIComponent(cursor)}` : '';
        const source = new EventSource(
          `${API_BASE_URL}/api/tasks/${encodeURIComponent(selectedTaskId)}/stream${suffix}`,
          { withCredentials: true }
        );
        eventSourceRef.current = source;
        source.addEventListener('stream_opened', (event) => {
          const payload = JSON.parse(event.data);
          finalSeq = Math.max(finalSeq, Number(payload.final_seq) || 0);
          if (payload.task) pendingTask = payload.task;
        });
        source.addEventListener('task', (event) => {
          const task = JSON.parse(event.data);
          finalSeq = Math.max(finalSeq, Number(task.final_seq) || 0);
          if (['done', 'failed', 'canceled'].includes(task.status)) pendingTask = task;
          else upsertTask(task);
        });
        source.addEventListener('log', (event) => {
          const log = JSON.parse(event.data);
          const seq = Number(log.seq) || 0;
          if (seq <= cursor) return;
          if (seq !== cursor + 1) {
            source.close();
            reconnectTimer = window.setTimeout(open, 50);
            return;
          }
          setTaskLogs((current) => appendLog(current, log));
          cursor = seq;
        });
        source.addEventListener('replay_complete', (event) => {
          const payload = JSON.parse(event.data);
          finalSeq = Math.max(finalSeq, Number(payload.final_seq) || 0);
          if (pendingTask && cursor >= finalSeq) {
            upsertTask(pendingTask);
            pendingTask = null;
          }
        });
        source.onerror = () => {
          source.close();
          if (disposed) return;
          setTaskError('任务日志连接中断，正在恢复');
          reconnectTimer = window.setTimeout(open, 500);
        };
      };
      open();

      return () => {
        disposed = true;
        if (reconnectTimer) window.clearTimeout(reconnectTimer);
        eventSourceRef.current?.close();
      };
    }

    loadTaskLogs(selectedTaskId);
    return undefined;
  }, [config.enabled, selectedTaskId, selectedTask?.status]);

  async function loadTasks() {
    if (!configRef.current.enabled) return;
    try {
      const data = await apiGet('/api/tasks');
      mergeTasks(data.items || []);
      setTaskError('');
    } catch (error) {
      console.error('loadTasks failed', error);
      setTaskError(error.message);
    }
  }

  async function loadTaskLogs(taskId) {
    if (!configRef.current.enabled || !taskId) return;
    try {
      const data = await apiGet(`/api/tasks/${encodeURIComponent(taskId)}/logs`);
      setTaskLogs(data.items || []);
      setTaskError('');
    } catch (error) {
      console.error('loadTaskLogs failed', error);
      setTaskError(error.message);
    }
  }

  async function cancelTask(taskId) {
    try {
      const task = await apiPost(`/api/tasks/${encodeURIComponent(taskId)}/cancel`, {});
      upsertTask(task);
      await loadTasks();
    } catch (error) {
      console.error('cancelTask failed', error);
      setTaskError(error.message);
    }
  }

  async function deleteTask(taskId) {
    try {
      await apiDelete(`/api/tasks/${encodeURIComponent(taskId)}`);
      tasksRef.current = tasksRef.current.filter((task) => task.id !== taskId);
      setTasks(tasksRef.current);
      if (selectedTaskId === taskId) {
        setSelectedTaskId('');
        setTaskLogs([]);
      }
      await loadTasks();
    } catch (error) {
      console.error('deleteTask failed', error);
      setTaskError(error.message);
    }
  }

  function upsertTask(task) {
    mergeTasks([task], { partial: true });
  }

  function openTaskPanel() {
    setHasUnreadFinishedTask(false);
    setIsTaskPanelOpen(true);
    loadTasks();
  }

  function mergeTasks(incoming, mergeOptions = {}) {
    const items = Array.isArray(incoming) ? incoming : [];
    const previous = tasksRef.current;
    const previousById = new Map(previous.map((task) => [task.id, task]));
    const next = mergeOptions.partial ? [...previous] : [...items];

    if (mergeOptions.partial) {
      for (const task of items) {
        const index = next.findIndex((item) => item.id === task.id);
        if (index === -1) next.unshift(task);
        else next[index] = task;
      }
    }

    const finished = next.filter((task) => {
      const before = previousById.get(task.id);
      return (
        before &&
        (before.status === 'running' || before.status === 'pending') &&
        ['done', 'failed', 'canceled'].includes(task.status)
      );
    });

    tasksRef.current = next;
    setTasks(next);

    if (finished.length > 0) {
      setHasUnreadFinishedTask(true);
      configRef.current.onFinished?.(finished);
    }
  }

  return {
    activeTaskCount,
    cancelTask,
    deleteTask,
    hasUnreadFinishedTask,
    isTaskPanelOpen,
    loadTasks,
    openTaskPanel,
    selectedTask,
    selectedTaskId,
    setIsTaskPanelOpen,
    setSelectedTaskId,
    taskError,
    taskLogs,
    tasks,
    upsertTask,
  };
}

function appendLog(logs, log) {
  if (logs.some((item) => item.id === log.id)) return logs;
  return [...logs, log].sort((a, b) => a.seq - b.seq);
}
