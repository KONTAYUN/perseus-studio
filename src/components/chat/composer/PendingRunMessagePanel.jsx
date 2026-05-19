import React from 'react';
import { CornerDownRight, Trash2 } from 'lucide-react';
import { Button } from '../../ui';

export default function PendingRunMessagePanel({
  message,
  notice,
  disabled,
  onSteer,
  onDelete,
}) {
  if (!message?.content) return null;
  return (
    <div className="pending-run-message-panel motion-popover pointer-events-auto mb-3">
      <div className="min-w-0 flex-1">
        <div className="text-[11px] font-bold uppercase tracking-wide text-blue-500/80 dark:text-blue-200/80">
          当前任务结束后自动发送
        </div>
        <div className="mt-1 max-h-24 overflow-y-auto whitespace-pre-wrap break-words pr-1 text-sm font-semibold leading-6 text-gray-800 dark:text-gray-100 custom-scrollbar">
          {message.content}
        </div>
        {notice?.text && (
          <div className={`mt-2 text-[11px] font-semibold ${notice.status === 'error' || notice.status === 'warn' ? 'text-amber-600 dark:text-amber-300' : 'text-gray-400 dark:text-gray-500'}`}>
            {notice.text}
          </div>
        )}
      </div>
      <div className="pending-run-message-actions">
        <Button
          type="button"
          variant="subtle"
          size="xs"
          disabled={disabled}
          className="pending-run-message-action"
          onClick={(e) => {
            e.stopPropagation();
            onSteer?.();
          }}
        >
          <CornerDownRight size={13} />
          引导当前任务
        </Button>
        <button
          type="button"
          className="pending-run-message-delete motion-button"
          onClick={(e) => {
            e.stopPropagation();
            onDelete?.();
          }}
          title="删除挂起消息"
        >
          <Trash2 size={14} />
        </button>
      </div>
    </div>
  );
}
