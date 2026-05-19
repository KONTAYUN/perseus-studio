import React from 'react';

export default function ProviderTestReport({ report }) {
  const tone =
    report.rating === 'agent_ready'
      ? 'border-emerald-500/20 bg-emerald-500/10 text-emerald-700 dark:text-emerald-200'
      : report.rating === 'usable'
        ? 'border-blue-500/20 bg-blue-500/10 text-blue-700 dark:text-blue-200'
        : report.rating === 'chat_only'
          ? 'border-amber-500/25 bg-amber-500/10 text-amber-700 dark:text-amber-200'
          : 'border-red-500/20 bg-red-500/10 text-red-700 dark:text-red-200';
  return (
    <div className={`settings-expand-enter mt-3 rounded-xl border px-3 py-2 text-xs ${tone}`}>
      <div className="flex items-center justify-between gap-3">
        <span className="font-bold">{report.summary || '测试完成'}</span>
        <span className="text-[11px] opacity-70">{durationLabel(report.results)}</span>
      </div>
      <div className="mt-2 grid gap-1.5">
        {(report.results || []).map((item) => (
          <div key={item.name} className="flex min-w-0 items-start justify-between gap-3 rounded-lg bg-white/45 px-2 py-1.5 dark:bg-black/10">
            <div className="min-w-0">
              <div className="font-semibold">{item.label}</div>
              {item.error && <div className="mt-0.5 break-words text-[11px] opacity-80">{item.error}</div>}
            </div>
            <div className="shrink-0 text-right text-[11px] font-bold">
              {item.status === 'passed' ? '通过' : '失败'}
              <div className="font-normal opacity-70">{item.duration_ms || 0}ms</div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function durationLabel(results = []) {
  const total = results.reduce((sum, item) => sum + (Number(item.duration_ms) || 0), 0);
  return total ? `${total}ms` : '';
}
