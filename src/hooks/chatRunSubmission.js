import { apiGet, apiPost } from '../api/client.js';

// Keep the short acceptance request alive until its run identity is known.
// Stopping during submission is remembered and honored before streaming starts.
export function createRunSubmission(conversationId = '') {
  return {
    conversationId,
    requestId: globalThis.crypto.randomUUID(),
    canceled: false,
    abort() { this.canceled = true; },
  };
}

export async function submitRunRequest(control, submit, knownRunId = '') {
  let result;
  try {
    result = await submit({ headers: { 'Content-Type': 'application/json', 'Idempotency-Key': control.requestId } });
  } catch (error) {
    // A lost HTTP response does not imply that the server rejected the run.
    if (error.status && error.status < 500) throw error;
    let recovered;
    try {
      recovered = await apiGet(knownRunId
        ? `/api/agent-runs/${encodeURIComponent(knownRunId)}`
        : `/api/agent-runs/by-request/${encodeURIComponent(control.requestId)}`);
    } catch { throw error; }
    const run = recovered?.run || recovered;
    if (!run?.id) throw error;
    result = { ...recovered, run, conversation: recovered.conversation || { id: run.conversation_id } };
    if (knownRunId) {
      // Recover this exact resume barrier, not an earlier attempt's terminal
      // event. Replaying the old failure would immediately stop the new stream.
      try {
        const events = await apiGet(`/api/agent-runs/${encodeURIComponent(knownRunId)}/events?types=run_resumed&limit=1`);
        const event = events.items?.at(-1);
        if (!event || JSON.parse(event.payload_json || '{}').request_id !== control.requestId) throw error;
        result.resume_event = event;
      } catch { throw error; }
    }
  }
  const runId = result.run?.id || knownRunId;
  if (control.canceled) {
    if (runId) {
      await apiPost(`/api/agent-runs/${encodeURIComponent(runId)}/cancel`, {
        source: 'stop_button', reason: 'stopped_during_submission',
      });
    }
    throw new DOMException('Aborted', 'AbortError');
  }
  return result;
}
