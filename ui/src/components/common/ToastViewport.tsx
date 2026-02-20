import { useEffect } from 'react';
import useToastStore, { Toast } from '../../state/toastStore';

type ToastItemProps = {
  toast: Toast;
  onDismiss: (id: string) => void;
};

function ToastItem({ toast, onDismiss }: ToastItemProps) {
  useEffect(() => {
    if (!toast.timeoutMs) {
      return;
    }
    const timeoutId = window.setTimeout(() => onDismiss(toast.id), toast.timeoutMs);
    return () => window.clearTimeout(timeoutId);
  }, [toast.id, toast.timeoutMs, onDismiss]);

  const role = toast.type === 'error' ? 'alert' : 'status';

  return (
    <div className={`toast toast--${toast.type}`} role={role} aria-live="polite">
      <span className="toast__icon" aria-hidden="true">
        {toast.type === 'success' ? 'OK' : null}
        {toast.type === 'error' ? '!' : null}
        {toast.type === 'info' ? 'i' : null}
        {toast.type === 'warning' ? '!' : null}
      </span>
      <span className="toast__message">{toast.message}</span>
      <button
        className="toast__close"
        type="button"
        aria-label="Dismiss notification"
        onClick={() => onDismiss(toast.id)}
      >
        x
      </button>
    </div>
  );
}

function ToastViewport() {
  const toasts = useToastStore((state) => state.toasts);
  const removeToast = useToastStore((state) => state.removeToast);

  if (toasts.length === 0) {
    return null;
  }

  return (
    <div className="toast-viewport" aria-live="polite">
      {toasts.map((toast) => (
        <ToastItem key={toast.id} toast={toast} onDismiss={removeToast} />
      ))}
    </div>
  );
}

export default ToastViewport;
