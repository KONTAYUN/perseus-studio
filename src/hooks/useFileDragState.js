import { useEffect, useState } from 'react';

export function useFileDragState() {
  const [draggingFileId, setDraggingFileId] = useState(null);
  const [draggingFilePath, setDraggingFilePath] = useState('');
  const [fileDragOverId, setFileDragOverId] = useState(null);
  const [fileDropIndicator, setFileDropIndicator] = useState(null);

  const resetFileDragState = () => {
    setFileDragOverId(null);
    setFileDropIndicator(null);
    setDraggingFileId(null);
    setDraggingFilePath('');
  };

  useEffect(() => {
    const clearDragState = () => resetFileDragState();
    window.addEventListener('dragend', clearDragState);
    window.addEventListener('drop', clearDragState);
    return () => {
      window.removeEventListener('dragend', clearDragState);
      window.removeEventListener('drop', clearDragState);
    };
  }, []);

  const handleFileDragStart = (e, fileId, filePath = '') => {
    e.stopPropagation();
    setDraggingFileId(fileId);
    setDraggingFilePath(filePath);
    e.dataTransfer.setData('file_id', fileId);
  };

  const handleFileDrop = (e) => {
    e.preventDefault();
    e.stopPropagation();
    resetFileDragState();
  };

  return {
    draggingFileId,
    draggingFilePath,
    fileDragOverId,
    fileDropIndicator,
    handleFileDragStart,
    handleFileDrop,
    resetFileDragState,
    setFileDragOverId,
    setFileDropIndicator,
  };
}
