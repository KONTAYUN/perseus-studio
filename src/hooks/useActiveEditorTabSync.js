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
  currentDocumentVersion,
  editorStatusMessage,
  externalFileConflict,
  aiPreview,
  savedFileContent,
  userRevision,
  savedUserRevision,
  markdownViewState,
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
    currentDocumentVersion,
    currentFileType,
    currentFileNotice,
    externalFileConflict,
    aiPreview,
    userRevision,
    savedUserRevision,
    editorStatusMessage,
    markdownViewState,
  ]);
}
