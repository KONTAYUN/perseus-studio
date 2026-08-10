const FALLBACK_DURATIONS = {
  '--motion-fast': 140,
  '--motion-base': 180,
  '--motion-popover': 180,
  '--motion-modal': 240,
  '--motion-layout': 280,
};

function parseCssTime(value, fallback) {
  const normalized = String(value || '').trim();
  if (!normalized) return fallback;
  if (normalized.endsWith('ms')) {
    const milliseconds = Number.parseFloat(normalized);
    return Number.isFinite(milliseconds) ? milliseconds : fallback;
  }
  if (normalized.endsWith('s')) {
    const seconds = Number.parseFloat(normalized);
    return Number.isFinite(seconds) ? seconds * 1000 : fallback;
  }
  return fallback;
}

function prefersReducedMotion() {
  return typeof window !== 'undefined'
    && typeof window.matchMedia === 'function'
    && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

export function readMotionDuration(token = '--motion-base', fallback = FALLBACK_DURATIONS[token] || 180) {
  if (typeof window === 'undefined' || typeof document === 'undefined') return fallback;
  if (prefersReducedMotion()) return 1;
  const value = window.getComputedStyle(document.documentElement).getPropertyValue(token);
  return parseCssTime(value, fallback);
}

export function readMotionTimingFunction(token = '--ease-motion', fallback = 'cubic-bezier(0.23, 1, 0.32, 1)') {
  if (typeof window === 'undefined' || typeof document === 'undefined') return fallback;
  return window.getComputedStyle(document.documentElement).getPropertyValue(token).trim() || fallback;
}
