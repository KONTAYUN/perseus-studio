import assert from 'node:assert/strict';
import test from 'node:test';

import { waitForAgentRunStream } from '../../src/hooks/chatRunStream.js';

class MockEventSource {
  static instances = [];

  constructor(url) {
    this.url = url;
    this.listeners = new Map();
    this.closed = false;
    MockEventSource.instances.push(this);
  }

  addEventListener(type, handler) {
    this.listeners.set(type, handler);
  }

  emit(type, data) {
    this.listeners.get(type)?.({ data: JSON.stringify(data) });
  }

  fail() {
    this.onerror?.();
  }

  close() {
    this.closed = true;
  }
}

test('waitForAgentRunStream resumes event stream from the last applied seq', async () => {
  const originalEventSource = globalThis.EventSource;
  const originalWindow = globalThis.window;
  try {
    globalThis.EventSource = MockEventSource;
    globalThis.window = {
      setInterval: () => 0,
      clearInterval: () => {},
      setTimeout: (fn) => {
        queueMicrotask(fn);
        return 0;
      },
    };
    MockEventSource.instances = [];

    let messages = [
      {
        id: 'temp_assistant',
        role: 'assistant',
        pending: true,
        runId: 'run-1',
        activeMessageId: '',
        kind: 'run_shell',
        content: '',
        progressItems: [],
      },
    ];

    const runPromise = waitForAgentRunStream({
      control: {},
      conversationID: 'conv-1',
      currentConversationIdRef: { current: 'conv-1' },
      loadConversations: async () => {},
      onAssistantToolEvents: async () => {},
      onTasksChanged: () => {},
      runId: 'run-1',
      selectedModel: null,
      setAgentRecoveryNotice: () => {},
      setChatError: () => {},
      setContextCompactionNotice: () => {},
      setContextUsage: () => {},
      setConversationMessages: (updater) => {
        messages = typeof updater === 'function' ? updater(messages) : updater;
      },
      setRunSteerItems: () => {},
      setRunSteerNotice: () => {},
      thinkingId: 'temp_assistant',
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
    await new Promise((resolve) => setImmediate(resolve));
    first.fail();
    await new Promise((resolve) => setImmediate(resolve));

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
    globalThis.EventSource = originalEventSource;
    globalThis.window = originalWindow;
  }
});

test('waitForAgentRunStream removes rejected completion preview messages', async () => {
  const originalEventSource = globalThis.EventSource;
  const originalWindow = globalThis.window;
  try {
    globalThis.EventSource = MockEventSource;
    globalThis.window = {
      setInterval: () => 0,
      clearInterval: () => {},
      setTimeout: (fn) => {
        queueMicrotask(fn);
        return 0;
      },
    };
    MockEventSource.instances = [];

    let messages = [
      {
        id: 'temp_assistant',
        role: 'assistant',
        pending: true,
        runId: 'run-2',
        activeMessageId: '',
        kind: 'run_shell',
        content: '',
        progressItems: [],
      },
    ];

    const runPromise = waitForAgentRunStream({
      control: {},
      conversationID: 'conv-2',
      currentConversationIdRef: { current: 'conv-2' },
      loadConversations: async () => {},
      onAssistantToolEvents: async () => {},
      onTasksChanged: () => {},
      runId: 'run-2',
      selectedModel: null,
      setAgentRecoveryNotice: () => {},
      setChatError: () => {},
      setContextCompactionNotice: () => {},
      setContextUsage: () => {},
      setConversationMessages: (updater) => {
        messages = typeof updater === 'function' ? updater(messages) : updater;
      },
      setRunSteerItems: () => {},
      setRunSteerNotice: () => {},
      thinkingId: 'temp_assistant',
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
    globalThis.EventSource = originalEventSource;
    globalThis.window = originalWindow;
  }
});
