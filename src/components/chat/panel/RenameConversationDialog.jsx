import React from 'react';
import { createPortal } from 'react-dom';
import { X } from 'lucide-react';
import { Button, IconButton, Input } from '../../ui';

export default function RenameConversationDialog({
  draft,
  onClose,
  onDraftChange,
  onSubmit,
  open,
  returnFocusRef,
}) {
  const dialogRef = React.useRef(null);
  const closeRef = React.useRef(onClose);
  closeRef.current = onClose;
  React.useEffect(() => {
    if (!open) return undefined;
    const previous = returnFocusRef?.current || document.activeElement;
    const keydown = event => {
      if (event.key === 'Escape') { event.preventDefault(); closeRef.current(); }
      if (event.key !== 'Tab') return;
      const controls = Array.from(dialogRef.current?.querySelectorAll('input, button:not(:disabled)') || []);
      const target = event.shiftKey ? controls.at(-1) : controls[0];
      if (document.activeElement === (event.shiftKey ? controls[0] : controls.at(-1)) || !dialogRef.current?.contains(document.activeElement)) {
        event.preventDefault(); target?.focus();
      }
    };
    document.addEventListener('keydown', keydown);
    return () => { document.removeEventListener('keydown', keydown); if (previous?.isConnected) previous.focus(); };
  }, [open, returnFocusRef]);
  if (!open) return null;

  return createPortal(
    <div
      className="fixed inset-0 z-[500] flex items-center justify-center bg-black/45 p-4 backdrop-blur-sm"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <form
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-label="重命名会话"
        onSubmit={onSubmit}
        className="motion-modal w-full max-w-md rounded-3xl border border-white/70 bg-white/95 p-5 backdrop-blur-2xl dark:border-white/10 dark:bg-[#252527]/95"
      >
        <div className="flex items-start justify-between gap-4">
          <div>
            <div className="text-base font-bold text-gray-900 dark:text-gray-100">重命名会话</div>
            <div className="mt-1 text-xs text-gray-500 dark:text-gray-400">给这段历史会话起一个更容易识别的名字。</div>
          </div>
          <IconButton
            type="button"
            onClick={onClose}
            title="关闭"
          >
            <X size={16} />
          </IconButton>
        </div>
        <Input
          autoFocus
          value={draft}
          onChange={(e) => onDraftChange(e.target.value)}
          className="mt-5 h-11 px-4 font-semibold"
          placeholder="会话名称"
        />
        <div className="mt-5 flex justify-end gap-2">
          <Button
            type="button"
            onClick={onClose}
            variant="secondary"
          >
            取消
          </Button>
          <Button
            type="submit"
          >
            保存
          </Button>
        </div>
      </form>
    </div>, document.body
  );
}
