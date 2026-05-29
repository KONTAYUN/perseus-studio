import React from 'react';
import { Archive, Gauge } from 'lucide-react';
import {
  buildContextUsageLabel,
  buildContextUsageTitle,
  formatContextTokenCount,
} from '../../../hooks/chatStateUtils';

export default function ContextUsageBadge({ usage, notice, canCompact = false, compactDisabled = false, onCompact }) {
  if (!usage && !notice && !canCompact) return null;
  const isCompacting = notice?.status === 'started';
  const percent = usage?.usedPercent ?? 0;
  const tone = percent >= 90
    ? 'ui-icon-button-danger-state'
    : percent >= 75
      ? 'ui-icon-button-warning-state'
      : '';
  const label = buildContextUsageLabel(usage, notice);
  const title = buildContextUsageTitle(usage);

  return (
    <div className="group/context relative hidden shrink-0 min-[460px]:block">
      <div
        aria-label="上下文用量"
        className={`motion-button ui-icon-button h-8 w-8 ${
          isCompacting
            ? 'ui-icon-button-warning-state bg-amber-500/10 dark:bg-amber-400/10'
            : tone
        }`}
      >
        <Gauge size={15} className={isCompacting ? 'animate-pulse' : ''} />
      </div>
      <div className="context-usage-popover pointer-events-none absolute left-0 top-9 z-[240] w-60 translate-y-1 rounded-2xl border border-gray-100 bg-white/95 p-3 text-[11px] leading-5 text-gray-500 opacity-0 backdrop-blur-xl transition-[opacity,transform] duration-150 group-hover/context:pointer-events-auto group-hover/context:translate-y-0 group-hover/context:opacity-100 dark:border-white/10 dark:bg-[#2C2C2E]/95 dark:text-gray-300">
        {usage ? (
          <>
            <div className="flex items-center justify-between gap-3">
              <div className="font-bold text-gray-700 dark:text-gray-100">{title}</div>
              <div className="rounded-full bg-gray-100 px-2 py-0.5 font-bold text-gray-600 dark:bg-white/10 dark:text-gray-200">
                {label}
              </div>
            </div>
            <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-white/10">
              <div
                className={`h-full rounded-full ${
                  percent >= 90 ? 'bg-red-500' : percent >= 75 ? 'bg-amber-500' : 'bg-blue-500'
                }`}
                style={{ width: `${Math.min(100, Math.max(0, percent))}%` }}
              />
            </div>
            <div className="mt-2">{percent}% 已用，剩余 {usage.remainingPercent}%</div>
            <div>
              已用 {formatContextTokenCount(usage.estimatedTokens)}，共 {formatContextTokenCount(usage.contextWindowTokens)}
            </div>
            <div>
              缓冲 {formatContextTokenCount(usage.compactReservedTokens)}，预算 {formatContextTokenCount(usage.compactionBudgetTokens)}
            </div>
            <div>
              消息 {usage.messageCount}，工具结果 {usage.toolResultCount}
            </div>
            {usage.source === 'stored' && (
              <div className="mt-1 text-gray-400 dark:text-gray-500">
                来自最近一次模型请求；新回复开始后会实时更新。
              </div>
            )}
            {notice?.text && (
              <div className="mt-2 rounded-xl bg-blue-50 px-2 py-1 font-semibold text-blue-600 dark:bg-blue-500/10 dark:text-blue-200">
                {notice.text}
              </div>
            )}
          </>
        ) : (
          <div className="font-semibold">{notice?.text || '上下文状态'}</div>
        )}
        {canCompact && (
          <button
            type="button"
            onClick={(event) => {
              event.stopPropagation();
              onCompact?.();
            }}
            disabled={compactDisabled}
            className="motion-row mt-2 flex w-full items-center gap-2 rounded-xl border border-gray-100 bg-gray-50/80 px-2.5 py-2 text-left text-[11px] font-bold text-gray-700 hover:border-blue-100 hover:bg-blue-50 hover:text-blue-600 disabled:cursor-not-allowed disabled:opacity-50 dark:border-white/10 dark:bg-white/5 dark:text-gray-200 dark:hover:border-blue-400/20 dark:hover:bg-blue-500/15 dark:hover:text-blue-200"
          >
            <Archive size={13} className="shrink-0" />
            <span className="min-w-0 flex-1 truncate">压缩上下文</span>
          </button>
        )}
      </div>
    </div>
  );
}
