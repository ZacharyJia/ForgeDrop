import React, { createContext, useCallback, useContext, useMemo, useRef, useState } from "react";

type ToastKind = "success" | "error" | "info";

type ToastItem = {
  id: string;
  kind: ToastKind;
  title?: string;
  message: string;
};

type ToastAPI = {
  success: (message: string, title?: string) => void;
  error: (message: string, title?: string) => void;
  info: (message: string, title?: string) => void;
};

const ToastContext = createContext<ToastAPI | null>(null);

function newID() {
  // No crypto dependency; good enough for UI keys.
  return Math.random().toString(16).slice(2) + Date.now().toString(16);
}

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);
  const timers = useRef<Map<string, number>>(new Map());

  const remove = useCallback((id: string) => {
    setItems((prev) => prev.filter((t) => t.id !== id));
    const tm = timers.current.get(id);
    if (tm) {
      window.clearTimeout(tm);
      timers.current.delete(id);
    }
  }, []);

  const push = useCallback(
    (kind: ToastKind, message: string, title?: string) => {
      const id = newID();
      setItems((prev) => [{ id, kind, title, message }, ...prev].slice(0, 5));
      const tm = window.setTimeout(() => remove(id), kind === "error" ? 6000 : 3500);
      timers.current.set(id, tm);
    },
    [remove]
  );

  const api = useMemo<ToastAPI>(() => {
    return {
      success: (m, t) => push("success", m, t),
      error: (m, t) => push("error", m, t),
      info: (m, t) => push("info", m, t),
    };
  }, [push]);

  return (
    <ToastContext.Provider value={api}>
      {children}
      <div className="toast-stack" aria-live="polite" aria-relevant="additions">
        {items.map((t) => (
          <div key={t.id} className={`toast toast-${t.kind}`} role="status">
            <div className="toast-body">
              {t.title && <div className="toast-title">{t.title}</div>}
              <div className="toast-message">{t.message}</div>
            </div>
            <button className="toast-close" onClick={() => remove(t.id)} aria-label="Close">
              ×
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): ToastAPI {
  const ctx = useContext(ToastContext);
  if (!ctx) {
    throw new Error("useToast must be used within ToastProvider");
  }
  return ctx;
}
