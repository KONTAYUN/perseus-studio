import assert from 'node:assert/strict';
import test from 'node:test';

import {
  activeContextPresetId,
  applyContextPreset,
  contextTemplateSummary,
  formatContextTokens,
  isRecommendedContextTemplate,
} from '../../src/components/settings/modelContextPresets.js';

test('applyContextPreset applies the 1M recommended template', () => {
  const draft = applyContextPreset({ name: 'model' }, '1m');
  assert.equal(draft.context_window_tokens, 1000000);
  assert.equal(draft.auto_compact_threshold_percent, 80);
  assert.equal(draft.compact_reserved_tokens, 128000);
  assert.equal(draft.auto_compact_enabled, true);
  assert.equal(draft.prune_tool_outputs_enabled, true);
});

test('activeContextPresetId tracks simple length choices', () => {
  assert.equal(activeContextPresetId({ context_window_tokens: 256000 }), '256k');
  assert.equal(activeContextPresetId({ context_window_tokens: 123456 }), 'custom');
});

test('isRecommendedContextTemplate detects advanced customization', () => {
  const draft = applyContextPreset({}, '512k');
  assert.equal(isRecommendedContextTemplate(draft), true);
  assert.equal(isRecommendedContextTemplate({ ...draft, auto_compact_threshold_percent: 90 }), false);
  assert.equal(isRecommendedContextTemplate({ ...draft, prune_tool_outputs_enabled: false }), false);
});

test('contextTemplateSummary formats compact labels', () => {
  assert.equal(formatContextTokens(0), '自动识别');
  assert.equal(formatContextTokens(64000), '64K');
  assert.equal(contextTemplateSummary(applyContextPreset({}, '64k')), '64K · 80% 触发 · 16K 缓冲');
});
