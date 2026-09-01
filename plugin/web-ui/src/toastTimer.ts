export type ToastTimer = {
  current: ReturnType<typeof setTimeout> | undefined;
};

export function scheduleToastDismiss(
  timer: ToastTimer,
  dismiss: () => void,
  delay = 4_000,
): void {
  if (timer.current !== undefined) {
    clearTimeout(timer.current);
  }
  timer.current = setTimeout(() => {
    timer.current = undefined;
    dismiss();
  }, delay);
}

export function clearToastDismiss(timer: ToastTimer): void {
  if (timer.current !== undefined) {
    clearTimeout(timer.current);
    timer.current = undefined;
  }
}
