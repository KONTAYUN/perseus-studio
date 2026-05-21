import assert from 'node:assert/strict';
import test from 'node:test';

import {
  appendRunProgressItem,
  appendUniqueProgressItem,
  buildAgentRecoveryNotice,
  buildContextCompactionNotice,
  buildContextUsageLabel,
  buildContextUsageTitle,
  buildProgressItem,
  formatContextTokenCount,
  hasStreamingAssistantMessage,
  isPendingStatusText,
  mergeCommittedRunStart,
  mergeAssistantMessageCompleted,
  mergeAssistantMessageStarted,
  normalizeContextUsageSummary,
  normalizeConversationMessage,
  normalizeStoredContextUsage,
  updateLoadingState,
} from '../../src/hooks/chatStateUtils.js';
import {
  RUN_STEER_STATUS,
  appendRunSteerItem,
  buildRunSteerItem,
  markRunSteerApplied,
  markRunSteerFailed,
  markRunSteerReceived,
  mergeRunSteerItemsWithMessages,
  pendingRunSteerPreviewItems,
  removeRunSteerItems,
  runSteerStatusLabel,
  unappliedRunSteerItems,
} from '../../src/hooks/chatSteerItems.js';

test('appendUniqueProgressItem keeps progress ids unique', () => {
  const item = { id: 'a', text: 'one' };
  assert.deepEqual(appendUniqueProgressItem([item], { id: 'a', text: 'again' }), [item]);
  assert.deepEqual(appendUniqueProgressItem([item], { id: 'b', text: 'two' }), [item, { id: 'b', text: 'two' }]);
});

test('buildProgressItem maps tool calls to friendly text', () => {
  assert.deepEqual(buildProgressItem('tool_call_started', { tool_call_id: 'tc1', tool_name: 'read_workspace_text' }), {
    id: 'tool_call_started:tc1',
    type: 'tool_call_started',
    text: '正在读取文件',
    toolName: 'read_workspace_text',
    status: '',
  });
  assert.equal(
    buildProgressItem('tool_call_completed', { tool_call_id: 'tc1', tool_name: 'read_workspace_text' }).text,
    '已读取文件'
  );
  assert.equal(
    buildProgressItem('tool_call_completed', { tool_call_id: 'tc2', tool_name: 'ask_followup_question' }),
    null
  );
  assert.equal(
    buildProgressItem('user_question_requested', { tool_call_id: 'tc2', question: '要哪种语气？' }),
    null
  );
});

test('appendRunProgressItem keeps live progress on the waiting shell only', () => {
  const progress = { id: 'tool:1', text: '正在读取文件' };
  const shell = { id: 'temp', role: 'assistant', pending: true, progressItems: [] };
  const streaming = { id: 'm1', role: 'assistant', pending: true, activeMessageId: 'm1', progressItems: [] };

  assert.deepEqual(appendRunProgressItem([shell], 'temp', progress)[0].progressItems, [progress]);
  assert.deepEqual(appendRunProgressItem([streaming], 'temp', progress)[0].progressItems, [progress]);
});

test('assistant message merges keep live progress while pending and clear it on completion', () => {
  const started = mergeAssistantMessageStarted(
    [{ id: 'temp', role: 'assistant', pending: true, progressItems: [{ id: 'p1', text: '正在读取文件' }] }],
    'temp',
    { id: 'm1', role: 'assistant', status: 'streaming', content: '' }
  );

  assert.equal(started[0].id, 'm1');
  assert.equal(started[0].pending, true);
  assert.deepEqual(started[0].progressItems, [{ id: 'p1', text: '正在读取文件' }]);

  const completed = mergeAssistantMessageCompleted(
    [{ ...started[0], progressItems: [{ id: 'p2', text: '已读取文件' }] }],
    'temp',
    { id: 'm1', role: 'assistant', content: '完成', attachments: [] }
  );

  assert.equal(completed[0].pending, false);
  assert.equal(completed[0].content, '完成');
  assert.deepEqual(completed[0].progressItems, []);
});

