import React, { createContext, useCallback, useContext, useState } from 'react';
import { CheckCircle2, AlertCircle, Info, X } from 'lucide-react';

export type ToastType = 'success' | 'error' | 'info';

interface ToastItem {
  id: string;
  type: ToastType;
  title: string;
  message?: string;
}

interface ToastContextValue {
  notify: (toast: Omit<ToastItem, 'id'>) => void;
  success: (title: string, message?: string) => void;
  error: (title: string, message?: string) => void;
  info: (title: string, message?: string) => void;
}

const ToastContext = createContext<ToastContextValue>({
  notify: () => {},
  success: () => {},
  error: () => {},
  info: () => {},
});

export function useToast() {
  return useContext(ToastContext);
}

export const ToastProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [toasts, setToasts] = useState<ToastItem[]>([]);

  const dismiss = useCallback((id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const notify = useCallback(
    (toast: Omit<ToastItem, 'id'>) => {
      const id = `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
      setToasts((prev) => [...prev, { ...toast, id }]);
      window.setTimeout(() => {
        setToasts((prev) => prev.filter((t) => t.id !== id));
      }, 4500);
    },
    []
  );

  const value: ToastContextValue = {
    notify,
    success: useCallback((title, message) => notify({ type: 'success', title, message }), [notify]),
    error: useCallback((title, message) => notify({ type: 'error', title, message }), [notify]),
    info: useCallback((title, message) => notify({ type: 'info', title, message }), [notify]),
  };

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div
        aria-live="polite"
        className="pointer-events-none fixed right-5 top-5 z-50 flex w-full max-w-sm flex-col gap-2.5"
      >
        {toasts.map((t) => (
          <div
            key={t.id}
            role="status"
            className={`pointer-events-auto flex items-start gap-3 rounded-xl border bg-white p-3.5 shadow-lg transition-all ${
              t.type === 'success'
                ? 'border-emerald-200'
                : t.type === 'error'
                  ? 'border-rose-200'
                  : 'border-sky-200'
            }`}
          >
            {t.type === 'success' && <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-emerald-600" />}
            {t.type === 'error' && <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-rose-600" />}
            {t.type === 'info' && <Info className="mt-0.5 h-4 w-4 shrink-0 text-sky-600" />}
            <div className="min-w-0 flex-1">
              <p className="text-xs font-semibold text-slate-900">{t.title}</p>
              {t.message && <p className="mt-0.5 text-xs text-slate-600 break-words">{t.message}</p>}
            </div>
            <button
              type="button"
              onClick={() => dismiss(t.id)}
              aria-label="Dismiss notification"
              className="rounded p-0.5 text-slate-400 hover:bg-slate-100 hover:text-slate-600"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
};

