import React from 'react';
import { CornerDownRight, Trash2 } from 'lucide-react';
import { Button } from '../../ui';
import useMotionPresence from '../../../hooks/useMotionPresence';

export default function PendingRunMessagePanel({
  message,
  notice,
  disabled,
  onSteer,
  onDelete,
}) {
  const hasMessage = Boolean(message?.content);
  const { exiting, present } = useMotionPresence(hasMessage, { exitToken: '--motion-base' });
  const lastMessageRef = React.useRef(null);
  if (message?.content) lastMessageRef.current = message;
  if (!present || !lastMessageRef.current?.content) return null;
  const visibleMessage = hasMessage ? message : lastMessageRef.current;
  return (
    <div className={`pending-run-message-panel motion-card pointer-events-auto mb-3 ${exiting ? 'motion-presence-exit pointer-events-none' : ''}`}>
      <div className="min-w-0 flex-1">
        <div className="text-[11px] font-bold uppercase tracking-wide text-blue-500/80 dark:text-blue-200/80">
          当前任务结束后自动发送
        </div>
        <div className="mt-1 max-h-24 overflow-y-auto whitespace-pre-wrap break-words pr-1 text-sm font-semibold leading-6 text-gray-800 dark:text-gray-100 custom-scrollbar">
          {visibleMessage.content}
        </div>
        {visibleMessage === message && notice?.text && (
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