test('committed user message anchors the assistant run shell after branch reloads', () => {
  const merged = mergeCommittedRunStart(
    [
      {
        id: 'old-assistant',
        role: 'assistant',
        content: '上一条回复',
        created_at: '2026-05-21T00:00:00Z',
      },
      {
        id: 'temp_assistant_1',
        role: 'assistant',
        pending: true,
        content: '',
        created_at: '2026-05-21T00:00:02Z',
      },
      {
        id: 'temp_user_1',
        role: 'user',
        content: '感觉还不够',
        conversation_id: 'branch-1',
        created_at: '2026-05-21T00:00:01Z',
      },
    ],
    {
      optimisticUserId: 'temp_user_1',
      thinkingId: 'temp_assistant_1',
      userMessage: {
        id: 'msg-user-1',
        role: 'user',
        content: '感觉还不够',
        conversation_id: 'branch-1',
        created_at: '2026-05-21T00:00:03Z',
      },
    }
  );

  assert.deepEqual(merged.map((item) => item.id), ['old-assistant', 'msg-user-1', 'temp_assistant_1']);
  assert.equal(merged[1].role, 'user');
  assert.equal(merged[2].role, 'assistant');
  assert.equal(merged[2].pending, true);
});

test('buildProgressItem maps context compaction events', () => {
  assert.equal(
    buildProgressItem('context_compaction_started', { reason: 'manual' }).text,
    '正在压缩上下文'
  );
  assert.equal(
    buildProgressItem('context_compaction_completed', { reason: 'auto_threshold' }).text,
    '上下文已自动压缩'
  );
});

test('buildProgressItem includes safe tool target summaries', () => {
  assert.equal(
    buildProgressItem('tool_call_started', {
      tool_call_id: 'tc1',
      tool_name: 'read_workspace_text',
      target_summary: 'drafts/chapter-1.md',
    }).text,
    '正在读取 drafts/chapter-1.md'
  );
  assert.equal(
    buildProgressItem('tool_call_completed', {
      tool_call_id: 'tc1',
      tool_name: 'save_workspace_file',
      target_summary: 'drafts/chapter-1.md',
    }).text,
    '已保存 drafts/chapter-1.md'
  );
});

test('buildProgressItem maps budget exhaustion events without connection wording', () => {
  assert.equal(
    buildProgressItem('agent_budget_exhausted', { max_rounds: 96 }).text,
    '达到轮次上限，正在总结当前进度'
  );
  assert.equal(
    buildProgressItem('agent_budget_summary_completed', { max_rounds: 96 }).text,
    '已总结当前进度，可继续本轮'
  );
});

test('buildProgressItem maps steer lifecycle events', () => {
  assert.equal(
    buildProgressItem('agent_steer_received', { steer_id: 's1' }).text,
    '已收到引导，会在下一次安全边界应用'
  );
  assert.equal(
    buildProgressItem('agent_steer_applied', { steer_id: 's1' }).text,
    '引导已应用到当前任务'
  );
});

test('run steer item stays pending until applied', () => {
  const item = buildRunSteerItem({
    clientSteerId: 'client-1',
    content: '请只处理选中的文件',
    conversationId: 'conv-1',
    runId: 'run-1',
  });
  assert.equal(item.status, RUN_STEER_STATUS.submitting);
  assert.equal(runSteerStatusLabel(item.status), '正在引导');
  assert.equal(runSteerStatusLabel(RUN_STEER_STATUS.received), '正在引导');
  assert.equal(runSteerStatusLabel(RUN_STEER_STATUS.applied), '已引导对话');
});

test('run steer item advances through received and applied events', () => {
  const initial = appendRunSteerItem([], buildRunSteerItem({
    clientSteerId: 'client-1',
    content: '请先停一下',
  }));
  const received = markRunSteerReceived(initial, {
    client_steer_id: 'client-1',
    steer_id: 'steer-1',
    content_preview: '请先停一下',
  });
  assert.equal(received[0].status, RUN_STEER_STATUS.received);
  assert.equal(received[0].steerId, 'steer-1');

  const applied = markRunSteerApplied(received, {
    steer_id: 'steer-1',
  });
  assert.equal(applied[0].status, RUN_STEER_STATUS.applied);
});

