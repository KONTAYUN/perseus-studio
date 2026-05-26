export const REASONING_SUPPORT_NONE = 'none';
export const REASONING_SUPPORT_OPENAI = 'openai';
export const REASONING_SUPPORT_ANTHROPIC_ADAPTIVE = 'anthropic_adaptive';

export const THINKING_MODE_STANDARD = 'standard';

const MODE_LABELS = {
  light: '轻量',
  [THINKING_MODE_STANDARD]: '标准',
  deep: '深度',
  expert: '专家',
};

export function reasoningSupportForModel(model = {}) {
  const profile = model?.thinking_profile || {};
  switch (profile?.wire_family) {
    case 'openai_reasoning':
      return REASONING_SUPPORT_OPENAI;
    case 'anthropic_thinking':
      return REASONING_SUPPORT_ANTHROPIC_ADAPTIVE;
    default: {
      const support = String(model?.reasoning_support || REASONING_SUPPORT_NONE).trim();
      if (support === REASONING_SUPPORT_OPENAI || support === REASONING_SUPPORT_ANTHROPIC_ADAPTIVE) {
        return support;
      }
      return REASONING_SUPPORT_NONE;
    }
  }
}

export function thinkingModeOptionsForModel(model = {}) {
  const levels = Array.isArray(model?.thinking_profile?.levels) ? model.thinking_profile.levels : [];
  const options = levels
    .map((level) => String(level || '').trim())
    .filter((level) => level && MODE_LABELS[level])
    .map((level) => ({ value: level, label: MODE_LABELS[level] }));
  return options;
}

export function supportsNativeReasoning(model = {}) {
  return thinkingModeOptionsForModel(model).length > 0 && reasoningSupportForModel(model) !== REASONING_SUPPORT_NONE;
}

export function normalizeThinkingModeForModel(mode, model = {}) {
  const options = thinkingModeOptionsForModel(model);
  const value = String(mode || THINKING_MODE_STANDARD).trim() || THINKING_MODE_STANDARD;
  if (options.length === 0) return THINKING_MODE_STANDARD;
  if (options.some((item) => item.value === value)) return value;
  const profileDefault = String(model?.thinking_profile?.default || THINKING_MODE_STANDARD).trim();
  if (options.some((item) => item.value === profileDefault)) return profileDefault;
  return options[0]?.value || THINKING_MODE_STANDARD;
}

export function thinkingModeLabelForModel(mode, model = {}) {
  const normalized = normalizeThinkingModeForModel(mode, model);
  return thinkingModeOptionsForModel(model).find((item) => item.value === normalized)?.label || '默认';
}
