import React from 'react';
import { X } from 'lucide-react';
import { Button, IconButton, Input } from '../../ui';

export default function RenameConversationDialog({
  draft,
  onClose,
  onDraftChange,
  onSubmit,
  open,
}) {
  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-[320] flex items-center justify-center bg-black/45 p-4 backdrop-blur-sm"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <form
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
    </div>
  );
}
