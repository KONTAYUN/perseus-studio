import { useEffect } from 'react';

export default function useActiveEditorTabSync({
  activeTabPath,
  buildTabSnapshot,
  currentFileContent,
  currentFileHash,
  currentFileNotice,
  currentFilePath,
  currentFileType,
  currentFileVersionId,
  editorStatusMessage,
  externalFileConflict,
  savedFileContent,
  setOpenTabs,
}) {
  useEffect(() => {
    if (!currentFilePath || activeTabPath !== currentFilePath) return;
    const snapshot = buildTabSnapshot();
    setOpenTabs((current) =>
      current.map((tab) => (tab.path === currentFilePath ? { ...tab, ...snapshot } : tab))
    );
  }, [
    currentFilePath,
    activeTabPath,
    currentFileContent,
    savedFileContent,
    currentFileHash,
    currentFileVersionId,
    currentFileType,
    currentFileNotice,
    externalFileConflict,
    editorStatusMessage,
  ]);
}
