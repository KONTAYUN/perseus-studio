import assert from 'node:assert/strict';
import test from 'node:test';

import {
  MockEventSource,
  flushRunStreamMicrotasks,
  installRunStreamTestGlobals,
  pendingRunShellMessage,
  startRunStreamForTest,
} from './chatRunStreamTestUtils.js';

test('waitForAgentRunStream forwards structured workspace edit preview events in order', async () => {
  const restoreGlobals = installRunStreamTestGlobals();
  try {
    let messages = [pendingRunShellMessage('run-preview')];
    const previews = [];
    const runPromise = startRunStreamForTest({
      conversationID: 'conv-preview',
      runId: 'run-preview',
      getMessages: () => messages,
      setMessages: (next) => {
        messages = next;
      },
      onAssistantPreviewEvent: async (event) => previews.push(event),
    });
    const source = MockEventSource.instances[0];
    source.emit('event', {
      seq: 1,
      type: 'workspace_edit_preview_started',
      payload: { preview_id: 'p1', path: 'draft.html', revision: 1, content: '<p>A</p>' },
    });
    source.emit('event', {
      seq: 2,
      type: 'workspace_edit_preview_updated',
      payload: { preview_id: 'p1', path: 'draft.html', revision: 2, content: '<p>AB</p>' },
    });
    source.emit('event', {
      seq: 3,
      type: 'workspace_edit_preview_reverted',
      payload: { preview_id: 'p1', path: 'draft.html', revision: 3, status: 'reverted' },
    });
    source.emit('event', {
      seq: 4,
      type: 'assistant_message_completed',
      payload: {
        assistant_message: {
          id: 'a-preview',
          role: 'assistant',
          content: 'done',
          conversation_id: 'conv-preview',
        },
      },
    });
    source.emit('run', { id: 'run-preview', status: 'succeeded' });
    await runPromise;

    assert.deepEqual(
      previews.map((item) => [item.type, item.revision]),
      [
        ['workspace_edit_preview_started', 1],
        ['workspace_edit_preview_updated', 2],
        ['workspace_edit_preview_reverted', 3],
      ]
    );
  } finally {
    restoreGlobals();
  }
});

test('waitForAgentRunStream resumes event stream from the last applied seq', async () => {
  const restoreGlobals = installRunStreamTestGlobals();
  try {
    let messages = [pendingRunShellMessage('run-1')];
    const runPromise = startRunStreamForTest({
      conversationID: 'conv-1',
      runId: 'run-1',
      getMessages: () => messages,
      setMessages: (next) => {
        messages = next;
      },
    });

    const first = MockEventSource.instances[0];
    first.emit('event', {
      seq: 1,
      type: 'assistant_message_started',
      payload: {
        assistant_message: {
          id: 'a1',
          role: 'assistant',
          status: 'streaming',
          content: '',
          conversation_id: 'conv-1',
        },
      },
    });
    first.emit('event', {
      seq: 2,
      type: 'assistant_message_delta',
      payload: { message_id: 'a1', delta: 'hello ' },
    });
    await flushRunStreamMicrotasks();
    first.fail();
    await flushRunStreamMicrotasks();

    assert.equal(MockEventSource.instances.length, 2);
    assert.match(MockEventSource.instances[1].url, /after_seq=2/);

    const second = MockEventSource.instances[1];
    second.emit('event', {
      seq: 2,
      type: 'assistant_message_delta',
      payload: { message_id: 'a1', delta: 'hello ' },
    });
    second.emit('event', {
      seq: 3,
      type: 'assistant_message_delta',
      payload: { message_id: 'a1', delta: 'world' },
    });
    second.emit('event', {
      seq: 4,
      type: 'assistant_message_completed',
      payload: {
        assistant_message: {
          id: 'a1',
          role: 'assistant',
          content: 'hello world',
          conversation_id: 'conv-1',
        },
      },
    });
    second.emit('run', { id: 'run-1', status: 'succeeded' });
    await runPromise;

    assert.equal(messages.find((item) => item.id === 'a1')?.content, 'hello world');
  } finally {
    restoreGlobals();
  }
});

