const sessions = new Map();
const healthByPath = new Map();
let cachedCharacters = 0;
export function registerDocumentSession(path, controller) {
  if (!path) return () => {};
  if (!sessions.has(path)) sessions.set(path, new Set());
  sessions.get(path).add(controller);
  return () => { sessions.get(path)?.delete(controller); if (!sessions.get(path)?.size) sessions.delete(path); };
}
export async function flushDocumentSessions(path) {
  for (const controller of sessions.get(path) || []) await controller.flush();
}
export function hasPendingDocumentSessions(path) {
  return [...(sessions.get(path) || [])].some(controller => controller.isPending?.());
}
export function documentHealth(path, source) {
  const value = healthByPath.get(path);
  return value?.source === source ? value : null;
}
export function notifyDocumentHealth(path, health, source) {
  const detail = { path, ...health, source };
  cachedCharacters -= healthByPath.get(path)?.source?.length || 0;
  healthByPath.delete(path); healthByPath.set(path, detail);
  cachedCharacters += source?.length || 0;
  // Diagnostics are transient; session drafts, not this cache, own recovery.
  while (healthByPath.size > 100 || cachedCharacters > 8 * 1024 * 1024) {
    const oldest = healthByPath.keys().next().value;
    cachedCharacters -= healthByPath.get(oldest)?.source?.length || 0;
    healthByPath.delete(oldest);
  }
  if (typeof window !== 'undefined') window.dispatchEvent(new CustomEvent('perseus:document-health', { detail }));
}
