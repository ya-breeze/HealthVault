import { useEffect, useState } from 'react';

const COARSE_POINTER_QUERY = '(pointer: coarse)';

// Same reasoning as TapTarget's `compactOnMouse` (components/ui/TapTarget.tsx):
// keyed off pointer type, not viewport width, because a phone in landscape is
// 667 CSS px wide and would otherwise be measured as "desktop". Starts `false`
// so server and first client render agree — `matchMedia` doesn't exist on the
// server — then upgrades in an effect once the real value is known.
export default function useCoarsePointer(): boolean {
  const [isCoarse, setIsCoarse] = useState(false);

  useEffect(() => {
    if (typeof matchMedia !== 'function') return;
    const mql = matchMedia(COARSE_POINTER_QUERY);
    setIsCoarse(mql.matches);
    const onChange = (e: MediaQueryListEvent) => setIsCoarse(e.matches);
    mql.addEventListener('change', onChange);
    return () => mql.removeEventListener('change', onChange);
  }, []);

  return isCoarse;
}
