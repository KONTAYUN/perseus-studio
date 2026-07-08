import React from 'react';
import { AlertTriangle, Edit3, X } from 'lucide-react';
import useMotionPresence from '../../hooks/useMotionPresence';
import { Button, IconButton, Input } from '../ui';

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
  const latestDialogRef = React.useRef(null);
  const { closeWithPresence, exiting, present } = useMotionPresence(Boolean(dialog));

  React.useEffect(() => {
    if (dialog) latestDialogRef.current = dialog;
  }, [dialog]);

  const renderedDialog = dialog || latestDialogRef.current;
  const requestClose = React.useCallback((result) => {
    closeWithPresence(() => onClose(result));
  }, [closeWithPresence, onClose]);

  React.useEffect(() => {
    if (!renderedDialog) return undefined;
    setValue(renderedDialog.initialValue || '');
    const onKeyDown = (event) => {
      if (event.key === 'Escape') requestClose(renderedDialog.type === 'prompt' ? null : false);
    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [renderedDialog, requestClose]);

  React.useEffect(() => {
    if (renderedDialog?.type !== 'prompt') return;
    const timer = window.setTimeout(() => {
      inputRef.current?.focus();
      inputRef.current?.select();
    }, 60);
    return () => window.clearTimeout(timer);
  }, [renderedDialog]);

  if (!present || !renderedDialog) return null;

  const isDanger = renderedDialog.tone === 'red' || renderedDialog.tone === 'danger';
  const submit = (event) => {
    event.preventDefault();
    if (renderedDialog.type === 'prompt') {
      requestClose(value);
      return;
    }
    requestClose(true);
  };

  return (
    <div
      className={`modal-backdrop motion-fade-reveal fixed inset-0 z-[500] flex items-center justify-center bg-black/45 p-4 backdrop-blur-md ${
        exiting ? 'opacity-0' : 'animate-in fade-in opacity-100'
      }`}
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) requestClose(renderedDialog.type === 'prompt' ? null : false);
      }}
    >
      <form
        onSubmit={submit}
        className={`ui-overlay-surface motion-modal w-full max-w-md border border-white/50 bg-white/95 p-5 backdrop-blur-3xl dark:border-white/10 dark:bg-[#252527]/95 ${
          exiting ? 'motion-presence-exit' : ''
        }`}
      >
        <div className="flex items-start gap-4">
          <div
            className={`dialog-tone-icon flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl ${
              isDanger
                ? 'bg-red-500/10 text-red-500 dark:bg-red-500/15 dark:text-red-300'
                : 'bg-blue-500/10 text-blue-500 dark:bg-blue-500/15 dark:text-blue-300'
            }`}
          >
            {renderedDialog.type === 'prompt' ? <Edit3 size={20} /> : <AlertTriangle size={20} />}
          </div>
          <div className="min-w-0 flex-1">
            <div className="text-base font-bold text-gray-900 dark:text-gray-100">{renderedDialog.title}</div>
            {renderedDialog.message && (
              <div className="mt-1 whitespace-pre-wrap break-words text-sm leading-6 text-gray-500 dark:text-gray-400">
                {renderedDialog.message}
              </div>
            )}
          </div>
          <IconButton
            type="button"
            onClick={() => requestClose(renderedDialog.type === 'prompt' ? null : false)}
            title="关闭"
          >
            <X size={16} />
          </IconButton>
        </div>

        {renderedDialog.type === 'prompt' && (
          <Input
            ref={inputRef}
            value={value}
            onChange={(event) => setValue(event.target.value)}
            placeholder={renderedDialog.placeholder}
            className="mt-5 h-11 px-4 font-semibold"
          />
        )}

        <div className="mt-6 flex justify-end gap-2">
          <Button
            type="button"
            onClick={() => requestClose(renderedDialog.type === 'prompt' ? null : false)}
            variant="secondary"
          >
            {renderedDialog.cancelLabel}
          </Button>
          <Button
            type="submit"
            variant={isDanger ? 'danger' : 'primary'}
          >
            {renderedDialog.confirmLabel}
          </Button>
        </div>
      </form>
    </div>
  );
}
