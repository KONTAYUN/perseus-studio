import assert from 'node:assert/strict';
import test from 'node:test';

import { completedActionItems, formatToolLabel, latestLiveProgressItem, loadingBubbleText, loadingStatusDisplayText, loadingStatusReserveText, pendingUserRequest, shouldReserveMessageFooter, userRequestAttachment, visibleMessageAttachments } from '../../src/components/chat/chatUtils.js';
import { shouldReloadWorkspaceForToolEvents } from '../../src/hooks/ideUtils.js';

test('visibleMessageAttachments hides editor context metadata', () => {
  const visible = visibleMessageAttachments({
    attachments: [
      { type: 'phase_message', name: 'phase metadata' },
      { type: 'current_file', name: 'current.md' },
      { kind: 'editor_panels_context', name: 'editor context' },
      { type: 'workspace_file', name: 'referenced.md' },
      { tool_name: 'request_user_input', question: '请粘贴日志', status: 'waiting_for_user' },
      { kind: 'text', name: 'uploaded.txt' },
    ],
  });

  assert.deepEqual(
    visible.map((item) => item.name),
    ['referenced.md', 'uploaded.txt']
  );
});

test('shouldReserveMessageFooter keeps spacing for visible content but not progress-only messages', () => {
  assert.equal(
    shouldReserveMessageFooter({
      pending: false,
      content: '',
      attachments: [{ type: 'selected_skill', name: 'screenwriting-master', skill_id: 'skill_demo' }],
      progressItems: [],
    }),
    true
  );

  assert.equal(
    shouldReserveMessageFooter({
      pending: false,
      content: '',
      attachments: [],
      progressItems: [{ id: 'tool:1', text: '正在准备工具箱' }],
    }),
    false
  );

  assert.equal(
    shouldReserveMessageFooter({
      pending: false,
      content: '',
      attachments: [],
      progressItems: [],
    }),
    false
  );
});

test('loadingBubbleText prefers real live tool progress over playful fallback', () => {
  const message = {
    pending: true,
    loadingState: { visibleText: '正在烹饪' },
    progressItems: [
      { id: 'internal', type: 'model_call_attempt_started', text: '正在裁剪' },
      { id: 'tool', type: 'tool_call_started', text: '正在读取 drafts/chapter-1.md' },
    ],
  };

  assert.equal(latestLiveProgressItem(message).text, '正在读取 drafts/chapter-1.md');
  assert.equal(loadingBubbleText(message), '正在读取 drafts/chapter-1.md');
});

test('loadingBubbleText does not use completed tool states as active progress', () => {
  const message = {
    pending: true,
    loadingState: { visibleText: '正在烹饪' },
    progressItems: [
      { id: 'tool_call_started:tc1', toolCallId: 'tc1', type: 'tool_call_started', text: '正在编辑 drafts/chapter-1.md' },
      { id: 'tool_call_completed:tc1', toolCallId: 'tc1', type: 'tool_call_completed', text: '已编辑 drafts/chapter-1.md' },
    ],
  };

  assert.equal(latestLiveProgressItem(message), null);
  assert.equal(loadingBubbleText(message), '正在烹饪');
});

test('loadingBubbleText falls back when progress only contains completed states', () => {
  assert.equal(
    loadingBubbleText({
      pending: true,
      loadingState: { visibleText: '正在烹饪' },
      progressItems: [{ id: 'done', type: 'tool_call_completed', text: '已读取 drafts/chapter-1.md' }],
    }),
    '正在烹饪'
  );
});

test('loadingBubbleText keeps playful fallback when no real live progress exists', () => {
  assert.equal(
    loadingBubbleText({
      pending: true,
      loadingState: { visibleText: '正在烹饪' },
      progressItems: [{ id: 'internal', type: 'model_call_attempt_started', text: '正在裁剪' }],
    }),
    '正在烹饪'
  );
});

test('completedActionItems derives completed labels from successful tool attachments', () => {
  const items = completedActionItems({
    role: 'assistant',
    pending: false,
    attachments: [
      { tool_call_id: 'tc1', tool_name: 'read_workspace_text', status: 'success', label: '正在翻阅底稿' },
      { tool_call_id: 'tc2', tool_name: 'read_workspace_text', status: 'success' },
      { tool_call_id: 'tc3', tool_name: 'apply_workspace_patch', status: 'success', status_kind: 'workspace_edit' },
      { tool_call_id: 'tc4', tool_name: 'run_workspace_command', status: 'failed' },
      { tool_call_id: 'tc5', tool_name: 'request_user_choice', status: 'waiting_for_user', question: '要哪种语气？' },
    ],
  });

  assert.deepEqual(
    items.map((item) => item.label),
    ['已读取文件', '已编辑文件']
  );
  assert.equal(formatToolLabel({ tool_name: 'web_extract', label: '正在摘录资料' }), '已读取网页');
  assert.equal(formatToolLabel({ tool_name: 'historical_write_tool', status_kind: 'workspace_edit' }), '已编辑文件');
});

test('userRequestAttachment keeps bounded choices without adding completed action chips', () => {
  const message = {
    role: 'assistant',
    kind: 'user_choice_request',
    pending: false,
    attachments: [
      {
        tool_call_id: 'tc1',
        tool_name: 'request_user_choice',
        status: 'waiting_for_user',
        question: '这篇文章要偏哪种语气？',
        options: ['正式', '轻松', '正式', ''],
      },
    ],
  };

  assert.deepEqual(userRequestAttachment(message), {
    question: '这篇文章要偏哪种语气？',
    options: ['正式', '轻松'],
    requestKind: 'choice',
    toolName: 'request_user_choice',
    status: 'waiting_for_user',
    toolCallId: 'tc1',
    runId: '',
    answerMessageId: '',
  });
  assert.deepEqual(completedActionItems(message), []);
});

