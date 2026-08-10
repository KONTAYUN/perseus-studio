import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { readMotionDuration } from './motionTokens';

export default function useMotionPresence(open, {
  exitDuration,
  exitToken = '--motion-base',
  onExitComplete,
} = {}) {
  const [present, setPresent] = useState(open);
  const [exiting, setExiting] = useState(false);
  const timeoutRef = useRef(null);
  const onExitCompleteRef = useRef(onExitComplete);
  const resolvedExitDuration = useMemo(
    () => (Number.isFinite(exitDuration) ? exitDuration : readMotionDuration(exitToken)),
    [exitDuration, exitToken]
  );

  useEffect(() => {
    onExitCompleteRef.current = onExitComplete;
  }, [onExitComplete]);

  const clearExitTimer = useCallback(() => {
    if (!timeoutRef.current) return;
    window.clearTimeout(timeoutRef.current);
    timeoutRef.current = null;
  }, []);

  const finishExit = useCallback((afterExit) => {
    setPresent(false);
    setExiting(false);
    afterExit?.();
    onExitCompleteRef.current?.();
  }, []);

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
    }, resolvedExitDuration);
    return clearExitTimer;
  }, [clearExitTimer, finishExit, open, present, resolvedExitDuration]);

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
    }, resolvedExitDuration);
  }, [clearExitTimer, finishExit, present, resolvedExitDuration]);

  return useMemo(() => ({
    closeWithPresence,
    exiting,
    present,
  }), [closeWithPresence, exiting, present]);
}
