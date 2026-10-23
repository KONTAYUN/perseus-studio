import assert from 'node:assert/strict';
import test from 'node:test';
import { createRunSubmission, submitRunRequest } from '../../src/hooks/chatRunSubmission.js';
import { createChatRegenerateResumeActions } from '../../src/hooks/chatRegenerateResumeActions.js';
import { apiGet } from '../../src/api/client.js';

test('stop during acceptance cancels the accepted run before streaming', async () => {
  const original = globalThis.fetch;
  const control = createRunSubmission('conv');
  let release;
  const accepted = new Promise((resolve) => { release = resolve; });
  const calls = [];
  globalThis.fetch = async (url, options) => { calls.push([url, options]); return Response.json({}); };
  try {
    const pending = submitRunRequest(control, async (options) => {
      assert.equal(options.headers['Idempotency-Key'], control.requestId);
      return accepted;
    });
    control.abort(); control.abort();
    release({ run: { id: 'run' }, conversation: { id: 'conv' } });
    await assert.rejects(pending, { name: 'AbortError' });
    assert.equal(calls.length, 1);
    assert.equal(calls[0][0], '/api/agent-runs/run/cancel');
  } finally { globalThis.fetch = original; }
});

test('lost submission response recovers the same run and committed input', async () => {
  const original = globalThis.fetch;
  const control = createRunSubmission('conv');
  globalThis.fetch = async (url) => {
    assert.equal(url, `/api/agent-runs/by-request/${control.requestId}`);
    return Response.json({ run: { id: 'run', conversation_id: 'conv' }, user_message: { id: 'user-msg' }, conversation: { id: 'conv' } });
  };
  try {
    const result = await submitRunRequest(control, async () => { throw new TypeError('connection lost'); });
    assert.equal(result.run.id, 'run');
    assert.equal(result.user_message.id, 'user-msg');
  } finally { globalThis.fetch = original; }
});

test('API timeout and caller cancellation preserve distinct causes', async () => {
  const original = globalThis.fetch;
  globalThis.fetch = async (_url, { signal }) => new Promise((_resolve, reject) => {
    if (signal.aborted) reject(signal.reason);
    else signal.addEventListener('abort', () => reject(signal.reason), { once: true });
  });
  try {
    await assert.rejects(apiGet('/slow', { timeoutMs: 10 }), { name: 'TimeoutError' });
    const controller = new AbortController();
    const pending = apiGet('/slow', { signal: controller.signal });
    controller.abort();
    await assert.rejects(pending, { name: 'AbortError' });
  } finally { globalThis.fetch = original; }
});

test('lost resume response restores only its own durable sequence barrier', async () => {
  const original = globalThis.fetch;
  const control = createRunSubmission('conv');
  let requestId = control.requestId;
  globalThis.fetch = async (url) => Response.json(url.endsWith('/run')
    ? { id: 'run', conversation_id: 'conv', status: 'running' }
    : { items: [{ seq: 42, payload_json: JSON.stringify({ request_id: requestId }) }] });
  try {
    const submit = async () => { throw new TypeError('response lost'); };
    const recovered = await submitRunRequest(control, submit, 'run');
    assert.equal(recovered.resume_event.seq, 42);
    requestId = 'an-earlier-resume';
    await assert.rejects(submitRunRequest(control, submit, 'run'), /response lost/);
  } finally { globalThis.fetch = original; }
});

function resumeHarness() {
  let messages = [], resumable, error;
  const currentConversationIdRef = { current: 'conv' };
  const activeChatAbortRef = { current: null };
  const actions = createChatRegenerateResumeActions({
    activeChatAbortRef, currentConversationId: 'conv', currentConversationIdRef,
    isChatSending: false, resumableRun: { runId: 'run', conversationId: 'conv' },
    setChatError(value) { error = value; }, setResumableRun(value) { resumable = value; },
    setIsChatSending() {}, setConversationMessages(update) { messages = update(messages); },
    waitForAgentRun() { throw new Error('must not stream a rejected resume'); },
  });
  return { ...actions, currentConversationIdRef, messages: () => messages, resumable: () => resumable, error: () => error };
}

test('rejected resume preserves a recovery entry', async () => {
  const original = globalThis.fetch, originalError = console.error;
  globalThis.fetch = async () => Response.json({ error: 'temporarily unavailable' }, { status: 409 });
  console.error = () => {};
  try {
    const h = resumeHarness(); await h.continueAgentRun();
    assert.equal(h.resumable().runId, 'run');
    assert.equal(h.messages().at(-1).resumableRunId, 'run');
  } finally { globalThis.fetch = original; console.error = originalError; }
});

test('late resume failure cannot write error or placeholder into another conversation', async () => {
  const original = globalThis.fetch, originalError = console.error;
  const h = resumeHarness();
  globalThis.fetch = async () => {
    h.currentConversationIdRef.current = 'other';
    return Response.json({ error: 'temporarily unavailable' }, { status: 409 });
  };
  console.error = () => {};
  try {
    await h.continueAgentRun();
    assert.equal(h.error(), '');
    assert.equal(h.resumable(), null);
    assert.equal(h.messages().at(-1).pending, true);
  } finally { globalThis.fetch = original; console.error = originalError; }
});
