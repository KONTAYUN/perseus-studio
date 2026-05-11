import assert from 'node:assert/strict';
import test from 'node:test';

import {
  appendUniqueProgressItem,
  buildProgressItem,
  hasStreamingAssistantMessage,
  isPendingStatusText,
  normalizeConversationMessage,
  updateLoadingState,
} from '../../src/hooks/chatStateUtils.js';

test('appendUniqueProgressItem keeps progress ids unique', () => {
  const item = { id: 'a', text: 'one' };
  assert.deepEqual(appendUniqueProgressItem([item], { id: 'a', text: 'again' }), [item]);
  assert.deepEqual(appendUniqueProgressItem([item], { id: 'b', text: 'two' }), [item, { id: 'b', text: 'two' }]);
});

test('buildProgressItem maps tool calls to friendly text', () => {
  assert.deepEqual(buildProgressItem('tool_call_started', { tool_call_id: 'tc1', tool_name: 'read_workspace_text' }), {
    id: 'tool_call_started:tc1',
    type: 'tool_call_started',
    text: '正在翻阅底稿',
    toolName: 'read_workspace_text',
    status: '',
  });
});

test('normalizeConversationMessage marks streaming assistant as pending', () => {
  const normalized = normalizeConversationMessage({ id: 'm1', role: 'assistant', status: 'streaming' });
  assert.equal(normalized.pending, true);
  assert.equal(normalized.activeMessageId, 'm1');
  assert.equal(hasStreamingAssistantMessage([normalized]), true);
});

test('updateLoadingState advances holding loader after minimum display time', () => {
  const message = {
    pending: true,
    loadingState: { text: '正在烹饪', visibleText: '正在烹饪', stage: 'holding', activeSince: 1000 },
  };
  assert.equal(updateLoadingState(message, 3999).loadingState.stage, 'holding');
  assert.equal(updateLoadingState(message, 4000).loadingState.stage, 'deleting');
});

test('isPendingStatusText recognizes short generated waiting labels', () => {
  assert.equal(isPendingStatusText('正在检查火候'), true);
  assert.equal(isPendingStatusText('这是正式正文，不应被当成等待状态'), false);
});
