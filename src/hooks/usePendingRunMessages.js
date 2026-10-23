import { useCallback, useEffect, useRef, useState } from 'react';
import { apiDelete, apiGet, apiPost } from '../api/client.js';

// The server consumes pending inputs atomically with run admission. React only
// caches this account's queue; refreshing never removes an unaccepted input.
export default function usePendingRunMessages({ userId, currentConversationId, currentConversationIdRef, setRunSteerNotice }) {
  const [cache, setCache] = useState({ userId: '', items: [] });
  const cacheRef = useRef(cache);
  const scopeRef = useRef(userId);
  const revision = useRef(0);
  scopeRef.current = userId;
  const items = cache.userId === userId ? cache.items : [];
  const pendingRunMessage = items.find(item => item.conversationId === currentConversationId) || null;

  const updateCache = useCallback(update => {
    if (scopeRef.current !== userId) return;
    const current = cacheRef.current.userId === userId ? cacheRef.current.items : [];
    const next = { userId, items: update(current) };
    cacheRef.current = next;
    setCache(next);
  }, [userId]);

  const refreshPendingMessages = useCallback(async (signal) => {
    if (!userId) return;
    const started = revision.current;
    const data = await apiGet('/api/pending-messages', { signal });
    if (!signal?.aborted && started === revision.current) updateCache(() => data.items || []);
  }, [userId, updateCache]);

  useEffect(() => {
    if (!userId) return undefined;
    const controller = new AbortController();
    let timer;
    const poll = async () => {
      try { await refreshPendingMessages(controller.signal); }
      catch {
        if (controller.signal.aborted) return;
        setRunSteerNotice(current => current?.status === 'error' ? current : {
          id: 'pending-load-error', status: 'error', conversationId: currentConversationIdRef.current,
          text: '暂时无法读取排队消息，连接恢复后重试。',
        });
      }
      if (!controller.signal.aborted) timer = window.setTimeout(poll, 2000);
    };
    poll();
    return () => { controller.abort(); window.clearTimeout(timer); };
  }, [userId, refreshPendingMessages, setRunSteerNotice, currentConversationIdRef]);

  const setPendingRunMessage = useCallback(async update => {
    if (!userId || scopeRef.current !== userId) return false;
    const conversationId = currentConversationIdRef.current;
    const current = (cacheRef.current.userId === userId ? cacheRef.current.items : []).find(item => item.conversationId === conversationId) || null;
    const next = typeof update === 'function' ? update(current) : update;
    if (next === current) return true;
    revision.current += 1;
    try {
      if (next) {
        const saved = await apiPost('/api/pending-messages', next);
        updateCache(items => [...items.filter(item => item.conversationId !== saved.conversationId), saved]);
      } else if (current) {
        await apiDelete(`/api/pending-messages/${encodeURIComponent(current.id)}`);
        updateCache(items => items.filter(item => item.id !== current.id));
      }
      return true;
    } catch (error) {
      if (scopeRef.current === userId) setRunSteerNotice({
        id: 'pending-save-error', status: 'error', conversationId: next?.conversationId || conversationId,
        text: next ? '排队消息未能保存，原输入已保留，请检查连接后重试。'
          : error.status === 409 ? error.message : '排队消息未能删除，请检查连接后重试。',
      });
      return false;
    } finally { revision.current += 1; }
  }, [userId, updateCache, currentConversationIdRef, setRunSteerNotice]);

  return { pendingRunMessage, pendingRunMessages: items, setPendingRunMessage, refreshPendingMessages };
}

export function useDurablePendingMessageAutoSend({ userId, pendingRunMessages, isChatSending, sendChatPayloadRef, refreshPendingMessages }) {
  const latest = useRef(null);
  const inFlight = useRef(false);
  const retry = useRef(new Map());
  latest.current = { userId, pendingRunMessages, isChatSending, refreshPendingMessages };
  useEffect(() => {
    if (!userId) return undefined;
    let disposed = false;
    let timer;
    const poll = async () => {
      const state = latest.current;
      const ids = new Set(state.pendingRunMessages.map(item => item.id));
      for (const id of retry.current.keys()) if (!ids.has(id)) retry.current.delete(id);
      const next = state.pendingRunMessages.find(item => (!item.latestStatus || item.latestStatus === 'succeeded') && (retry.current.get(item.id)?.at || 0) <= Date.now());
      if (state.userId === userId && next && !state.isChatSending && !inFlight.current) {
        inFlight.current = true;
        try {
          const accepted = await sendChatPayloadRef.current?.({ ...next, pendingMessageId: next.id, attachments: [], fileReferences: [], skillReferences: [], currentFileContext: null });
          if (!accepted) {
            const delay = Math.min((retry.current.get(next.id)?.delay || 2500) * 2, 30000);
            retry.current.set(next.id, { delay, at: Date.now() + delay });
          } else retry.current.delete(next.id);
          if (!disposed) await latest.current.refreshPendingMessages();
        } catch { /* The send action reports errors; the server retains unaccepted inputs. */ }
        finally { inFlight.current = false; }
      }
      if (!disposed) timer = window.setTimeout(poll, 2000);
    };
    poll();
    return () => { disposed = true; window.clearTimeout(timer); };
  }, [userId, sendChatPayloadRef]);
}
