import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

const DEFAULT_EXIT_DURATION_MS = 260;

export default function useMotionPresence(open, {
  exitDuration = DEFAULT_EXIT_DURATION_MS,
  onExitComplete,
} = {}) {
  const [present, setPresent] = useState(open);
  const [exiting, setExiting] = useState(false);
  const timeoutRef = useRef(null);

  const clearExitTimer = useCallback(() => {
    if (!timeoutRef.current) return;
    window.clearTimeout(timeoutRef.current);
    timeoutRef.current = null;
  }, []);

  const finishExit = useCallback((afterExit) => {
    setPresent(false);
    setExiting(false);
    afterExit?.();
    onExitComplete?.();
  }, [onExitComplete]);

  useEffect(() => {
    if (open) {
      clearExitTimer();
      setPresent(true);
      setExiting(false);
      return undefined;
    }
    if (!present) return undefined;
    setExiting(true);
    timeoutRef.current = window.setTimeout(() => {
      timeoutRef.current = null;
      finishExit();
    }, exitDuration);
    return clearExitTimer;
  }, [clearExitTimer, exitDuration, finishExit, open, present]);

  useEffect(() => clearExitTimer, [clearExitTimer]);

  const closeWithPresence = useCallback((close) => {
    clearExitTimer();
    if (!present) {
      close?.();
      return;
    }
    setExiting(true);
    timeoutRef.current = window.setTimeout(() => {
      timeoutRef.current = null;
      finishExit(close);
    }, exitDuration);
  }, [clearExitTimer, exitDuration, finishExit, present]);

  return useMemo(() => ({
    closeWithPresence,
    exiting,
    present,
  }), [closeWithPresence, exiting, present]);
}
