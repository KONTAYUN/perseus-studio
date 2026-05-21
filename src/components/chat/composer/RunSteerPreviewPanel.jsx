import React from 'react';
import { CornerDownRight, RefreshCw, Send, Trash2 } from 'lucide-react';
import { Button } from '../../ui';
import { pendingRunSteerPreviewItems, RUN_STEER_STATUS } from '../../../hooks/chatSteerItems';

export default function RunSteerPreviewPanel({ items, onDelete, onQueueAfterRun, onRetry }) {
  const pendingItems = pendingRunSteerPreviewItems(items);
  if (pendingItems.length === 0) return null;
  const failedItems = pendingItems.filter((item) => item.status === RUN_STEER_STATUS.failed);
  const waitingItems = pendingItems.filter((item) => item.status !== RUN_STEER_STATUS.failed);
  return (
    <div className="pending-run-message-panel motion-popover pointer-events-auto mb-3">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1.5 text-[11px] font-bold uppercase tracking-wide text-blue-500/80 dark:text-blue-200/80">
          <CornerDownRight size={13} />
          <span>{previewHeading(pendingItems)}</span>
          {waitingItems.length > 0 && (
            <span className="inline-flex gap-0.5">
              <span className="h-1 w-1 animate-bounce rounded-full bg-current [animation-delay:-0.2s]" />
              <span className="h-1 w-1 animate-bounce rounded-full bg-current [animation-delay:-0.1s]" />
              <span className="h-1 w-1 animate-bounce rounded-full bg-current" />
            </span>
          )}
        </div>
        <div className="mt-1 max-h-24 overflow-y-auto whitespace-pre-wrap break-words pr-1 text-sm font-semibold leading-6 text-gray-800 dark:text-gray-100 custom-scrollbar">
          {pendingItems.map((item) => item.content).join('\n\n')}
        </div>
        {failedItems.length > 0 && (
          <div className="mt-2 text-[11px] font-semibold text-amber-600 dark:text-amber-300">
            {failedItems[0].error || '引导没有被当前任务接收，可以重试、删除，或改为任务结束后发送。'}
          </div>
        )}
      </div>
      <div className="pending-run-message-actions">
        {failedItems.length === 1 && (
          <>
            <Button
              type="button"
              variant="subtle"
              size="xs"
              className="pending-run-message-action"
              onClick={(e) => {
                e.stopPropagation();
                onRetry?.(failedItems[0].id || failedItems[0].clientSteerId || failedItems[0].steerId);
              }}
            >
              <RefreshCw size={13} />
              重试
            </Button>
            <Button
              type="button"
              variant="subtle"
              size="xs"
              className="pending-run-message-action"
              onClick={(e) => {
                e.stopPropagation();
                onQueueAfterRun?.(failedItems[0].id || failedItems[0].clientSteerId || failedItems[0].steerId);
              }}
            >
              <Send size={13} />
              任务结束后发送
            </Button>
          </>
        )}
        <button
          type="button"
          className="pending-run-message-delete motion-button"
          onClick={(e) => {
            e.stopPropagation();
            for (const item of pendingItems) {
              onDelete?.(item.id || item.clientSteerId || item.steerId);
            }
          }}
          title="删除等待中的引导"
        >
          <Trash2 size={14} />
        </button>
      </div>
    </div>
  );
}

function previewHeading(items) {
  if (items.length > 1) return `${items.length} 条引导等待处理`;
  const item = items[0];
  if (item?.status === RUN_STEER_STATUS.failed) return '引导未送达';
  if (item?.status === RUN_STEER_STATUS.received) return '等待应用到当前任务';
  return '正在发送引导';
}
