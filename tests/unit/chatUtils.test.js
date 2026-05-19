import assert from 'node:assert/strict';
import test from 'node:test';

import { completedActionItems, followupQuestionAttachment, formatToolLabel, loadingBubbleText, pendingFollowupQuestion, shouldReserveMessageFooter, visibleMessageAttachments } from '../../src/components/chat/chatUtils.js';
import { shouldReloadWorkspaceForToolEvents } from '../../src/hooks/ideUtils.js';

test('visibleMessageAttachments hides editor context metadata', () => {
  const visible = visibleMessageAttachments({
    attachments: [
      { type: 'phase_message', name: 'phase metadata' },
      { type: 'current_file', name: 'current.md' },
      { kind: 'editor_panels_context', name: 'editor context' },
      { type: 'workspace_file', name: 'referenced.md' },
      { tool_name: 'ask_followup_question', question: '要哪种语气？', status: 'success' },
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

test('completedActionItems derives completed labels from successful tool attachments', () => {
  const items = completedActionItems({
    role: 'assistant',
    pending: false,
    attachments: [
      { tool_call_id: 'tc1', tool_name: 'read_workspace_text', status: 'success', label: '正在翻阅底稿' },
      { tool_call_id: 'tc2', tool_name: 'read_workspace_text', status: 'success' },
      { tool_call_id: 'tc3', tool_name: 'save_workspace_file', status: 'success' },
      { tool_call_id: 'tc4', tool_name: 'run_workspace_command', status: 'failed' },
      { tool_call_id: 'tc5', tool_name: 'ask_followup_question', status: 'success', question: '要哪种语气？' },
    ],
  });

  assert.deepEqual(
    items.map((item) => item.label),
    ['已读取文件', '已保存文件']
  );
  assert.equal(formatToolLabel({ tool_name: 'web_extract', label: '正在摘录资料' }), '已读取网页');
  assert.equal(formatToolLabel({ tool_name: 'edit_workspace_text' }), '已编辑文件');
});

test('followupQuestionAttachment extracts options without adding completed action chip', () => {
  const message = {
    role: 'assistant',
    pending: false,
    attachments: [
      {
        tool_call_id: 'tc1',
        tool_name: 'ask_followup_question',
        status: 'success',
        question: '这篇文章要偏哪种语气？',
        options: ['正式', '轻松', '正式', ''],
        allow_freeform: true,
      },
    ],
  };

  assert.deepEqual(followupQuestionAttachment(message), {
    question: '这篇文章要偏哪种语气？',
    options: ['正式', '轻松'],
    allowFreeform: true,
  });
  assert.deepEqual(completedActionItems(message), []);
});

test('pendingFollowupQuestion returns only the latest unanswered assistant question', () => {
  const messages = [
    {
      id: 'a1',
      role: 'assistant',
      pending: false,
      attachments: [
        {
          tool_name: 'ask_followup_question',
          status: 'success',
          question: '旧问题？',
          options: ['A', 'B'],
        },
      ],
    },
    { id: 'u1', role: 'user', content: '已经回答旧问题' },
    {
      id: 'a2',
      role: 'assistant',
      pending: false,
      created_at: '2026-05-16T00:00:00Z',
      attachments: [
        {
          tool_name: 'ask_followup_question',
          status: 'success',
          question: '新问题？',
          options: ['C', 'D'],
          allow_freeform: false,
        },
      ],
    },
  ];

  assert.deepEqual(pendingFollowupQuestion(messages), {
    messageId: 'a2',
    createdAt: '2026-05-16T00:00:00Z',
    question: '新问题？',
    options: ['C', 'D'],
    allowFreeform: false,
  });
});

test('pendingFollowupQuestion clears once user answers after the assistant question', () => {
  assert.equal(
    pendingFollowupQuestion([
      {
        id: 'a1',
        role: 'assistant',
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

test('shouldReloadWorkspaceForToolEvents reloads for AI file writes', () => {
  assert.equal(
    shouldReloadWorkspaceForToolEvents([{ tool_name: 'save_workspace_file', status: 'success', path: 'new-file.md' }]),
    true
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
