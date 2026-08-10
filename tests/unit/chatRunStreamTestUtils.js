import { waitForAgentRunStream } from '../../src/hooks/chatRunStream.js';

export class MockEventSource {
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

export function installRunStreamTestGlobals() {
  const originalEventSource = globalThis.EventSource;
  const originalWindow = globalThis.window;
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
  return () => {
    globalThis.EventSource = originalEventSource;
    globalThis.window = originalWindow;
  };
}

export function pendingRunShellMessage(runId) {
  return {
    id: 'temp_assistant',
    role: 'assistant',
    pending: true,
    runId,
    activeMessageId: '',
    kind: 'run_shell',
    content: '',
    progressItems: [],
  };
}

export function startRunStreamForTest({
  conversationID,
  getMessages,
  getSteerItems = () => [],
  runId,
  setMessages,
  onAssistantPreviewEvent = async () => {},
  setSteerItems = () => {},
  thinkingId = 'temp_assistant',
  userMessageId,
}) {
  return waitForAgentRunStream({
    control: {},
    conversationID,
    currentConversationIdRef: { current: conversationID },
    userMessageId,
    loadConversations: async () => {},
    onAssistantToolEvents: async () => {},
    onAssistantPreviewEvent,
    onTasksChanged: () => {},
    runId,
    selectedModel: null,
    setAgentRecoveryNotice: () => {},
    setChatError: () => {},
    setContextCompactionNotice: () => {},
    setContextUsage: () => {},
    setConversationMessages: (updater) => {
      setMessages(typeof updater === 'function' ? updater(getMessages()) : updater);
    },
    setRunSteerItems: (updater) => {
      setSteerItems(typeof updater === 'function' ? updater(getSteerItems()) : updater);
    },
    setRunSteerNotice: () => {},
    thinkingId,
  });
}

export function flushRunStreamMicrotasks() {
  return new Promise((resolve) => setImmediate(resolve));
}
