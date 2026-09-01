import { afterEach, describe, expect, it, vi } from "vitest";
import {
  clearToastDismiss,
  scheduleToastDismiss,
  type ToastTimer,
} from "./toastTimer";

describe("toast dismissal timer", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("keeps a newer toast visible until its own timeout", () => {
    vi.useFakeTimers();
    const timer: ToastTimer = { current: undefined };
    const dismissFirst = vi.fn();
    const dismissSecond = vi.fn();

    scheduleToastDismiss(timer, dismissFirst);
    vi.advanceTimersByTime(1_000);
    scheduleToastDismiss(timer, dismissSecond);

    vi.advanceTimersByTime(3_000);
    expect(dismissFirst).not.toHaveBeenCalled();
    expect(dismissSecond).not.toHaveBeenCalled();

    vi.advanceTimersByTime(1_000);
    expect(dismissSecond).toHaveBeenCalledTimes(1);
  });

  it("cancels dismissal during cleanup", () => {
    vi.useFakeTimers();
    const timer: ToastTimer = { current: undefined };
    const dismiss = vi.fn();

    scheduleToastDismiss(timer, dismiss);
    clearToastDismiss(timer);
    vi.runAllTimers();

    expect(dismiss).not.toHaveBeenCalled();
  });
});
