import assert from 'node:assert/strict';
import test from 'node:test';

import {
  appendRunProgressItem,
  appendUniqueProgressItem,
  buildAgentRecoveryNotice,
  buildAssistantRunShell,
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
import { mergeConversationSnapshotPreservingActive } from '../../src/hooks/chatMessageMerge.js';
import {
  RUN_STEER_STATUS,
  appendRunSteerItem,
  buildRunSteerItem,
  buildAppliedRunSteerItemsFromEvents,
  markRunSteerApplied,
  markRunSteerFailed,
  markRunSteerReceived,
  mergeRunSteerItemsWithMessages,
  pendingRunSteerPreviewItems,
  removeRunSteerItems,
  runSteerStatusLabel,
  visibleRunSteerTimelineItems,
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
    runId: '',
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

test('assistant message start removes duplicate shell for the same run when persisted streaming draft already exists', () => {
  const started = mergeAssistantMessageStarted([
    { id: 'u1', role: 'user', content: '继续', conversation_id: 'conv-1' },
    { id: 'a1', role: 'assistant', status: 'streaming', content: '', pending: true, activeMessageId: 'a1', conversation_id: 'conv-1' },
    buildAssistantRunShell({
      id: 'run-1',
      conversationId: 'conv-1',
      runId: 'run-1',
    }),
  ], 'temp-assistant', {
    id: 'a1',
    role: 'assistant',
    status: 'streaming',
    content: '',
    conversation_id: 'conv-1',
  }, { runId: 'run-1' });

  assert.deepEqual(started.map((item) => item.id), ['u1', 'a1']);
  assert.equal(started[1].pending, true);
  assert.equal(started[1].runId, 'run-1');
});

test('assistant segments from the same run stay as separate immutable messages', () => {
  const first = mergeAssistantMessageCompleted(
    mergeAssistantMessageStarted(
      [
        { id: 'u1', role: 'user', content: '继续', conversation_id: 'conv-1' },
        buildAssistantRunShell({
          id: 'shell-1',
          conversationId: 'conv-1',
          runId: 'run-1',
        }),
      ],
      'shell-1',
      { id: 'a1', role: 'assistant', status: 'streaming', content: '', conversation_id: 'conv-1' },
      { runId: 'run-1' }
    ),
    'shell-1',
    { id: 'a1', role: 'assistant', content: '第一段', attachments: [], conversation_id: 'conv-1' },
    { runId: 'run-1' }
  );

  const second = mergeAssistantMessageCompleted(
    mergeAssistantMessageStarted(
      first,
      'shell-2',
      { id: 'a2', role: 'assistant', status: 'streaming', content: '', conversation_id: 'conv-1' },
      { runId: 'run-1' }
    ),
    'shell-2',
    { id: 'a2', role: 'assistant', content: '第二段', attachments: [], conversation_id: 'conv-1' },
    { runId: 'run-1' }
  );

  assert.deepEqual(second.map((item) => item.id), ['u1', 'a1', 'a2']);
  assert.equal(second[1].content, '第一段');
  assert.equal(second[2].content, '第二段');
  assert.equal(second[1].runId, 'run-1');
  assert.equal(second[2].runId, 'run-1');
});

test('conversation snapshot merge preserves newer active stream text', () => {
  const merged = mergeConversationSnapshotPreservingActive([
    {
      id: 'a1',
      role: 'assistant',
      content: '已经流出来的文字更长',
      pending: true,
      activeMessageId: 'a1',
      runId: 'run-1',
      progressItems: [{ id: 'p1', text: '正在保存文件' }],
      created_at: '2026-05-21T00:00:01Z',
    },
  ], [
    {
      id: 'a1',
      role: 'assistant',
      content: '已经流出来',
      pending: false,
      attachments: [{ tool_name: 'save_workspace_file' }],
      created_at: '2026-05-21T00:00:01Z',
    },
  ]);

  assert.equal(merged.length, 1);
  assert.equal(merged[0].content, '已经流出来的文字更长');
  assert.equal(merged[0].pending, true);
  assert.equal(merged[0].runId, 'run-1');
  assert.deepEqual(merged[0].attachments, [{ tool_name: 'save_workspace_file' }]);
  assert.equal(merged[0].progressItems[0].text, '正在保存文件');
});

test('conversation snapshot merge accepts terminal snapshots after stream state is no longer active', () => {
  const merged = mergeConversationSnapshotPreservingActive([
    {
      id: 'a1',
      role: 'assistant',
      content: '旧的本地文字',
      pending: false,
      created_at: '2026-05-21T00:00:01Z',
    },
  ], [
    {
      id: 'a1',
      role: 'assistant',
      content: '服务端最终文字',
      pending: false,
      created_at: '2026-05-21T00:00:01Z',
    },
  ]);

  assert.equal(merged[0].content, '服务端最终文字');
  assert.equal(merged[0].pending, false);
});

test('conversation snapshot merge does not reorder active local run state by timestamp', () => {
  const merged = mergeConversationSnapshotPreservingActive([
    {
      id: 'u1',
      role: 'user',
      content: '第一轮',
      created_at: '2026-05-21T00:00:00Z',
    },
    {
      id: 'run_error_run-1',
      role: 'assistant',
      content: '上一轮失败',
      error: true,
      resumableRunId: 'run-1',
      created_at: '2026-05-21T00:00:05Z',
    },
    {
      id: 'u2',
      role: 'user',
      content: '第二轮',
      created_at: '2026-05-21T00:00:01Z',
    },
  ], [
    {
      id: 'u1',
      role: 'user',
      content: '第一轮',
      created_at: '2026-05-21T00:00:00Z',
    },
    {
      id: 'u2',
      role: 'user',
      content: '第二轮',
      created_at: '2026-05-21T00:00:01Z',
    },
  ]);

  assert.deepEqual(merged.map((item) => item.id), ['u1', 'run_error_run-1', 'u2']);
});

test('conversation snapshot merge appends new snapshot messages without timestamp resorting', () => {
  const merged = mergeConversationSnapshotPreservingActive([
    {
      id: 'u1',
      role: 'user',
      content: '第一轮',
      created_at: '2026-05-21T00:00:00Z',
    },
    {
      id: 'run_error_run-1',
      role: 'assistant',
      content: '上一轮失败',
      error: true,
      resumableRunId: 'run-1',
      created_at: '2026-05-21T00:00:05Z',
    },
  ], [
    {
      id: 'u1',
      role: 'user',
      content: '第一轮',
      created_at: '2026-05-21T00:00:00Z',
    },
    {
      id: 'u2',
      role: 'user',
      content: '第二轮',
      created_at: '2026-05-21T00:00:01Z',
    },
  ]);

  assert.deepEqual(merged.map((item) => item.id), ['u1', 'run_error_run-1', 'u2']);
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
    conversationId: 'conv-1',
    runId: 'run-1',
  }));
  const received = markRunSteerReceived(initial, {
    client_steer_id: 'client-1',
    steer_id: 'steer-1',
    content_preview: '请先停一下',
  });
  assert.equal(received[0].status, RUN_STEER_STATUS.received);
  assert.equal(received[0].steerId, 'steer-1');
  assert.equal(received[0].conversation_id, 'conv-1');
  assert.equal(received[0].runId, 'run-1');

  const applied = markRunSteerApplied(received, {
    steer_id: 'steer-1',
  });
  assert.equal(applied[0].status, RUN_STEER_STATUS.applied);
  assert.equal(applied[0].conversation_id, 'conv-1');
  assert.equal(applied[0].runId, 'run-1');
});

