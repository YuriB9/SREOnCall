type ToastFn = (message: string, type?: 'error' | 'success') => void

let _fn: ToastFn | null = null

export function showToast(message: string, type: 'error' | 'success' = 'error') {
  _fn?.(message, type)
}

export function _registerToastFn(fn: ToastFn | null) {
  _fn = fn
}
