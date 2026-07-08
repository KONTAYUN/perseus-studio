import React from 'react';
import { AlertCircle, CheckCircle2, Info, Loader2, X } from 'lucide-react';

export default function ToastViewport({ toasts, onDismiss, onRemove }) {
  if (!toasts?.length) return null;

  return (
    <div className="toast-viewport" aria-live="polite" aria-atomic="false">
      {toasts.map((toast) => (
        <ToastItem
          key={toast.id}
          toast={toast}
          onDismiss={onDismiss}
          onRemove={onRemove}
        />
      ))}
    </div>
  );
}

function ToastItem({ toast, onDismiss, onRemove }) {
  React.useEffect(() => {
    if (toast.exiting) return undefined;
    const timer = window.setTimeout(() => onDismiss?.(toast.id), toast.duration || 3000);
    return () => window.clearTimeout(timer);
  }, [onDismiss, toast.duration, toast.exiting, toast.id]);

  const Icon = iconForTone(toast.tone);

  return (
    <div
      className={`toast-item toast-item-${toast.tone || 'info'} ${
        toast.exiting ? 'motion-toast-exit' : 'motion-toast'
      }`}
      role={toast.tone === 'error' ? 'alert' : 'status'}
      onAnimationEnd={() => {
        if (toast.exiting) onRemove?.(toast.id);
      }}
    >
      <span className="toast-icon" aria-hidden="true">
        <Icon size={16} strokeWidth={2.1} />
      </span>
      <div className="toast-text">{toast.text}</div>
      <button
        type="button"
        className="toast-close motion-button"
        onClick={() => onDismiss?.(toast.id)}
        title="关闭通知"
      >
        <X size={13} />
      </button>
    </div>
  );
}

function iconForTone(tone) {
  if (tone === 'success') return CheckCircle2;
  if (tone === 'error' || tone === 'warning') return AlertCircle;
  if (tone === 'info') return Info;
  return Loader2;
}
