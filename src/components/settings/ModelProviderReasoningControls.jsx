import React from 'react';
import { Field, Select } from './SettingsUI';

export function ModelProviderReasoningField({ value, onChange }) {
  return (
    <Field label="原生思考能力">
      <Select value={value || 'auto'} onChange={(e) => onChange(e.target.value)}>
        <option value="auto">按来源识别</option>
        <option value="disabled">关闭</option>
        <option value="openai_reasoning">OpenAI reasoning_effort</option>
        <option value="anthropic_adaptive">Anthropic thinking</option>
      </Select>
    </Field>
  );
}

export function reasoningCapabilityLabel(value, source = '') {
  switch (value) {
    case 'disabled':
      return '关闭';
    case 'openai_reasoning':
      return source === 'provider_discovery' ? 'OpenAI · 发现' : 'OpenAI';
    case 'anthropic_adaptive':
      return source === 'provider_discovery' ? 'Anthropic · 发现' : 'Anthropic';
    default:
      return '来源';
  }
}

export function reasoningSupportLabel(profile = {}) {
  const levels = Array.isArray(profile?.levels) ? profile.levels : [];
  if (levels.length > 0) {
    return `${levels.length} 档`;
  }
  return '默认';
}

export function modelReasoningDraft(model = {}) {
  return {
    reasoning_capability: model.reasoning_capability || 'auto',
    reasoning_capability_source: model.reasoning_capability_source || '',
    reasoning_profile_levels: model.reasoning_profile_levels || '',
  };
}
