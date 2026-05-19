import { useEffect, useState } from 'react';
import { apiPost } from '../api/client';
import { getAncestorFolderPaths } from './ideUtils';

export function useFileSearchState({
  openFile,
  setIsAddMenuOpen,
  setIsTrashPanelOpen,
  setLastSelectedPath,
  setOpenFolders,
  setSelectedItemPath,
  setSelectedItemType,
  setSelectedPaths,
}) {
  const [isFileSearchOpen, setIsFileSearchOpen] = useState(false);
  const [fileSearchQuery, setFileSearchQuery] = useState('');
  const [fileSearchResults, setFileSearchResults] = useState([]);
  const [isFileSearchLoading, setIsFileSearchLoading] = useState(false);
  const [fileSearchError, setFileSearchError] = useState('');

  function openFileSearch() {
    setIsAddMenuOpen?.(false);
    setIsTrashPanelOpen(false);
    setIsFileSearchOpen(true);
  }

  function closeFileSearch() {
    setIsFileSearchOpen(false);
    setFileSearchError('');
  }

  useEffect(() => {
    if (!isFileSearchOpen) return undefined;
    const query = fileSearchQuery.trim();
    if (!query) {
      setFileSearchResults([]);
      setFileSearchError('');
      setIsFileSearchLoading(false);
      return undefined;
    }

    let cancelled = false;
    const timeout = window.setTimeout(async () => {
      setIsFileSearchLoading(true);
      setFileSearchError('');
      try {
        const data = await apiPost('/api/files/search', { query, limit: 80 });
        if (!cancelled) setFileSearchResults(data.items || []);
      } catch (error) {
        console.error('searchFiles failed', error);
        if (!cancelled) {
          setFileSearchResults([]);
          setFileSearchError(error.message);
        }
      } finally {
        if (!cancelled) setIsFileSearchLoading(false);
      }
    }, 140);

    return () => {
      cancelled = true;
      window.clearTimeout(timeout);
    };
  }, [fileSearchQuery, isFileSearchOpen]);

  async function openSearchResult(item) {
    if (!item?.path) return;
    setOpenFolders((current) => {
      const next = new Set(current);
      for (const folderPath of getAncestorFolderPaths(item.path)) next.add(folderPath);
      if (item.type === 'folder') next.add(item.path);
      return next;
    });
    setSelectedItemPath(item.path);
    setSelectedItemType(item.type || 'file');
    setSelectedPaths(new Set([item.path]));
    setLastSelectedPath(item.path);
    closeFileSearch();
    if (item.type === 'file') {
      await openFile?.(item.path);
    }
  }

  return {
    closeFileSearch,
    fileSearchError,
    fileSearchQuery,
    fileSearchResults,
    isFileSearchLoading,
    isFileSearchOpen,
    openFileSearch,
    openSearchResult,
    setFileSearchQuery,
  };
}
