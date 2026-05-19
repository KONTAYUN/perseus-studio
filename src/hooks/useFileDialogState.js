import { useRef, useState } from 'react';

export function useFileDialogState() {
  const [fileDialog, setFileDialog] = useState(null);
  const fileDialogResolver = useRef(null);

  function submitFileDialog(value) {
    if (fileDialogResolver.current) {
      fileDialogResolver.current(value);
      fileDialogResolver.current = null;
    }
    setFileDialog(null);
  }

  function cancelFileDialog() {
    if (fileDialogResolver.current) {
      fileDialogResolver.current(fileDialog?.kind === 'confirm' ? false : '');
      fileDialogResolver.current = null;
    }
    setFileDialog(null);
  }

  function openInputDialog(config) {
    return new Promise((resolve) => {
      fileDialogResolver.current = resolve;
      setFileDialog({ kind: 'input', ...config });
    });
  }

  function openConfirmDialog(config) {
    return new Promise((resolve) => {
      fileDialogResolver.current = resolve;
      setFileDialog({ kind: 'confirm', cancelText: '取消', ...config });
    });
  }

  return {
    cancelFileDialog,
    fileDialog,
    openConfirmDialog,
    openInputDialog,
    submitFileDialog,
  };
}
