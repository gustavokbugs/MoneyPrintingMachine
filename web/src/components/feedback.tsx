import { CircleAlert, CircleCheck, TriangleAlert, X } from "lucide-react";
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { t } from "../i18n";
import { Button, ICON_MD } from "./ui";

/* ---------- Toasts ---------- */

type ToastTone = "success" | "error";
interface ToastItem {
  id: number;
  tone: ToastTone;
  message: string;
}

const TOAST_MS = 4000;

const ToastContext = createContext<((tone: ToastTone, message: string) => void) | null>(null);

/**
 * ToastProvider renders a polite live region. Toasts never take focus and
 * dismiss themselves after four seconds, or on demand.
 */
export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);
  const nextId = useRef(1);
  const timers = useRef(new Map<number, number>());

  const dismiss = useCallback((id: number) => {
    setItems((prev) => prev.filter((i) => i.id !== id));
    const timer = timers.current.get(id);
    if (timer !== undefined) window.clearTimeout(timer);
    timers.current.delete(id);
  }, []);

  const push = useCallback(
    (tone: ToastTone, message: string) => {
      const id = nextId.current++;
      setItems((prev) => [...prev.slice(-2), { id, tone, message }]);
      timers.current.set(id, window.setTimeout(() => dismiss(id), TOAST_MS));
    },
    [dismiss],
  );

  useEffect(() => {
    const map = timers.current;
    return () => map.forEach((timer) => window.clearTimeout(timer));
  }, []);

  return (
    <ToastContext.Provider value={push}>
      {children}
      <div className="toasts" role="status" aria-live="polite" aria-atomic="false">
        {items.map((item) => (
          <div key={item.id} className={`toast toast-${item.tone}`}>
            {item.tone === "success" ? <CircleCheck size={ICON_MD} aria-hidden="true" /> : <CircleAlert size={ICON_MD} aria-hidden="true" />}
            <p>{item.message}</p>
            <button type="button" className="btn btn-ghost btn-sm btn-icon" aria-label={t("common.dismiss")} onClick={() => dismiss(item.id)}>
              <X size={16} aria-hidden="true" />
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

/** useToast returns a function that shows a success or error toast. */
export function useToast() {
  const push = useContext(ToastContext);
  if (!push) throw new Error("useToast must be used inside ToastProvider");
  return useMemo(
    () => ({
      success: (message: string) => push("success", message),
      error: (message: string) => push("error", message),
    }),
    [push],
  );
}

/* ---------- Confirmation dialog ---------- */

export interface ConfirmOptions {
  title: string;
  body: string;
  confirmLabel: string;
  tone?: "danger" | "primary";
}

const ConfirmContext = createContext<((o: ConfirmOptions) => Promise<boolean>) | null>(null);

/**
 * ConfirmProvider renders one native modal <dialog>: the rest of the page
 * becomes inert, Escape cancels, and focus returns to the trigger.
 */
export function ConfirmProvider({ children }: { children: ReactNode }) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const cancelRef = useRef<HTMLButtonElement>(null);
  const resolver = useRef<((v: boolean) => void) | null>(null);
  const returnFocus = useRef<HTMLElement | null>(null);
  const [options, setOptions] = useState<ConfirmOptions | null>(null);

  const close = useCallback((result: boolean) => {
    dialogRef.current?.close();
    resolver.current?.(result);
    resolver.current = null;
    setOptions(null);
    returnFocus.current?.focus();
  }, []);

  const confirm = useCallback((o: ConfirmOptions) => {
    returnFocus.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    setOptions(o);
    return new Promise<boolean>((resolve) => {
      resolver.current = resolve;
    });
  }, []);

  useEffect(() => {
    const d = dialogRef.current;
    if (options && d && !d.open) {
      d.showModal();
      // The safe choice receives focus first.
      cancelRef.current?.focus();
    }
  }, [options]);

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      <dialog
        ref={dialogRef}
        className="dialog"
        aria-labelledby="confirm-title"
        aria-describedby="confirm-body"
        onCancel={(e) => {
          e.preventDefault();
          close(false);
        }}
      >
        {options && (
          <div className="dialog-body">
            <div className="row">
              {options.tone === "danger" && <TriangleAlert size={ICON_MD} aria-hidden="true" className="error-text" />}
              <h2 id="confirm-title">{options.title}</h2>
            </div>
            <p id="confirm-body">{options.body}</p>
            <div className="dialog-actions">
              <button ref={cancelRef} type="button" className="btn btn-secondary" onClick={() => close(false)}>
                {t("common.cancel")}
              </button>
              <Button variant={options.tone === "danger" ? "danger" : "primary"} onClick={() => close(true)}>
                {options.confirmLabel}
              </Button>
            </div>
          </div>
        )}
      </dialog>
    </ConfirmContext.Provider>
  );
}

/** useConfirm returns a function that resolves to true when confirmed. */
export function useConfirm() {
  const confirm = useContext(ConfirmContext);
  if (!confirm) throw new Error("useConfirm must be used inside ConfirmProvider");
  return confirm;
}
