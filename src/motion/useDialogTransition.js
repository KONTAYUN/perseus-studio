import { useCallback, useEffect, useRef, useState } from 'react';
import { MOTION } from './policy.js';
import { useReducedMotion } from './useReducedMotion.js';

export function useDialogTransition(onClose) {
  const [closing, setClosing] = useState(false);
  const reduced = useReducedMotion();
  const callback = useRef(onClose);
  useEffect(() => { callback.current = onClose; }, [onClose]);
  useEffect(() => {
    if (!closing) return undefined;
    const timer = window.setTimeout(() => callback.current(), reduced ? 0 : MOTION.fast);
    return () => window.clearTimeout(timer);
  }, [closing, reduced]);
  const requestClose = useCallback(() => setClosing(true), []);
  useEffect(() => {
    const handleKey = (event) => { if (event.key === 'Escape' && !event.defaultPrevented) requestClose(); };
    document.addEventListener('keydown', handleKey);
    return () => document.removeEventListener('keydown', handleKey);
  }, [requestClose]);
  return { closing, requestClose };
}
