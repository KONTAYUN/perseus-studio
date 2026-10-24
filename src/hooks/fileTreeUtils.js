import { getParentPath, pathMatches, remapPathPrefix, remapPathSet, removeFolderState, removePathSet, renameFolderState } from './ideUtils.js';

export function nextAvailablePathInTree(flatFiles, basePath, defaultName) {
  const buildPath = (name) => (basePath ? `${basePath}/${name}` : name);
  const paths = new Set();
  const collect = items => {
    for (const item of items) {
      if (paths.has(item.path)) continue;
      paths.add(item.path);
      collect(item.children || []);
    }
  };
  // Visible rows still contain their collapsed children; include those in name checks.
  collect(flatFiles);
  const exists = path => paths.has(path);
  if (!exists(buildPath(defaultName))) return buildPath(defaultName);

  const dotIndex = defaultName.lastIndexOf('.');
  const hasExtension = dotIndex > 0;
  const stem = hasExtension ? defaultName.slice(0, dotIndex) : defaultName;
  const extension = hasExtension ? defaultName.slice(dotIndex) : '';

  for (let index = 2; index < 10000; index += 1) {
    const candidate = buildPath(`${stem} (${index})${extension}`);
    if (!exists(candidate)) return candidate;
  }
  return buildPath(`${stem} (${Date.now()})${extension}`);
}

export function defaultCreateBasePath(selectedItemPath, selectedItemType) {
  if (!selectedItemPath) return '';
  if (selectedItemType === 'folder') return selectedItemPath;
  if (selectedItemType === 'file') return getParentPath(selectedItemPath);
  return '';
}

export function reducePathSelectionOnDelete(selectedItemPath, selectedItemType, selectedPaths, removedPath) {
  const shouldResetSelection = pathMatches(selectedItemPath, removedPath);
  return {
    selectedItemPath: pathMatches(selectedItemPath, removedPath) ? '' : selectedItemPath,
    selectedItemType: shouldResetSelection ? 'root' : selectedItemType,
    selectedPaths: removePathSet(selectedPaths, removedPath),
  };
}

export function reducePathSelectionOnRename(selectedItemPath, selectedPaths, openFolders, oldPath, newPath) {
  return {
    selectedItemPath: remapPathPrefix(selectedItemPath, oldPath, newPath),
    selectedPaths: remapPathSet(selectedPaths, oldPath, newPath),
    openFolders: renameFolderState(openFolders, oldPath, newPath),
  };
}

export function removeOpenFolderPath(openFolders, removedPath) {
  return removeFolderState(openFolders, removedPath);
}

export function topLevelSelectedItems(selectedItems) {
  return selectedItems.filter(
    (item) => !selectedItems.some((other) => other.path !== item.path && pathMatches(item.path, other.path))
  );
}
