export const CONTEXT_LENGTH_PRESETS = [
  {
    id: 'auto',
    label: '自动',
    caption: '识别',
    settings: {
      context_window_tokens: 0,
      auto_compact_threshold_percent: 80,
      compact_reserved_tokens: 0,
      auto_compact_enabled: true,
      prune_tool_outputs_enabled: true,
    },
  },
  {
    id: '64k',
    label: '64K',
    caption: '轻量',
    settings: {
      context_window_tokens: 64000,
      auto_compact_threshold_percent: 80,
      compact_reserved_tokens: 16000,
      auto_compact_enabled: true,
      prune_tool_outputs_enabled: true,
    },
  },
  {
    id: '256k',
    label: '256K',
    caption: '常用',
    settings: {
      context_window_tokens: 256000,
      auto_compact_threshold_percent: 80,
      compact_reserved_tokens: 48000,
      auto_compact_enabled: true,
      prune_tool_outputs_enabled: true,
    },
  },
  {
    id: '512k',
    label: '512K',
    caption: '长文',
    settings: {
      context_window_tokens: 512000,
      auto_compact_threshold_percent: 80,
      compact_reserved_tokens: 80000,
      auto_compact_enabled: true,
      prune_tool_outputs_enabled: true,
    },
  },
  {
    id: '1m',
    label: '1M',
    caption: '超长',
    settings: {
      context_window_tokens: 1000000,
      auto_compact_threshold_percent: 80,
      compact_reserved_tokens: 128000,
      auto_compact_enabled: true,
      prune_tool_outputs_enabled: true,
    },
  },
];

export const DEFAULT_CONTEXT_PRESET_ID = 'auto';

export function getContextPreset(id) {
  return CONTEXT_LENGTH_PRESETS.find((preset) => preset.id === id) || CONTEXT_LENGTH_PRESETS[0];
}

export function applyContextPreset(draft, presetId) {
  return {
    ...draft,
    ...getContextPreset(presetId).settings,
  };
}

export function activeContextPresetId(settings) {
  const windowTokens = Number(settings?.context_window_tokens) || 0;
  return (
    CONTEXT_LENGTH_PRESETS.find((preset) => preset.settings.context_window_tokens === windowTokens)?.id ||
    'custom'
  );
}

export function isRecommendedContextTemplate(settings) {
  const presetId = activeContextPresetId(settings);
  if (presetId === 'custom') {
    return false;
  }
  const preset = getContextPreset(presetId);
  return (
    Number(settings?.auto_compact_threshold_percent) === preset.settings.auto_compact_threshold_percent &&
    Number(settings?.compact_reserved_tokens) === preset.settings.compact_reserved_tokens &&
    settings?.auto_compact_enabled !== false &&
    settings?.prune_tool_outputs_enabled !== false
  );
}

export function formatContextTokens(value) {
  const tokens = Number(value) || 0;
  if (tokens <= 0) return '自动识别';
  if (tokens >= 1000000 && tokens % 1000000 === 0) return `${tokens / 1000000}M`;
  if (tokens >= 1000 && tokens % 1000 === 0) return `${tokens / 1000}K`;
  return tokens.toLocaleString('en-US');
}

export function contextTemplateSummary(settings) {
  const threshold = Number(settings?.auto_compact_threshold_percent) || 80;
  const reserved = Number(settings?.compact_reserved_tokens) || 0;
  const reservedLabel = reserved > 0 ? formatContextTokens(reserved) : '自动';
  return `${formatContextTokens(settings?.context_window_tokens)} · ${threshold}% 触发 · ${reservedLabel} 缓冲`;
}
