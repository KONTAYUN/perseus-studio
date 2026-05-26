import assert from 'node:assert/strict';
import test from 'node:test';

import {
  normalizeThinkingModeForModel,
  supportsNativeReasoning,
  thinkingModeLabelForModel,
  thinkingModeOptionsForModel,
} from '../../src/hooks/modelReasoningUtils.js';

test('thinkingModeOptionsForModel uses provider-native mode sets', () => {
  assert.deepEqual(
    thinkingModeOptionsForModel({
      thinking_profile: { wire_family: 'openai_reasoning', levels: ['light', 'standard', 'deep', 'expert'] },
    }).map((item) => item.value),
    ['light', 'standard', 'deep', 'expert']
  );
  assert.deepEqual(
    thinkingModeOptionsForModel({
      thinking_profile: { wire_family: 'anthropic_thinking', levels: ['standard', 'deep', 'expert'] },
    }).map((item) => item.value),
    ['standard', 'deep', 'expert']
  );
  assert.deepEqual(
    thinkingModeOptionsForModel({ thinking_profile: { wire_family: 'none', levels: [] } }).map((item) => item.value),
    []
  );
  assert.deepEqual(
    thinkingModeOptionsForModel({
      thinking_profile: { wire_family: 'openai_reasoning', levels: ['light', 'deep'] },
    }).map((item) => item.value),
    ['light', 'deep']
  );
});

test('normalizeThinkingModeForModel trims unsupported modes on model switch', () => {
  const openai = {
    thinking_profile: { wire_family: 'openai_reasoning', levels: ['light', 'standard', 'deep', 'expert'], default: 'standard' },
  };
  const anthropic = {
    thinking_profile: { wire_family: 'anthropic_thinking', levels: ['standard', 'deep', 'expert'], default: 'standard' },
  };
  const none = { thinking_profile: { wire_family: 'none', levels: [] } };
  assert.equal(normalizeThinkingModeForModel('expert', openai), 'expert');
  assert.equal(normalizeThinkingModeForModel('light', anthropic), 'standard');
  assert.equal(normalizeThinkingModeForModel('deep', none), 'standard');
  assert.equal(supportsNativeReasoning(none), false);
  assert.equal(thinkingModeLabelForModel('light', openai), '轻量');
});
