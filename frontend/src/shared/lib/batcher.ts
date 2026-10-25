export interface Batcher<T> {
  /** Queues an item; the batch is delivered `delayMs` after the first push. */
  push: (item: T) => void;
  /** Drops anything queued and stops the timer. */
  cancel: () => void;
}

/**
 * Collects items and hands them over in one call per time window. A burst of
 * 500 log lines becomes one state update and one render instead of 500.
 *
 * A timer is used rather than requestAnimationFrame because rAF stops while the
 * window is hidden, and the queue would grow without bound.
 */
export function createBatcher<T>(
  onFlush: (items: T[]) => void,
  delayMs: number,
): Batcher<T> {
  let queue: T[] = [];
  let timer: ReturnType<typeof setTimeout> | null = null;

  const flush = () => {
    timer = null;
    const items = queue;
    queue = [];
    if (items.length > 0) onFlush(items);
  };

  return {
    push: (item) => {
      queue.push(item);
      timer ??= setTimeout(flush, delayMs);
    },
    cancel: () => {
      if (timer !== null) clearTimeout(timer);
      timer = null;
      queue = [];
    },
  };
}
