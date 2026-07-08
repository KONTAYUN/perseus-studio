import { useCallback, useRef, useState } from 'react';

const DEFAULT_TOAST_DURATION = 3000;
const MAX_VISIBLE_TOASTS = 4;

export default function useToastQueue() {
  const [toasts, setToasts] = useState([]);
  const sequenceRef = useRef(0);
  const lastToastRef = useRef({ text: '', at: 0 });

  const dismissToast = useCallback((id) => {
    setToasts((current) =>
      current.map((toast) => (toast.id === id ? { ...toast, exiting: true } : toast))
    );
  }, []);

  const removeToast = useCallback((id) => {
    setToasts((current) => current.filter((toast) => toast.id !== id));
  }, []);

  const showToast = useCallback((input, options = {}) => {
    const normalized = normalizeToastInput(input, options);
    if (!normalized.text) return '';

    const now = Date.now();
    if (lastToastRef.current.text === normalized.text && now - lastToastRef.current.at < 450) {
      return '';
    }
    lastToastRef.current = { text: normalized.text, at: now };

    const id = `toast_${now}_${sequenceRef.current++}`;
    const toast = {
      id,
      text: normalized.text,
      tone: normalized.tone,
      duration: normalized.duration,
      exiting: false,
    };

    setToasts((current) => [...current, toast].slice(-MAX_VISIBLE_TOASTS));
    return id;
  }, []);

  return {
    dismissToast,
    removeToast,
    showToast,
    toasts,
  };
}

function normalizeToastInput(input, options = {}) {
  const raw = typeof input === 'object' && input !== null ? input : { text: input };
  const text = String(raw.message ?? raw.text ?? '').trim();
  return {
    text,
    tone: normalizeToastTone(raw.tone ?? options.tone, text),
    duration: Number(raw.duration ?? options.duration) > 0
      ? Number(raw.duration ?? options.duration)
      : DEFAULT_TOAST_DURATION,
  };
}

function normalizeToastTone(tone, text) {
  const normalized = String(tone || '').toLowerCase();
  if (['success', 'error', 'warning', 'info'].includes(normalized)) return normalized;

  const lowerText = String(text || '').toLowerCase();
  if (
    lowerText.includes('error') ||
    lowerText.includes('failed') ||
    lowerText.includes('失败') ||
    lowerText.includes('错误')
  ) {
    return 'error';
  }
  if (
    lowerText.includes('warn') ||
    lowerText.includes('无法') ||
    lowerText.includes('不能') ||
    lowerText.includes('跳过')
  ) {
    return 'warning';
  }
  if (
    lowerText.includes('已') ||
    lowerText.includes('完成') ||
    lowerText.includes('成功') ||
    lowerText.includes('插入')
  ) {
    return 'success';
  }
  return 'info';
}
