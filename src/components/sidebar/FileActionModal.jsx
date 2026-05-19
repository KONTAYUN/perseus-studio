import React from 'react';
import { Button, DialogActions, Input } from '../ui';

export default function FileActionModal({ ide }) {
  const dialog = ide.fileDialog;
  const [value, setValue] = React.useState('');

  React.useEffect(() => {
    setValue(dialog?.defaultValue || '');
  }, [dialog]);

  if (!dialog) return null;

  const submit = () => {
    if (dialog.kind === 'confirm') {
      ide.submitFileDialog(true);
      return;
    }
    ide.submitFileDialog(value.trim());
  };

  return (
    <div className="fixed inset-0 z-[120] flex items-center justify-center bg-black/35 dark:bg-black/60 backdrop-blur-md p-4">
      <div className="ui-overlay-surface w-full max-w-sm overflow-hidden border border-white/40 bg-white/95 dark:border-white/10 dark:bg-[#1C1C1E]/95">
        <div className="px-5 py-4 border-b border-gray-100 dark:border-white/10">
          <h3 className="text-sm font-bold text-gray-800 dark:text-gray-100">{dialog.title}</h3>
        </div>
        <div className="px-5 py-4">
          {dialog.kind === 'confirm' ? (
            <p className="text-sm text-gray-600 dark:text-gray-300">{dialog.message}</p>
          ) : (
            <label className="block">
              <span className="block mb-2 text-xs font-semibold text-gray-500 dark:text-gray-400">
                {dialog.label}
              </span>
              <Input
                data-testid="file-dialog-input"
                autoFocus
                value={value}
                onChange={(e) => setValue(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') submit();
                  if (e.key === 'Escape') ide.cancelFileDialog();
                }}
              />
            </label>
          )}
        </div>
        <DialogActions>
          <Button
            data-testid="file-dialog-cancel"
            type="button"
            onClick={ide.cancelFileDialog}
            variant="secondary"
          >
            {dialog.cancelText || '取消'}
          </Button>
          <Button
            data-testid="file-dialog-submit"
            type="button"
            onClick={submit}
            variant={dialog.danger ? 'danger' : 'primary'}
          >
            {dialog.confirmText || '确认'}
          </Button>
        </DialogActions>
      </div>
    </div>
  );
}