test('waitForAgentRunStream reconnects from the last contiguous event and waits for final_seq', async () => {
  const restoreGlobals = installRunStreamTestGlobals();
  try {
    let messages = [pendingRunShellMessage('run-gap')];
    let settled = false;
    const runPromise = startRunStreamForTest({
      conversationID: 'conv-gap',
      runId: 'run-gap',
      getMessages: () => messages,
      setMessages: (next) => {
        messages = next;
      },
    }).then(() => {
      settled = true;
    });

    const first = MockEventSource.instances[0];
    first.emit('event', {
      seq: 1,
      type: 'assistant_message_started',
      payload: {
        assistant_message: {
          id: 'a-gap',
          role: 'assistant',
          status: 'streaming',
          content: '',
          conversation_id: 'conv-gap',
        },
      },
    });
    first.emit('event', {
      seq: 3,
      type: 'assistant_message_delta',
      payload: { message_id: 'a-gap', delta: 'missing' },
    });
    await flushRunStreamMicrotasks();

    assert.equal(MockEventSource.instances.length, 2);
    assert.match(MockEventSource.instances[1].url, /after_seq=1/);

    const second = MockEventSource.instances[1];
    second.emit('event', {
      seq: 2,
      type: 'assistant_message_delta',
      payload: { message_id: 'a-gap', delta: 'complete' },
    });
    second.emit('event', {
      seq: 3,
      type: 'assistant_message_completed',
      payload: {
        assistant_message: {
          id: 'a-gap',
          role: 'assistant',
          content: 'complete',
          conversation_id: 'conv-gap',
        },
      },
    });
    second.emit('run', { id: 'run-gap', status: 'succeeded', final_seq: 3 });
    await flushRunStreamMicrotasks();
    assert.equal(settled, false);

    second.emit('replay_complete', { through_seq: 3, final_seq: 3 });
    await runPromise;
    assert.equal(messages.find((item) => item.id === 'a-gap')?.content, 'complete');
  } finally {
    restoreGlobals();
  }
});

test('waitForAgentRunStream removes rejected completion preview messages', async () => {
  const restoreGlobals = installRunStreamTestGlobals();
  try {
    let messages = [pendingRunShellMessage('run-2')];
    const runPromise = startRunStreamForTest({
      conversationID: 'conv-2',
      runId: 'run-2',
      getMessages: () => messages,
      setMessages: (next) => {
        messages = next;
      },
    });

    const source = MockEventSource.instances[0];
    source.emit('event', {
      seq: 1,
      type: 'assistant_message_started',
      payload: {
        assistant_message: {
          id: 'preview-1',
          role: 'assistant',
          status: 'streaming',
          content: '',
          conversation_id: 'conv-2',
        },
      },
    });
    source.emit('event', {
      seq: 2,
      type: 'assistant_message_delta',
      payload: { message_id: 'preview-1', delta: '临时完成内容' },
    });
    source.emit('event', {
      seq: 3,
      type: 'assistant_message_discarded',
      payload: { message_id: 'preview-1', reason: 'completion_rejected' },
    });
    source.emit('event', {
      seq: 4,
      type: 'assistant_message_started',
      payload: {
        assistant_message: {
          id: 'final-1',
          role: 'assistant',
          status: 'streaming',
          content: '',
          conversation_id: 'conv-2',
        },
      },
    });
    source.emit('event', {
      seq: 5,
      type: 'assistant_message_delta',
      payload: { message_id: 'final-1', delta: '修正后的内容' },
    });
    source.emit('event', {
      seq: 6,
      type: 'assistant_message_completed',
      payload: {
        assistant_message: {
          id: 'final-1',
          role: 'assistant',
          content: '修正后的内容',
          conversation_id: 'conv-2',
        },
      },
    });
    source.emit('run', { id: 'run-2', status: 'succeeded' });
    await runPromise;

    assert.equal(messages.some((item) => item.id === 'preview-1'), false);
    assert.equal(messages.find((item) => item.id === 'final-1')?.content, '修正后的内容');
  } finally {
    restoreGlobals();
  }
});

