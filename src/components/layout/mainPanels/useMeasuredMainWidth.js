import { useLayoutEffect, useState } from 'react';

export function useMeasuredMainWidth(mainRef) {
  const [mainWidth, setMainWidth] = useState(0);

  useLayoutEffect(() => {
    if (!mainRef.current) return undefined;
    const update = () => setMainWidth(Math.round(mainRef.current?.clientWidth || 0));
    update();
    const observer = new ResizeObserver(update);
    observer.observe(mainRef.current);
    return () => observer.disconnect();
  }, [mainRef]);

  return mainWidth;
}
