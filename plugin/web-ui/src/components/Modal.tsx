import {
  useEffect,
  useRef,
  type PropsWithChildren,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";

type Props = PropsWithChildren<{
  title: ReactNode;
  onClose: () => void;
  footer?: ReactNode;
  dialogClassName?: string;
  busy?: boolean;
}>;

export function Modal({
  title,
  onClose,
  footer,
  dialogClassName = "",
  busy = false,
  children,
}: Props) {
  const closeButton = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    const focusedBeforeOpen = document.activeElement as HTMLElement | null;
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    closeButton.current?.focus();
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !busy) {
        event.preventDefault();
        onClose();
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => {
      document.body.style.overflow = overflow;
      window.removeEventListener("keydown", onKeyDown);
      focusedBeforeOpen?.focus();
    };
  }, [busy, onClose]);

  const close = () => {
    if (!busy) {
      onClose();
    }
  };

  return createPortal(
    <div className="modal" role="presentation" onMouseDown={close}>
      <section
        className={`dialog ${dialogClassName}`}
        role="dialog"
        aria-modal="true"
        aria-busy={busy}
        aria-label={typeof title === "string" ? title : undefined}
        onMouseDown={(event) => event.stopPropagation()}
      >
        <header className="dialog-head">
          <h2>{title}</h2>
          <button
            ref={closeButton}
            className="close"
            onClick={close}
            disabled={busy}
            aria-label="Close"
          >
            ×
          </button>
        </header>
        <div className="dialog-body">{children}</div>
        {footer && <footer className="dialog-footer">{footer}</footer>}
      </section>
    </div>,
    document.body,
  );
}