test('waitForAgentRunStream shows a pending shell between assistant segments', async () => {
  const restoreGlobals = installRunStreamTestGlobals();
  try {
    let messages = [pendingRunShellMessage('run-3')];
    const runPromise = startRunStreamForTest({
      conversationID: 'conv-3',
      runId: 'run-3',
      getMessages: () => messages,
      setMessages: (next) => {
        messages = next;
      },
    });

    const source = MockEventSource.instances[0];
    source.emit('event', {
      seq: 1,
      type: 'assistant_message_started',
      payload: {
        assistant_message: {
          id: 'a1',
          role: 'assistant',
          status: 'streaming',
          content: '',
          conversation_id: 'conv-3',
        },
      },
    });
    source.emit('event', {
      seq: 2,
      type: 'assistant_message_delta',
      payload: { message_id: 'a1', delta: '第一段已经完成' },
    });
    source.emit('event', {
      seq: 3,
      type: 'assistant_message_completed',
      payload: {
        assistant_message: {
          id: 'a1',
          role: 'assistant',
          content: '第一段已经完成',
          conversation_id: 'conv-3',
        },
      },
    });
    await flushRunStreamMicrotasks();

    assert.deepEqual(messages.map((item) => item.id), ['a1', 'temp_assistant']);
    assert.equal(messages[0].pending, false);
    assert.equal(messages[0].status, 'final');
    assert.equal(messages[0].content, '第一段已经完成');
    assert.equal(messages[1].kind, 'run_shell');
    assert.equal(messages[1].pending, true);

    source.emit('event', {
      seq: 4,
      type: 'assistant_message_started',
      payload: {
        assistant_message: {
          id: 'a2',
          role: 'assistant',
          status: 'streaming',
          content: '',
          conversation_id: 'conv-3',
        },
      },
    });
    await flushRunStreamMicrotasks();

    assert.deepEqual(messages.map((item) => item.id), ['a1', 'a2']);
    assert.equal(messages[1].pending, true);
    assert.equal(messages[1].activeMessageId, 'a2');

    source.emit('event', {
      seq: 5,
      type: 'assistant_message_completed',
      payload: {
        assistant_message: {
          id: 'a2',
          role: 'assistant',
          content: '第二段开始输出',
          conversation_id: 'conv-3',
        },
      },
    });
    source.emit('run', { id: 'run-3', status: 'succeeded' });
    await runPromise;

    assert.deepEqual(messages.map((item) => item.id), ['a1', 'a2']);
    assert.equal(messages.some((item) => item.kind === 'run_shell'), false);
  } finally {
    restoreGlobals();
  }
});

test('waitForAgentRunStream does not keep empty completed assistant segments', async () => {
  const restoreGlobals = installRunStreamTestGlobals();
  try {
    let messages = [pendingRunShellMessage('run-4')];
    const runPromise = startRunStreamForTest({
      conversationID: 'conv-4',
      runId: 'run-4',
      getMessages: () => messages,
      setMessages: (next) => {
        messages = next;
      },
    });

    const source = MockEventSource.instances[0];
    source.emit('event', {
      seq: 1,
      type: 'assistant_message_started',
      payload: {
        assistant_message: {
          id: 'a-empty',
          role: 'assistant',
          status: 'streaming',
          content: '',
          conversation_id: 'conv-4',
        },
      },
    });
    source.emit('event', {
      seq: 2,
      type: 'assistant_message_delta',
      payload: { message_id: 'a-empty', delta: '\n' },
    });
    source.emit('event', {
      seq: 3,
      type: 'assistant_message_completed',
      payload: {
        assistant_message: {
          id: 'a-empty',
          role: 'assistant',
          content: '',
          status: 'final',
          attachments: [],
          conversation_id: 'conv-4',
          meta: { kind: 'assistant_segment' },
        },
      },
    });
    await flushRunStreamMicrotasks();

    assert.equal(messages.some((item) => item.id === 'a-empty'), false);
    assert.equal(messages.some((item) => item.kind === 'run_shell' && item.pending), true);

    source.emit('event', {
      seq: 4,
      type: 'assistant_message_started',
      payload: {
        assistant_message: {
          id: 'a-final',
          role: 'assistant',
          status: 'streaming',
          content: '',
          conversation_id: 'conv-4',
        },
      },
    });
    source.emit('event', {
      seq: 5,
      type: 'assistant_message_delta',
      payload: { message_id: 'a-final', delta: '真正回复' },
    });
    source.emit('event', {
      seq: 6,
      type: 'assistant_message_completed',
      payload: {
        assistant_message: {
          id: 'a-final',
          role: 'assistant',
          content: '真正回复',
          conversation_id: 'conv-4',
        },
      },
    });
    source.emit('run', { id: 'run-4', status: 'succeeded' });
    await runPromise;

    assert.deepEqual(messages.map((item) => item.id), ['a-final']);
    assert.equal(messages[0].content, '真正回复');
  } finally {
    restoreGlobals();
  }
});

