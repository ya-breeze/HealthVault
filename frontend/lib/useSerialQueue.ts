import { useCallback, useRef } from 'react';

// A FIFO async queue: each claimed operation only starts once every
// previously-claimed one has settled (success or failure), so issue order
// and commit order are always the same thing, with nothing left to compare
// or guess afterward. A failed operation never wedges the queue — the next
// claim still runs. Shared by ReviewClient (meal-mutation ordering) and
// LanguageContext (GET/PUT ordering against the settings blob) so this
// exact "claim the ref synchronously, don't snapshot it" mechanism, which
// has already needed fixing more than once (independently, in each file)
// after being duplicated by hand, only needs to live in one place.
export function useSerialQueue() {
  const queueRef = useRef<Promise<unknown>>(Promise.resolve());
  const claim = useCallback(<T,>(op: () => Promise<T>): Promise<T> => {
    const run = queueRef.current.then(op);
    queueRef.current = run.catch(() => undefined);
    return run;
  }, []);
  // Resolves once every operation claimed so far — including one claimed
  // moments earlier in the same synchronous block — has settled. Read
  // queueRef.current at call time, not captured earlier: ReviewClient's
  // queueDelete uses this to wait out anything queued immediately behind a
  // delete before its caller navigates away, which only works if this
  // reflects the queue's tail as of when drain() is actually called.
  const drain = useCallback((): Promise<unknown> => queueRef.current, []);
  return { claim, drain };
}
