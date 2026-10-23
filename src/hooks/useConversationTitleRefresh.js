import { useEffect, useRef } from 'react';

export function createConversationTitleRefresh({ refresh, schedule = setTimeout, clear = clearTimeout }) {
  let owner = null;
  const stop = () => {
    if (!owner) return;
    owner.controller.abort();
    clear(owner.timer);
    owner = null;
  };
  const start = (conversationId) => {
    stop();
    const current = { controller: new AbortController(), timer: null, attempt: 0 };
    owner = current;
    const delays = [1000, 3000, 6000, 10000, 15000, 15000];
    const tick = async () => {
      if (owner !== current) return;
      try {
        const items = await refresh({ signal: current.controller.signal, quiet: true });
        if (owner !== current) return;
        const conversation = items?.find((item) => item.id === conversationId);
        if (conversation?.title && conversation.title !== '新对话') { owner = null; return; }
      } catch {
        if (owner !== current) return;
      }
      current.attempt++;
      if (current.attempt >= delays.length) { owner = null; return; }
      current.timer = schedule(tick, delays[current.attempt]);
    };
    current.timer = schedule(tick, delays[0]);
  };
  return { start, stop };
}

export default function useConversationTitleRefresh(loadConversations) {
  const loader = useRef(loadConversations);
  const refresh = useRef(null);
  useEffect(() => { loader.current = loadConversations; });
  useEffect(() => {
    const current = createConversationTitleRefresh({ refresh: (options) => loader.current(options) });
    refresh.current = current;
    return () => { current.stop(); refresh.current = null; };
  }, []);
  return (conversationId) => refresh.current?.start(conversationId);
}