test('received run steer stays visible in composer preview until applied', () => {
  const initial = appendRunSteerItem([], buildRunSteerItem({
    clientSteerId: 'client-1',
    content: '先别改结尾',
    conversationId: 'conv-1',
    runId: 'run-1',
  }));
  const received = markRunSteerReceived(initial, {
    client_steer_id: 'client-1',
    steer_id: 'steer-1',
    content_preview: '先别改结尾',
  });
  assert.equal(pendingRunSteerPreviewItems(received).length, 1);
  assert.equal(mergeRunSteerItemsWithMessages([], received).length, 0);

  const applied = markRunSteerApplied(received, { steer_id: 'steer-1' });
  assert.equal(pendingRunSteerPreviewItems(applied).length, 0);
  assert.equal(mergeRunSteerItemsWithMessages([], applied).length, 1);
});

test('visible run steer items stay scoped to the selected conversation', () => {
  const items = [
    {
      id: 'steer-a',
      conversation_id: 'conv-a',
      status: RUN_STEER_STATUS.applied,
      content: 'a',
    },
    {
      id: 'steer-b',
      conversation_id: 'conv-b',
      status: RUN_STEER_STATUS.applied,
      content: 'b',
    },
  ];

  assert.deepEqual(visibleRunSteerTimelineItems(items, ''), []);
  assert.deepEqual(visibleRunSteerTimelineItems(items, 'conv-a'), [items[0]]);
});

test('applied steer events can be restored from durable run events', () => {
  const items = buildAppliedRunSteerItemsFromEvents([
    {
      id: 'evt-1',
      run_id: 'run-1',
      conversation_id: 'conv-1',
      type: 'agent_steer_received',
      payload: {
        steer_id: 'steer-1',
        client_steer_id: 'client-1',
        content: '请先整理目录',
        created_at: '2026-05-21T06:08:00Z',
      },
    },
    {
      id: 'evt-2',
      run_id: 'run-1',
      conversation_id: 'conv-1',
      type: 'agent_steer_applied',
      payload: {
        steer_id: 'steer-1',
        client_steer_id: 'client-1',
        created_at: '2026-05-21T06:08:10Z',
      },
    },
  ], {
    conversationId: 'conv-1',
    runId: 'run-1',
  });

  assert.equal(items.length, 1);
  assert.equal(items[0].status, RUN_STEER_STATUS.applied);
  assert.equal(items[0].conversation_id, 'conv-1');
  assert.equal(items[0].runId, 'run-1');
  assert.equal(items[0].steerId, 'steer-1');
  assert.equal(items[0].content, '请先整理目录');
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
