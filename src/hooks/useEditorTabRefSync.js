import { useEffect } from 'react';

export default function useEditorTabRefSync({
  activeTabPath,
  activeTabPathRef,
  currentFileContent,
  currentFileContentRef,
  currentFilePath,
  currentFilePathRef,
  externalFileConflict,
  externalFileConflictRef,
  isDirty,
  isDirtyRef,
  isTextFile,
  isTextFileRef,
  openTabs,
  openTabsRef,
  savedFileContent,
  savedFileContentRef,
}) {
  useEffect(() => {
    currentFilePathRef.current = currentFilePath;
  }, [currentFilePath, currentFilePathRef]);

  useEffect(() => {
    currentFileContentRef.current = currentFileContent;
  }, [currentFileContent, currentFileContentRef]);

  useEffect(() => {
    savedFileContentRef.current = savedFileContent;
  }, [savedFileContent, savedFileContentRef]);

  useEffect(() => {
    isTextFileRef.current = isTextFile;
  }, [isTextFile, isTextFileRef]);

  useEffect(() => {
    externalFileConflictRef.current = externalFileConflict;
  }, [externalFileConflict, externalFileConflictRef]);

  useEffect(() => {
    openTabsRef.current = openTabs;
  }, [openTabs, openTabsRef]);

  useEffect(() => {
    activeTabPathRef.current = activeTabPath;
  }, [activeTabPath, activeTabPathRef]);

  useEffect(() => {
    isDirtyRef.current = isDirty;
  }, [isDirty, isDirtyRef]);
}
