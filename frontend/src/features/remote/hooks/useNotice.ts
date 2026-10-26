import { useCallback, useState } from "react";
import { acknowledge, browserStorage, isAcknowledged } from "../notices";

/**
 * Whether the person has read a provider's notice, and a way to say they have.
 * An acknowledgement is also kept in memory, so it holds for this run even where
 * storage is unavailable.
 */
export function useNotice(providerId: string): { acknowledged: boolean; acknowledge: () => void } {
  const [thisRun, setThisRun] = useState<ReadonlySet<string>>(new Set());

  const ack = useCallback(() => {
    acknowledge(browserStorage(), providerId);
    setThisRun((prev) => new Set(prev).add(providerId));
  }, [providerId]);

  return { acknowledged: thisRun.has(providerId) || isAcknowledged(browserStorage(), providerId), acknowledge: ack };
}
