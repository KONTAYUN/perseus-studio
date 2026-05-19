import { useEffect } from 'react';

export default function useIdeLifecycleEffects({
  isDarkMode,
  isEnabled,
  layoutNotice,
  loadConversations,
  loadTree,
  setLayoutNotice,
}) {
  useEffect(() => {
    if (isDarkMode) document.documentElement.classList.add('dark');
    else document.documentElement.classList.remove('dark');
  }, [isDarkMode]);

  useEffect(() => {
    if (!isEnabled) return;
    loadTree();
    loadConversations();
  }, [isEnabled]);

  useEffect(() => {
    if (!layoutNotice) return undefined;
    const timer = window.setTimeout(() => setLayoutNotice(''), 1800);
    return () => window.clearTimeout(timer);
  }, [layoutNotice]);
}
