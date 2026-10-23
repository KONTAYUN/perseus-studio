import React from 'react';
import { AlertTriangle, Edit3, X } from 'lucide-react';
import { Button, IconButton, Input } from '../ui';
import useModalFocus from '../../hooks/useModalFocus';

export function useAppDialog() {
  const [dialog, setDialog] = React.useState(null);

  const close = React.useCallback((result) => {
    setDialog((current) => {
      current?.resolve?.(result);
      return null;
    });
  }, []);

  const confirm = React.useCallback((options = {}) => new Promise((resolve) => {
    setDialog({
      type: 'confirm',
      title: options.title || '确认操作',
      message: options.message || '',
      confirmLabel: options.confirmLabel || '确认',
      cancelLabel: options.cancelLabel || '取消',
      tone: options.tone || 'blue',
      resolve,
    });
  }), []);

  const prompt = React.useCallback((options = {}) => new Promise((resolve) => {
    setDialog({
      type: 'prompt',
      title: options.title || '输入内容',
      message: options.message || '',
      initialValue: options.initialValue || '',
      placeholder: options.placeholder || '',
      confirmLabel: options.confirmLabel || '保存',
      cancelLabel: options.cancelLabel || '取消',
      tone: options.tone || 'blue',
      resolve,
    });
  }), []);

  const api = React.useMemo(() => ({ confirm, prompt }), [confirm, prompt]);

  return { dialog, close, api };
}

export default function AppDialog({ dialog, onClose }) {
  const [value, setValue] = React.useState('');
  const inputRef = React.useRef(null);
  const modalRef = React.useRef(null);
  const dismiss = React.useCallback(() => onClose(dialog?.type === 'prompt' ? null : false), [dialog?.type, onClose]);
  useModalFocus(modalRef, Boolean(dialog), dismiss, dialog?.type === 'prompt' ? 'input' : '[data-dialog-cancel]');

  React.useLayoutEffect(() => {
    if (!dialog) return;
    setValue(dialog.initialValue || '');
    if (dialog.type === 'prompt') queueMicrotask(() => inputRef.current?.select());
  }, [dialog, onClose]);

  if (!dialog) return null;

  const isDanger = dialog.tone === 'red' || dialog.tone === 'danger';
  const submit = (event) => {
    event.preventDefault();
    if (dialog.type === 'prompt') {
      onClose(value);
      return;
    }
    onClose(true);
  };

  return (
    <div
      className="modal-backdrop fixed inset-0 z-[500] flex items-center justify-center bg-black/45 p-4 backdrop-blur-md animate-in fade-in duration-300"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose(dialog.type === 'prompt' ? null : false);
      }}
    >
      <form
        ref={modalRef}
        role="dialog"
        aria-modal="true"
        aria-label={dialog.title}
        tabIndex={-1}
        onSubmit={submit}
        className="ui-overlay-surface motion-modal w-full max-w-md border border-white/50 bg-white/95 p-5 backdrop-blur-3xl dark:border-white/10 dark:bg-[#252527]/95"
      >
        <div className="flex items-start gap-4">
          <div
            className={`dialog-tone-icon flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl ${
              isDanger
                ? 'bg-red-500/10 text-red-500 dark:bg-red-500/15 dark:text-red-300'
                : 'bg-blue-500/10 text-blue-500 dark:bg-blue-500/15 dark:text-blue-300'
            }`}
          >
            {dialog.type === 'prompt' ? <Edit3 size={20} /> : <AlertTriangle size={20} />}
          </div>
          <div className="min-w-0 flex-1">
            <div className="text-base font-bold text-gray-900 dark:text-gray-100">{dialog.title}</div>
            {dialog.message && (
              <div className="mt-1 whitespace-pre-wrap break-words text-sm leading-6 text-gray-500 dark:text-gray-400">
                {dialog.message}
              </div>
            )}
          </div>
          <IconButton
            type="button"
            onClick={() => onClose(dialog.type === 'prompt' ? null : false)}
            title="关闭"
          >
            <X size={16} />
          </IconButton>
        </div>

        {dialog.type === 'prompt' && (
          <Input
            ref={inputRef}
            value={value}
            onChange={(event) => setValue(event.target.value)}
            placeholder={dialog.placeholder}
            className="mt-5 h-11 px-4 font-semibold"
          />
        )}

        <div className="mt-6 flex justify-end gap-2">
          <Button
            data-dialog-cancel=""
            type="button"
            onClick={() => onClose(dialog.type === 'prompt' ? null : false)}
            variant="secondary"
          >
            {dialog.cancelLabel}
          </Button>
          <Button
            type="submit"
            variant={isDanger ? 'danger' : 'primary'}
          >
            {dialog.confirmLabel}
          </Button>
        </div>
      </form>
    </div>
  );
}