test('pendingUserRequest returns only the latest unanswered assistant request', () => {
  const messages = [
    {
      id: 'a1',
      role: 'assistant',
      kind: 'user_choice_request',
      pending: false,
      attachments: [
        {
          tool_name: 'request_user_choice',
          status: 'waiting_for_user',
          question: '旧问题？',
          options: ['A', 'B'],
        },
      ],
    },
    { id: 'u1', role: 'user', content: '已经回答旧问题' },
    {
      id: 'a2',
      runId: 'run-2',
      role: 'assistant',
      kind: 'user_input_request',
      pending: false,
      created_at: '2026-05-16T00:00:00Z',
      attachments: [
        {
          tool_name: 'request_user_input',
          tool_call_id: 'tc2',
          status: 'waiting_for_user',
          question: '新问题？',
          options: ['must not render'],
        },
      ],
    },
  ];

  assert.deepEqual(pendingUserRequest(messages), {
    messageId: 'a2',
    createdAt: '2026-05-16T00:00:00Z',
    question: '新问题？',
    options: [],
    requestKind: 'input',
    toolName: 'request_user_input',
    status: 'waiting_for_user',
    toolCallId: 'tc2',
    runId: 'run-2',
    answerMessageId: '',
  });
});

test('pendingUserRequest clears once user answers after the assistant request', () => {
  assert.equal(
    pendingUserRequest([
      {
        id: 'a1',
        role: 'assistant',
        kind: 'followup_question',
        pending: false,
        attachments: [
          {
            tool_name: 'ask_followup_question',
            status: 'success',
            question: '要哪种语气？',
            options: ['正式', '轻松'],
          },
        ],
      },
      { id: 'u1', role: 'user', content: '轻松一点' },
    ]),
    null
  );
});

test('answered legacy follow-up remains visible but is no longer pending', () => {
  const message = {
    id: 'a1',
    runId: 'run-1',
    role: 'assistant',
    kind: 'followup_question',
    pending: false,
    attachments: [
      {
        tool_call_id: 'tc1',
        tool_name: 'ask_followup_question',
        status: 'answered',
        question: 'Run this?\n\n```bash\nssh server\n```',
        options: ['Run it'],
        answer_message_id: 'u1',
      },
    ],
  };

  assert.deepEqual(userRequestAttachment(message), {
    question: 'Run this?\n\n```bash\nssh server\n```',
    options: ['Run it'],
    requestKind: 'legacy_followup',
    toolName: 'ask_followup_question',
    status: 'answered',
    toolCallId: 'tc1',
    runId: 'run-1',
    answerMessageId: 'u1',
  });
  assert.equal(pendingUserRequest([message]), null);
});

test('ordinary assistant messages never render stale follow-up attachments', () => {
  const message = {
    id: 'a2',
    role: 'assistant',
    kind: 'completion_result',
    content: 'The server check is complete.',
    attachments: [
      {
        tool_call_id: 'old-question',
        tool_name: 'ask_followup_question',
        status: 'answered',
        question: 'Old question?',
        options: ['Old option'],
      },
    ],
  };

  assert.equal(userRequestAttachment(message), null);
});

test('shouldReloadWorkspaceForToolEvents reloads for AI file writes', () => {
  assert.equal(
    shouldReloadWorkspaceForToolEvents([{ type: 'workspace_file_changed', status: 'success', changed_paths: ['new-file.md'] }]),
    true
  );
  assert.equal(
    shouldReloadWorkspaceForToolEvents([{ tool_name: 'legacy_unknown', status: 'success', changed_paths: ['new-file.md'] }]),
    false
  );
  assert.equal(
    shouldReloadWorkspaceForToolEvents([{ tool_name: 'save_workspace_file', status: 'failed', path: 'new-file.md' }]),
    false
  );
});

test('loadingBubbleText keeps intentional empty typing frame', () => {
  assert.equal(
    loadingBubbleText({
      loadingState: { text: '正在整理上下文', visibleText: '', stage: 'typing' },
    }, '正在处理'),
    ''
  );
  assert.equal(loadingBubbleText({}, '正在处理'), '正在处理');
});

test('loadingStatusDisplayText reserves the empty typing frame for pending bubbles', () => {
  assert.equal(
    loadingStatusDisplayText({
      pending: true,
      loadingState: { text: '正在整理上下文', visibleText: '', stage: 'typing' },
    }),
    '\u00A0'
  );
  assert.equal(loadingStatusDisplayText({}, ''), '');
});

test('loadingStatusReserveText keeps the full pending label during empty typing frames', () => {
  assert.equal(
    loadingStatusReserveText({
      pending: true,
      loadingState: { text: '正在读取 very/long/path/chapter-1.md', visibleText: '', stage: 'typing' },
    }),
    '正在读取 very/long/path/chapter-1.md'
  );
  assert.equal(
    loadingStatusReserveText({ pending: true }, '', '正在处理'),
    '正在处理'
  );
});

test('loadingStatusReserveText keeps long live progress stable during empty frames', () => {
  assert.equal(
    loadingStatusReserveText(
      {
        pending: true,
        loadingState: { text: '正在烹饪', visibleText: '', stage: 'typing' },
        progressItems: [
          {
            id: 'agent_recovery_retrying:1',
            type: 'agent_recovery_retrying',
            text: '连接中断，10 秒后从断点继续 1/2',
          },
        ],
      },
      ''
    ),
    '连接中断，10 秒后从断点继续 1/2'
  );
});
