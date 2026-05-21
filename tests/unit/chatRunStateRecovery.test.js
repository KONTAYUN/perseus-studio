import assert from 'node:assert/strict';
import test from 'node:test';

import {
  appendRunProgressItem,
  appendRunTerminalMessage,
  mergeAssistantMessageStarted,
} from '../../src/hooks/chatStateUtils.js';
import { applyLatestRunState } from '../../src/hooks/chatRunStateRecovery.js';

test('appendRunTerminalMessage restores recoverable failed run state', () => {
  const messages = appendRunTerminalMessage([
    { id: 'u1', role: 'user', content: '写十集', conversation_id: 'conv-1' },
    { id: 'a1', role: 'assistant', content: '已写第三十一集。', conversation_id: 'conv-1' },
  ], {
    id: 'run-1',
    conversation_id: 'conv-1',
    user_message_id: 'u1',
    status: 'failed',
    error_message: '模型供应商 TLS 连接中断',
    diagnostic_log_id: 'diag-1',
    completed_at: '2026-05-21T06:08:50Z',
  });

  assert.equal(messages.length, 3);
  assert.equal(messages[2].error, true);
  assert.equal(messages[2].resumableRunId, 'run-1');
  assert.equal(messages[2].diagnosticLogId, 'diag-1');
  assert.match(messages[2].content, /TLS/);
  assert.equal(appendRunTerminalMessage(messages, { id: 'run-1', conversation_id: 'conv-1', user_message_id: 'u1', status: 'failed' }).length, 3);
});

test('applyLatestRunState restores running live tool progress from persisted events', () => {
  const messages = applyLatestRunState([
    { id: 'u1', role: 'user', content: '写续集', conversation_id: 'conv-1' },
    { id: 'a1', role: 'assistant', content: '现在开始逐集写入。', conversation_id: 'conv-1' },
  ], {
    id: 'run-1',
    conversation_id: 'conv-1',
    user_message_id: 'u1',
    status: 'running',
    stage: 'executing_tool',
    updated_at: '2026-05-21T06:08:14Z',
  }, [
    {
      type: 'tool_call_started',
      payload: {
        tool_call_id: 'tc-save-31',
        tool_name: 'save_workspace_file',
        target_summary: '白凌传·造化篇续集/第三十一集.txt',
      },
    },
  ]);

  assert.equal(messages.length, 3);
  assert.equal(messages[2].pending, true);
  assert.equal(messages[2].runId, 'run-1');
  assert.equal(messages[2].progressItems[0].text, '正在保存 白凌传·造化篇续集/第三十一集.txt');
  assert.equal(messages[2].loadingState.visibleText, '正在保存 白凌传·造化篇续集/第三十一集.txt');
});

test('applyLatestRunState falls back to run stage when no tool is active', () => {
  const messages = applyLatestRunState([
    { id: 'u1', role: 'user', content: '写续集', conversation_id: 'conv-1' },
  ], {
    id: 'run-1',
    conversation_id: 'conv-1',
    user_message_id: 'u1',
    status: 'running',
    stage: 'calling_model',
  }, []);

  assert.equal(messages[1].pending, true);
  assert.equal(messages[1].progressItems.length, 0);
  assert.equal(messages[1].loadingState.visibleText, '正在等待模型响应');
});

test('run-id based pending shell continues receiving later updates after recovery', () => {
  const restored = applyLatestRunState([
    { id: 'u1', role: 'user', content: '写续集', conversation_id: 'conv-1' },
  ], {
    id: 'run-1',
    conversation_id: 'conv-1',
    user_message_id: 'u1',
    status: 'running',
    stage: 'executing_tool',
  }, [
    {
      type: 'tool_call_started',
      payload: {
        run_id: 'run-1',
        tool_call_id: 'tc-1',
        tool_name: 'save_workspace_file',
        target_summary: 'chapter-31.txt',
      },
    },
  ]);

  const next = appendRunProgressItem(restored, 'temp_run_1', {
    id: 'tool_call_completed:tc-1',
    runId: 'run-1',
    type: 'tool_call_completed',
    text: '已保存 chapter-31.txt',
    toolName: 'save_workspace_file',
    status: 'success',
  });

  assert.equal(next[1].progressItems[0].text, '正在保存 chapter-31.txt');
  assert.equal(next[1].progressItems.length, 2);
  assert.equal(next[1].runId, 'run-1');
});

test('restored run shell is replaced by later assistant start using run id', () => {
  const restored = applyLatestRunState([
    { id: 'u1', role: 'user', content: '继续写', conversation_id: 'conv-1' },
  ], {
    id: 'run-1',
    conversation_id: 'conv-1',
    user_message_id: 'u1',
    status: 'running',
    stage: 'calling_model',
  }, []);

  const next = mergeAssistantMessageStarted(restored, 'old-temp-shell', {
    id: 'a1',
    role: 'assistant',
    content: '',
    status: 'streaming',
    conversation_id: 'conv-1',
  }, { runId: 'run-1' });

  assert.equal(next.length, 2);
  assert.equal(next[1].id, 'a1');
  assert.equal(next[1].runId, 'run-1');
  assert.equal(next[1].pending, true);
});
