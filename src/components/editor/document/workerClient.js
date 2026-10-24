export function createDocumentWorker() {
  const worker = new Worker(new URL('./session.worker.js', import.meta.url), { type: 'module' });
  const pending = new Map(); let sequence = 0, terminal;
  const fail = error => { terminal = error; worker.terminate(); for (const { reject, timer } of pending.values()) { clearTimeout(timer); reject(error); } pending.clear(); };
  worker.onmessage = ({ data }) => {
    const request = pending.get(data.id); if (!request) return;
    pending.delete(data.id); clearTimeout(request.timer);
    if (data.error) request.reject(Object.assign(new Error(data.error.message), { category: data.error.category }));
    else request.resolve(data.result);
  };
  worker.onerror = () => fail(new Error('文稿处理组件加载失败，请重试。原文件未修改。'));
  worker.onmessageerror = () => fail(new Error('文稿处理结果无法读取，原文件未修改。'));
  return {
    call(method, args = {}) { return new Promise((resolve, reject) => {
      if (terminal) { reject(terminal); return; }
      const id = ++sequence, timer = setTimeout(() => fail(new Error('文稿处理超过资源预算，原稿和编辑草稿已保留。')), 60_000);
      pending.set(id, { resolve, reject, timer });
      try { worker.postMessage({ id, method, args }); } catch (error) { fail(error); }
    }); },
    destroy() { worker.terminate(); fail(new Error('文稿编辑会话已关闭')); },
  };
}
