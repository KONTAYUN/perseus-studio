export function scheduleDelay(callback, delay) {
  const scheduler = globalThis.window?.setTimeout || globalThis.setTimeout;
  if (typeof scheduler === 'function') {
    return scheduler(callback, delay);
  }
  queueMicrotask(callback);
  return 0;
}

export function startInterval(callback, delay) {
  const scheduler = globalThis.window?.setInterval || globalThis.setInterval;
  if (typeof scheduler === 'function') {
    return scheduler(callback, delay);
  }
  return null;
}

export function stopInterval(timer) {
  if (timer == null) return;
  const clearer = globalThis.window?.clearInterval || globalThis.clearInterval;
  if (typeof clearer === 'function') {
    clearer(timer);
  }
}

export function stopDelay(timer) {
  if (timer == null) return;
  const clearer = globalThis.window?.clearTimeout || globalThis.clearTimeout;
  if (typeof clearer === 'function') clearer(timer);
}

export function wait(delay) {
  return new Promise((resolve) => scheduleDelay(resolve, delay));
}
