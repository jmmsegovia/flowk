import { create } from 'zustand';

export type ToastType = 'success' | 'error' | 'info' | 'warning';

export type Toast = {
  id: string;
  type: ToastType;
  message: string;
  timeoutMs: number | null;
};

type ToastState = {
  toasts: Toast[];
  addToast: (toast: { type: ToastType; message: string; timeoutMs?: number | null }) => string;
  removeToast: (id: string) => void;
  clearToasts: () => void;
};

const DEFAULT_TIMEOUT = 4000;

const useToastStore = create<ToastState>((set) => ({
  toasts: [],
  addToast: (toast) => {
    const id = `toast_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`;
    const timeoutMs = toast.timeoutMs === undefined ? DEFAULT_TIMEOUT : toast.timeoutMs;
    set((state) => ({
      toasts: [...state.toasts, { ...toast, timeoutMs, id }]
    }));
    return id;
  },
  removeToast: (id) =>
    set((state) => ({
      toasts: state.toasts.filter((toast) => toast.id !== id)
    })),
  clearToasts: () => set({ toasts: [] })
}));

export default useToastStore;
