import React from 'react';
import { Checkbox, Field, Input, Select } from './SettingsUI';
import { ModelProviderReasoningField } from './ModelProviderReasoningControls';
import { CONTEXT_LENGTH_PRESETS, activeContextPresetId, applyContextPreset, contextTemplateSummary, formatContextTokens, getContextPreset, isRecommendedContextTemplate } from './modelContextPresets';

export default function ModelContextSettings({ draft, setDraft }) {
  const contextPresetId = activeContextPresetId(draft);
  const contextUsesRecommendedTemplate = isRecommendedContextTemplate(draft);
  const selectedContextPreset = contextPresetId === 'custom'
    ? { label: formatContextTokens(draft.context_window_tokens), caption: '自定义' }
    : getContextPreset(contextPresetId);
  return (<details className="settings-advanced">
    <summary>高级设置<span>思考能力、上下文与协议兼容性</span></summary>
    <ModelProviderReasoningField value={draft.reasoning_capability} onChange={(value) => setDraft({ ...draft,
      reasoning_capability: value, reasoning_capability_source: value === 'auto' ? '' : 'explicit_override', reasoning_profile_levels: '' })} />
          <div className="mt-4 rounded-2xl border border-gray-200 bg-white/70 p-3 dark:border-white/10 dark:bg-white/[0.03]">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <div className="text-xs font-bold text-gray-600 dark:text-gray-300">上下文管理</div>
                <div className="mt-1 text-[11px] leading-4 text-gray-400 dark:text-gray-500">
                  选择大致上下文长度，系统会套用推荐压缩模板；完整聊天记录仍会保留。
                </div>
              </div>
              {!contextUsesRecommendedTemplate && (
                <span className="shrink-0 rounded-full bg-amber-50 px-2 py-0.5 text-[10px] font-bold text-amber-600 dark:bg-amber-500/10 dark:text-amber-200">
                  已自定义
                </span>
              )}
            </div>
            <Field label="上下文长度">
              <Select
                value={contextPresetId}
                onChange={(event) => setDraft((current) => applyContextPreset(current, event.target.value))}
              >
                {contextPresetId === 'custom' && (
                  <option value="custom" disabled>
                    {selectedContextPreset.label} · {selectedContextPreset.caption}
                  </option>
                )}
                {CONTEXT_LENGTH_PRESETS.map((preset) => (
                  <option key={preset.id} value={preset.id}>
                    {preset.label} · {preset.caption}
                  </option>
                ))}
              </Select>
            </Field>
            {contextPresetId === 'custom' && (
              <div className="settings-expand-enter mt-2 rounded-xl bg-amber-50/80 px-3 py-2 text-[11px] leading-4 text-amber-700 dark:bg-amber-500/10 dark:text-amber-200">
                当前窗口为 {formatContextTokens(draft.context_window_tokens)}，不属于快捷模板。可在高级设置里继续精调。
              </div>
            )}
            <div className="mt-3 flex items-center justify-between gap-3 border-t border-gray-200/70 pt-3 dark:border-white/10">
              <div className="min-w-0 text-[11px] font-semibold leading-4 text-gray-500 dark:text-gray-400">
                {contextTemplateSummary(draft)}
              </div>
            </div>
              <div className="settings-expand-enter mt-3 border-t border-gray-200/70 pt-3 dark:border-white/10">
                <div className="grid grid-cols-2 gap-3">
                  <Field label="窗口 tokens">
                    <Input
                      type="number"
                      min="0"
                      value={draft.context_window_tokens}
                      onChange={(e) => setDraft({ ...draft, context_window_tokens: Number(e.target.value) || 0 })}
                      placeholder="0 自动"
                    />
                  </Field>
                  <Field label="触发水位 %">
                    <Input
                      type="number"
                      min="50"
                      max="95"
                      value={draft.auto_compact_threshold_percent}
                      onChange={(e) => setDraft({ ...draft, auto_compact_threshold_percent: Number(e.target.value) || 80 })}
                    />
                  </Field>
                </div>
                {draft.protocol === 'openai' && (
                  <Field label="系统消息布局">
                    <Select value={draft.system_message_mode} onChange={(event) => setDraft({ ...draft, system_message_mode: event.target.value })}>
                      <option value="single">合并为一条（兼容模式）</option>
                      <option value="multiple">保留多条（供应商明确支持）</option>
                    </Select>
                  </Field>
                )}
                <Field label="保留缓冲 tokens">
                  <Input
                    type="number"
                    min="0"
                    value={draft.compact_reserved_tokens}
                    onChange={(e) => setDraft({ ...draft, compact_reserved_tokens: Number(e.target.value) || 0 })}
                    placeholder="0 自动"
                  />
                </Field>
                <div className="mt-3 flex flex-wrap gap-3">
                  <Checkbox
                    label="自动压缩"
                    checked={draft.auto_compact_enabled}
                    onChange={(value) => setDraft({ ...draft, auto_compact_enabled: value })}
                  />
                  <Checkbox
                    label="压缩旧工具输出"
                    checked={draft.prune_tool_outputs_enabled}
                    onChange={(value) => setDraft({ ...draft, prune_tool_outputs_enabled: value })}
                  />
                </div>
              </div>
          </div>

  </details>);
}
