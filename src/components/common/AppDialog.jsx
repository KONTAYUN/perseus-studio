import React from 'react';
import { AlertTriangle, Edit3, X } from 'lucide-react';

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

  React.useEffect(() => {
    if (!dialog) return undefined;
    setValue(dialog.initialValue || '');
    const onKeyDown = (event) => {
      if (event.key === 'Escape') onClose(dialog.type === 'prompt' ? null : false);
    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [dialog, onClose]);

  React.useEffect(() => {
    if (dialog?.type !== 'prompt') return;
    const timer = window.setTimeout(() => {
      inputRef.current?.focus();
      inputRef.current?.select();
    }, 60);
    return () => window.clearTimeout(timer);
  }, [dialog]);

  if (!dialog) return null;

  const isDanger = dialog.tone === 'red' || dialog.tone === 'danger';
  const confirmClass = isDanger
    ? 'bg-red-500 shadow-red-500/25 hover:bg-red-600'
    : 'bg-blue-500 shadow-blue-500/25 hover:bg-blue-600';

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
      className="fixed inset-0 z-[500] flex items-center justify-center bg-black/45 p-4 backdrop-blur-md animate-in fade-in duration-300"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose(dialog.type === 'prompt' ? null : false);
      }}
    >
      <form
        onSubmit={submit}
        className="ui-overlay-surface motion-modal w-full max-w-md border border-white/50 bg-white/95 p-5 backdrop-blur-3xl dark:border-white/10 dark:bg-[#252527]/95"
      >
        <div className="flex items-start gap-4">
          <div
            className={`flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl ${
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
          <button
            type="button"
            onClick={() => onClose(dialog.type === 'prompt' ? null : false)}
            className="motion-button rounded-xl p-2 text-gray-400 hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-white/10 dark:hover:text-gray-100"
            title="关闭"
          >
            <X size={16} />
          </button>
        </div>

        {dialog.type === 'prompt' && (
          <input
            ref={inputRef}
            value={value}
            onChange={(event) => setValue(event.target.value)}
            placeholder={dialog.placeholder}
            className="mt-5 h-11 w-full rounded-2xl border border-gray-200 bg-white px-4 text-sm font-semibold text-gray-800 outline-none transition-all focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 dark:border-white/10 dark:bg-[#1C1C1E] dark:text-white"
          />
        )}

        <div className="mt-6 flex justify-end gap-2">
          <button
            type="button"
            onClick={() => onClose(dialog.type === 'prompt' ? null : false)}
            className="motion-button rounded-2xl bg-gray-100 px-4 py-2 text-sm font-bold text-gray-600 hover:bg-gray-200 dark:bg-white/10 dark:text-gray-300 dark:hover:bg-white/15"
          >
            {dialog.cancelLabel}
          </button>
          <button
            type="submit"
            className={`motion-button rounded-2xl px-4 py-2 text-sm font-bold text-white shadow-lg ${confirmClass}`}
          >
            {dialog.confirmLabel}
          </button>
        </div>
      </form>
    </div>
  );
}
