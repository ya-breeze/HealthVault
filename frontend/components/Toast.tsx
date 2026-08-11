'use client';
import { createContext, useCallback, useContext, useRef, useState } from 'react';

export type ToastVariant = 'success' | 'error';

interface ToastEntry {
  id: number;
  message: string;
  variant: ToastVariant;
}

interface ToastContextValue {
  showToast: (message: string, variant: ToastVariant) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

const AUTO_DISMISS_MS = 3000;

const VARIANT_STYLES: Record<ToastVariant, string> = {
  success: 'border-green-200 dark:border-green-800 text-green-800 dark:text-green-300',
  error: 'border-red-200 dark:border-red-800 text-red-800 dark:text-red-300',
};

// App-wide toast provider — a single stack of transient success/error
// messages, additive to (never a replacement for) each component's own
// inline error text. See ui-notifications spec: "Food Review Mutation
// Feedback".
export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<ToastEntry[]>([]);
  const nextId = useRef(0);

  const dismiss = useCallback((id: number) => {
    setToasts(ts => ts.filter(t => t.id !== id));
  }, []);

  const showToast = useCallback((message: string, variant: ToastVariant) => {
    const id = nextId.current++;
    setToasts(ts => [...ts, { id, message, variant }]);
    setTimeout(() => dismiss(id), AUTO_DISMISS_MS);
  }, [dismiss]);

  return (
    <ToastContext.Provider value={{ showToast }}>
      {children}
      <div className="fixed bottom-4 inset-x-0 z-50 flex flex-col items-center gap-2 px-4 pointer-events-none">
        {toasts.map(t => (
          <div
            key={t.id}
            role="status"
            className={`pointer-events-auto flex items-center gap-3 max-w-sm w-full sm:w-auto bg-bg-elevated border rounded-xl shadow-lg px-4 py-2.5 text-sm font-medium ${VARIANT_STYLES[t.variant]}`}
          >
            <span className="flex-1">{t.message}</span>
            <button
              onClick={() => dismiss(t.id)}
              className="text-text-muted hover:text-text leading-none"
              aria-label="Dismiss notification"
            >
              &times;
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext);
  if (!ctx) {
    throw new Error('useToast must be used within a ToastProvider');
  }
  return ctx;
}
