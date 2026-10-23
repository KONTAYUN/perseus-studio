import { scheduleDelay, stopDelay } from './chatRunStreamTimers.js';

// Own the stream, reconnect timer and serialized delivery as one lifetime.
// A failed handler terminates delivery; later queued events cannot revive it.
export function consumeRunTransport({ url, control, onCancel, onRun, onEvent, poll, isSettled, initialSequence = 0 }) {
  return new Promise((resolve, reject) => {
    let source = null;
    let timer = null;
    let stopped = false;
    let attempts = 0;
    let sequence = Math.max(0, Number(initialSequence) || 0);
    let chain = Promise.resolve();
    const cleanup = () => {
      source?.close();
      source = null;
      stopDelay(timer);
      timer = null;
      if (control.abort === abort) control.abort = () => {};
    };
    const end = (error) => {
      if (stopped) return;
      stopped = true;
      cleanup();
      if (error) reject(error); else resolve();
    };
    const abort = () => {
      if (stopped) return;
      try { onCancel?.(); } finally { end(new DOMException('Aborted', 'AbortError')); }
    };
    control.abort = abort;
    if (control.canceled) { abort(); return; }
    const enqueue = (handler) => {
      chain = chain.then(async () => {
        if (stopped) return;
        await handler();
        if (isSettled()) end();
      }).catch(end);
    };
    const connect = () => {
      if (stopped) return;
      try {
        const current = new EventSource(url(sequence), { withCredentials: true });
        source = current;
        current.addEventListener('run', (event) => enqueue(() => onRun(JSON.parse(event.data))));
        current.addEventListener('event', (event) => enqueue(async () => {
          const item = JSON.parse(event.data);
          if (item?.seq && item.seq <= sequence) return;
          await onEvent(item);
          if (item?.seq) sequence = Math.max(sequence, item.seq);
          attempts = 0;
        }));
        current.onerror = () => {
          if (stopped || source !== current) return;
          current.close();
          source = null;
          // Finish queued events before deciding the resume sequence.
          enqueue(() => {
            if (isSettled()) return;
            if (attempts < 3) {
              attempts += 1;
              timer = scheduleDelay(connect, Math.min(500 * attempts, 1500));
            } else {
              return poll(() => stopped).then(() => end());
            }
          });
        };
      } catch (error) { end(error); }
    };
    connect();
  });
}