test('run steer failure stays in pending input preview, not committed timeline', () => {
  const item = buildRunSteerItem({
    clientSteerId: 'client-1',
    content: '不要改全部章节',
    createdAt: '2026-05-20T00:00:01Z',
  });
  const failed = markRunSteerFailed([item], 'client-1', 'network failed');
  assert.equal(failed[0].status, RUN_STEER_STATUS.failed);
  assert.equal(failed[0].content, '不要改全部章节');
  assert.equal(failed[0].error, 'network failed');

  const merged = mergeRunSteerItemsWithMessages([
    { id: 'm1', role: 'assistant', content: 'working', created_at: '2026-05-20T00:00:00Z' },
  ], failed);
  assert.equal(merged.length, 1);
  assert.equal(pendingRunSteerPreviewItems(failed).length, 1);
});

test('run steer item stays out of timeline until applied', () => {
  const item = buildRunSteerItem({
    clientSteerId: 'client-1',
    content: '请加强恐怖感',
    createdAt: '2026-05-20T00:00:01Z',
  });
  const received = markRunSteerReceived([item], {
    client_steer_id: 'client-1',
    steer_id: 'steer-1',
    content_preview: '请加强恐怖感',
  });
  assert.deepEqual(mergeRunSteerItemsWithMessages([], received), []);
  assert.equal(unappliedRunSteerItems(received).length, 1);

  const applied = markRunSteerApplied(received, { steer_id: 'steer-1' });
  const merged = mergeRunSteerItemsWithMessages([], applied);
  assert.equal(merged.length, 1);
  assert.equal(merged[0].kind, 'steer');
  assert.deepEqual(removeRunSteerItems(applied, ['client-1']), []);
});

test('committed run steer is ordered before the active assistant shell', () => {
  const item = markRunSteerApplied([
    buildRunSteerItem({
      clientSteerId: 'client-1',
      content: '先按我的新要求调整',
      createdAt: '2026-05-20T00:00:02Z',
    }),
  ], { client_steer_id: 'client-1', steer_id: 'steer-1' });

  const merged = mergeRunSteerItemsWithMessages([
    {
      id: 'temp-assistant',
      role: 'assistant',
      pending: true,
      content: '',
      created_at: '2026-05-20T00:00:01Z',
    },
  ], item);

  assert.equal(merged[0].kind, 'steer');
  assert.equal(merged[1].id, 'temp-assistant');
});

test('normalizeContextUsageSummary calculates display metrics', () => {
  const summary = normalizeContextUsageSummary({
    estimated_tokens: 48000,
    context_window_tokens: 100000,
    compact_reserved_tokens: 10000,
    compaction_budget_tokens: 80000,
    message_count: 12,
    tool_result_count: 4,
  });
  assert.equal(summary.usedPercent, 48);
  assert.equal(summary.remainingPercent, 52);
  assert.equal(summary.remainingTokens, 52000);
  assert.equal(formatContextTokenCount(summary.estimatedTokens), '48K');
});

test('normalizeStoredContextUsage restores persisted request metadata', () => {
  const summary = normalizeStoredContextUsage({
    usage: {
      event_id: 'evt-1',
      run_id: 'run-1',
      created_at: '2026-05-13T00:00:00Z',
      payload: {
        estimated_tokens: 64000,
        context_window_tokens: 128000,
      },
    },
  });
  assert.equal(summary.source, 'stored');
  assert.equal(summary.eventId, 'evt-1');
  assert.equal(summary.runId, 'run-1');
  assert.equal(summary.usedPercent, 50);
  assert.equal(buildContextUsageTitle(summary), '上次请求的上下文窗口');
  assert.equal(buildContextUsageLabel(summary, null), '50%');
});

test('buildContextCompactionNotice labels lifecycle events', () => {
  const notice = buildContextCompactionNotice('context_compaction_completed', {
    reason: 'auto_threshold',
    compacted_message_count: 8,
  });
  assert.equal(notice.status, 'completed');
  assert.equal(notice.text, '上下文已自动压缩');
  assert.equal(notice.compactedMessageCount, 8);
  assert.equal(buildContextUsageLabel(null, notice), '已自动压缩');
});

test('buildAgentRecoveryNotice shows retry countdown', () => {
  const notice = buildAgentRecoveryNotice('agent_recovery_retrying', {
    attempt: 1,
    max_attempts: 2,
    checkpoint_seq: 3,
    delay_ms: 1500,
  });
  assert.equal(notice.status, 'retrying');
  assert.equal(notice.attempt, 1);
  assert.equal(notice.maxAttempts, 2);
  assert.match(notice.text, /2 秒后/);
  assert.match(notice.text, /1\/2/);
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
