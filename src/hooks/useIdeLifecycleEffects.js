import { useEffect } from 'react';

export default function useIdeLifecycleEffects({
  isDarkMode,
  isEnabled,
  layoutNotice,
  loadConversations,
  loadTree,
  notify,
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
    notify?.(layoutNotice);
    setLayoutNotice('');
    return undefined;
  }, [layoutNotice, notify, setLayoutNotice]);
}
