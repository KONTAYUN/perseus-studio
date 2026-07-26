import assert from 'node:assert/strict';
import test from 'node:test';

import {
  chatFailureHeading,
  chatFailureMetadata,
  formatRetryAfterLabel,
  isProviderRateLimitFailure,
  isResumeBlockedByFailure,
} from '../../src/hooks/chatErrorUtils.js';

test('chatFailureHeading reserves connection wording for transport failures', () => {
  assert.equal(chatFailureHeading('agent_protocol_violation', { resumable: true }), '模型动作协议错误');
  assert.equal(chatFailureHeading('tool_failure', { resumable: true }), '工具执行失败');
  assert.equal(chatFailureHeading('context_window_exceeded', { resumable: true }), '上下文超出模型限制');
  assert.equal(chatFailureHeading('provider_stream_interrupted', { resumable: true }), '连接中断');
  assert.equal(chatFailureHeading('provider_connection_failed', { resumable: true }), '连接中断');
});

test('chatFailureMetadata normalizes snake_case run failure fields', () => {
  const metadata = chatFailureMetadata({
    failure_class: 'provider_tpm_rate_limited',
    retry_after_ms: 60000,
    reset_at: '2026-07-10T12:00:00Z',
    provider_id: 'provider-1',
    model_id: 'model-1',
    can_resume_now: false,
    recommended_action: 'Switch model or wait.',
  });

  assert.deepEqual(metadata, {
    failureClass: 'provider_tpm_rate_limited',
    retryAfterMs: 60000,
    resetAt: '2026-07-10T12:00:00Z',
    providerId: 'provider-1',
    modelId: 'model-1',
    canResumeNow: false,
    recommendedAction: 'Switch model or wait.',
  });
});

test('isResumeBlockedByFailure blocks all provider rate limit classes during cooldown', () => {
  for (const failureClass of [
    'provider_rate_limited',
    'provider_tpm_rate_limited',
    'provider_rpm_rate_limited',
    'provider_quota_rate_limited',
    'provider_unknown_rate_limited',
  ]) {
    assert.equal(isProviderRateLimitFailure(failureClass), true, failureClass);
    assert.equal(isResumeBlockedByFailure({ failureClass, retryAfterMs: 1000 }), true, failureClass);
  }

  assert.equal(isResumeBlockedByFailure({ failureClass: 'provider_transient_http', retryAfterMs: 1000 }), false);
  assert.equal(isResumeBlockedByFailure({ failureClass: 'provider_tpm_rate_limited', retryAfterMs: 0 }), false);
  assert.equal(isResumeBlockedByFailure({ failureClass: 'provider_tpm_rate_limited', canResumeNow: false }), true);
});

test('formatRetryAfterLabel uses seconds and minutes labels', () => {
  assert.match(formatRetryAfterLabel(1500), /2/);
  assert.match(formatRetryAfterLabel(61000), /2/);
  assert.equal(typeof formatRetryAfterLabel(0, '2026-07-10T12:00:00Z'), 'string');
});