test('waitForAgentRunStream places received steer in the message timeline before later assistant output', async () => {
  const restoreGlobals = installRunStreamTestGlobals();
  try {
    let messages = [
      {
        id: 'u1',
        role: 'user',
        content: '先创建版本3',
        conversation_id: 'conv-5',
      },
      pendingRunShellMessage('run-5'),
    ];
    let steerItems = [];

    const runPromise = startRunStreamForTest({
      conversationID: 'conv-5',
      userMessageId: 'u1',
      runId: 'run-5',
      getMessages: () => messages,
      setMessages: (next) => {
        messages = next;
      },
      getSteerItems: () => steerItems,
      setSteerItems: (next) => {
        steerItems = next;
      },
    });

    const source = MockEventSource.instances[0];
    source.emit('event', {
      seq: 1,
      type: 'assistant_message_started',
      payload: {
        assistant_message: {
          id: 'a1',
          role: 'assistant',
          status: 'streaming',
          content: '',
          conversation_id: 'conv-5',
          meta: { kind: 'assistant_segment', run_id: 'run-5' },
        },
      },
    });
    source.emit('event', {
      seq: 2,
      type: 'assistant_message_completed',
      payload: {
        assistant_message: {
          id: 'a1',
          role: 'assistant',
          content: '第一段完成',
          conversation_id: 'conv-5',
          meta: { kind: 'assistant_segment', run_id: 'run-5' },
        },
      },
    });
    await flushRunStreamMicrotasks();
    assert.deepEqual(messages.map((item) => item.id), ['u1', 'a1', 'temp_assistant']);

    source.emit('event', {
      seq: 3,
      type: 'agent_steer_received',
      payload: {
        steer_id: 'steer-1',
        client_steer_id: 'client-1',
        content: '改成警局开场',
        content_preview: '改成警局开场',
        message_id: 'steer-msg-1',
        user_message: {
          id: 'steer-msg-1',
          role: 'user',
          content: '改成警局开场',
          conversation_id: 'conv-5',
          status: 'final',
          created_at: '2026-05-21T00:00:02Z',
          meta: {
            kind: 'steer',
            status: 'received',
            run_id: 'run-5',
            steer_id: 'steer-1',
            client_steer_id: 'client-1',
          },
        },
      },
    });
    await flushRunStreamMicrotasks();

    assert.deepEqual(messages.map((item) => item.id), ['u1', 'a1', 'steer-msg-1', 'temp_assistant']);
    assert.equal(messages[2].kind, 'steer');
    assert.equal(messages[2].status, 'received');
    assert.equal(steerItems[0].messageId, 'steer-msg-1');

    source.emit('event', {
      seq: 4,
      type: 'assistant_message_started',
      payload: {
        assistant_message: {
          id: 'a2',
          role: 'assistant',
          status: 'streaming',
          content: '',
          conversation_id: 'conv-5',
          meta: { kind: 'assistant_segment', run_id: 'run-5' },
        },
      },
    });
    await flushRunStreamMicrotasks();

    assert.deepEqual(messages.map((item) => item.id), ['u1', 'a1', 'steer-msg-1', 'a2']);

    source.emit('event', {
      seq: 5,
      type: 'agent_steer_applied',
      payload: {
        steer_id: 'steer-1',
        client_steer_id: 'client-1',
        message_id: 'steer-msg-1',
        user_message: {
          id: 'steer-msg-1',
          role: 'user',
          content: '改成警局开场',
          conversation_id: 'conv-5',
          status: 'final',
          created_at: '2026-05-21T00:00:02Z',
          meta: {
            kind: 'steer',
            status: 'applied',
            run_id: 'run-5',
            steer_id: 'steer-1',
            client_steer_id: 'client-1',
          },
        },
      },
    });
    await flushRunStreamMicrotasks();

    assert.deepEqual(messages.map((item) => item.id), ['u1', 'a1', 'steer-msg-1', 'a2']);
    assert.equal(messages[2].status, 'applied');

    source.emit('event', {
      seq: 6,
      type: 'assistant_message_completed',
      payload: {
        assistant_message: {
          id: 'a2',
          role: 'assistant',
          content: '按警局开场继续',
          conversation_id: 'conv-5',
          meta: { kind: 'assistant_segment', run_id: 'run-5' },
        },
      },
    });
    source.emit('run', { id: 'run-5', status: 'succeeded' });
    await runPromise;

    assert.deepEqual(messages.map((item) => item.id), ['u1', 'a1', 'steer-msg-1', 'a2']);
    assert.equal(messages[3].content, '按警局开场继续');
  } finally {
    restoreGlobals();
  }
});

