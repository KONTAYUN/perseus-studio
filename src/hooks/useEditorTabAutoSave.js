import { useEffect } from 'react';

export default function useEditorTabAutoSave({
  autoSaveTimerRef,
  currentFileContent,
  currentFilePath,
  externalFileConflict,
  isDirty,
  isDirtyRef,
  isEditorLoading,
  isSavingFile,
  isTextFile,
  saveFileContent,
  savedFileContent,
}) {
  useEffect(() => {
    if (autoSaveTimerRef.current) {
      window.clearTimeout(autoSaveTimerRef.current);
      autoSaveTimerRef.current = null;
    }
    if (!currentFilePath || !isTextFile || !isDirty || externalFileConflict || isEditorLoading || isSavingFile) {
      return undefined;
    }
    autoSaveTimerRef.current = window.setTimeout(() => {
      autoSaveTimerRef.current = null;
      saveFileContent({ auto: true }).catch((error) => console.error('autoSave failed', error));
    }, 1000);
    return () => {
      if (autoSaveTimerRef.current) {
        window.clearTimeout(autoSaveTimerRef.current);
        autoSaveTimerRef.current = null;
      }
    };
  }, [currentFilePath, currentFileContent, savedFileContent, isTextFile, isDirty, externalFileConflict, isEditorLoading, isSavingFile]);

  useEffect(() => {
    const onBeforeUnload = (event) => {
      if (!isDirtyRef.current) return;
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', onBeforeUnload);
    return () => window.removeEventListener('beforeunload', onBeforeUnload);
  }, []);
}
