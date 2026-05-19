import { useState } from 'react';

export function useFileSelectionState(flatFiles) {
  const [selectedItemPath, setSelectedItemPath] = useState('');
  const [selectedItemType, setSelectedItemType] = useState('root');
  const [selectedPaths, setSelectedPaths] = useState(new Set());
  const [lastSelectedPath, setLastSelectedPath] = useState('');
  const selectedItems = flatFiles.filter((item) => selectedPaths.has(item.path));

  function selectItem(path, type) {
    setSelectedItemPath(path);
    setSelectedItemType(type);
    setSelectedPaths(new Set([path]));
    setLastSelectedPath(path);
  }

  function clearSelection() {
    setSelectedItemPath('');
    setSelectedItemType('root');
    setSelectedPaths(new Set());
    setLastSelectedPath('');
  }

  function selectTreeItem(path, type, event) {
    setSelectedItemPath(path);
    setSelectedItemType(type);

    if (event?.shiftKey && lastSelectedPath) {
      const paths = flatFiles.map((item) => item.path);
      const start = paths.indexOf(lastSelectedPath);
      const end = paths.indexOf(path);
      if (start !== -1 && end !== -1) {
        const [from, to] = start < end ? [start, end] : [end, start];
        setSelectedPaths(new Set(paths.slice(from, to + 1)));
        return;
      }
    }

    if (event?.metaKey || event?.ctrlKey) {
      setSelectedPaths((current) => {
        const next = new Set(current);
        if (next.has(path)) next.delete(path);
        else next.add(path);
        return next;
      });
      setLastSelectedPath(path);
      return;
    }

    setSelectedPaths(new Set([path]));
    setLastSelectedPath(path);
  }

  return {
    clearSelection,
    lastSelectedPath,
    selectItem,
    selectedItemPath,
    selectedItemType,
    selectedItems,
    selectedPaths,
    selectTreeItem,
    setLastSelectedPath,
    setSelectedItemPath,
    setSelectedItemType,
    setSelectedPaths,
  };
}
