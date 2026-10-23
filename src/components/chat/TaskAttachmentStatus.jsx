import { useEffect, useState } from 'react';
import { apiGet } from '../../api/client';

const labels = { pending: '等待执行', running: '执行中', done: '已完成', failed: '失败', canceled: '已取消' };

export default function TaskAttachmentStatus({ taskId, tasks = [] }) {
  const live = tasks.find((task) => task.id === taskId);
  const [fetched, setFetched] = useState(null);
  useEffect(() => {
    if (live || !taskId) return undefined;
    const controller = new AbortController();
    let timer;
    async function refresh() {
      try {
        const task = await apiGet(`/api/tasks/${encodeURIComponent(taskId)}`, { signal: controller.signal });
        if (controller.signal.aborted) return;
        setFetched({ taskId, status: task.status });
        if (task.status === 'pending' || task.status === 'running') timer = setTimeout(refresh, 4000);
      } catch (error) {
        if (controller.signal.aborted) return;
        setFetched({ taskId, error: error.status === 404 ? '任务已删除或不可访问' : '状态暂时不可用' });
        if (error.status !== 404) timer = setTimeout(refresh, 4000);
      }
    }
    refresh();
    return () => { controller.abort(); clearTimeout(timer); };
  }, [taskId, Boolean(live)]);
  const current = live || (fetched?.taskId === taskId ? fetched : null);
  return <span>状态：{current?.error || labels[current?.status] || current?.status || '正在获取'}</span>;
}