test('waitForAgentRunStream removes committed steer message after cancel event', async () => {
  const restoreGlobals = installRunStreamTestGlobals();
  try {
    let messages = [
      { id: 'u1', role: 'user', content: '继续', conversation_id: 'conv-6' },
      pendingRunShellMessage('run-6'),
    ];

    const runPromise = startRunStreamForTest({
      conversationID: 'conv-6',
      userMessageId: 'u1',
      runId: 'run-6',
      getMessages: () => messages,
      setMessages: (next) => {
        messages = next;
      },
    });

    const source = MockEventSource.instances[0];
    source.emit('event', {
      seq: 1,
      type: 'agent_steer_received',
      payload: {
        steer_id: 'steer-1',
        client_steer_id: 'client-1',
        content: '这条不要了',
        message_id: 'steer-msg-1',
        user_message: {
          id: 'steer-msg-1',
          role: 'user',
          content: '这条不要了',
          conversation_id: 'conv-6',
          status: 'final',
          created_at: '2026-05-21T00:00:02Z',
          meta: { kind: 'steer', status: 'received', run_id: 'run-6' },
        },
      },
    });
    await flushRunStreamMicrotasks();
    assert.equal(messages.some((item) => item.id === 'steer-msg-1'), true);

    source.emit('event', {
      seq: 2,
      type: 'agent_steer_canceled',
      payload: {
        steer_id: 'steer-1',
        client_steer_id: 'client-1',
        message_id: 'steer-msg-1',
        message_deleted: true,
      },
    });
    await flushRunStreamMicrotasks();

    assert.equal(messages.some((item) => item.id === 'steer-msg-1'), false);
    source.emit('event', {
      seq: 3,
      type: 'assistant_message_started',
      payload: {
        assistant_message: {
          id: 'a-final',
          role: 'assistant',
          status: 'streaming',
          content: '',
          conversation_id: 'conv-6',
          meta: { kind: 'assistant_segment', run_id: 'run-6' },
        },
      },
    });
    source.emit('event', {
      seq: 4,
      type: 'assistant_message_completed',
      payload: {
        assistant_message: {
          id: 'a-final',
          role: 'assistant',
          content: '继续',
          conversation_id: 'conv-6',
          meta: { kind: 'assistant_segment', run_id: 'run-6' },
        },
      },
    });
    source.emit('run', { id: 'run-6', status: 'succeeded' });
    await runPromise;
  } finally {
    restoreGlobals();
  }
});
