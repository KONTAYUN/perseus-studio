import { useEffect, useRef, useState } from 'react';
import { apiGet, apiPost } from '../api/client.js';

export default function useWorkspaceChangeReview(toolCallId, index, onRefresh) {
  const [preview, setPreview] = useState(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const alive = useRef(true);
  const readController = useRef(null);
  useEffect(() => {
    alive.current = true;
    return () => { alive.current = false; readController.current?.abort(); };
  }, []);

  async function load() {
    readController.current?.abort();
    const controller = new AbortController();
    readController.current = controller;
    setBusy(true); setError('');
    try {
      const data = await apiGet(`/api/workspace/changes?tool_call_id=${encodeURIComponent(toolCallId)}&index=${index}`, { signal: controller.signal });
      if (alive.current && readController.current === controller) setPreview(data);
    } catch (err) {
      if (alive.current && err.name !== 'AbortError') setError(err.message);
    } finally {
      if (alive.current && readController.current === controller) setBusy(false);
    }
  }

  async function revert() {
    setBusy(true); setError('');
    try {
      const event = await apiPost('/api/workspace/changes', { tool_call_id: toolCallId, index });
      if (alive.current) setPreview((current) => ({ ...current, can_revert: false, revert_state: 'completed', reason: '' }));
      try { await onRefresh?.([event]); }
      catch { if (alive.current) setError('已撤销，编辑器刷新失败，请重新打开文件。'); }
    } catch (err) {
      if (alive.current) setError(err.message);
    } finally {
      if (alive.current) setBusy(false);
    }
  }

  return { preview, busy, error, load, revert };
}
