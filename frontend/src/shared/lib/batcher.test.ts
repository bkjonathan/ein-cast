import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createBatcher } from "./batcher";

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("createBatcher", () => {
  it("delivers a burst as one batch after the delay", () => {
    const onFlush = vi.fn();
    const b = createBatcher<number>(onFlush, 50);

    b.push(1);
    b.push(2);
    b.push(3);
    expect(onFlush).not.toHaveBeenCalled();

    vi.advanceTimersByTime(50);
    expect(onFlush).toHaveBeenCalledTimes(1);
    expect(onFlush).toHaveBeenCalledWith([1, 2, 3]);
  });

  it("starts a fresh window for the next burst", () => {
    const onFlush = vi.fn();
    const b = createBatcher<string>(onFlush, 50);

    b.push("a");
    vi.advanceTimersByTime(50);
    b.push("b");
    vi.advanceTimersByTime(50);

    expect(onFlush.mock.calls).toEqual([[["a"]], [["b"]]]);
  });

  it("cancel drops what is queued and stops the timer", () => {
    const onFlush = vi.fn();
    const b = createBatcher<number>(onFlush, 50);

    b.push(1);
    b.cancel();
    vi.advanceTimersByTime(500);
    expect(onFlush).not.toHaveBeenCalled();

    b.push(2); // still usable afterwards
    vi.advanceTimersByTime(50);
    expect(onFlush).toHaveBeenCalledWith([2]);
  });

  it("does nothing when nothing was pushed", () => {
    const onFlush = vi.fn();
    createBatcher<number>(onFlush, 50);
    vi.advanceTimersByTime(500);
    expect(onFlush).not.toHaveBeenCalled();
  });
});
