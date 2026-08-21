import { message } from 'antd';

export interface ApiError {
  message: string;
  code?: string;
  status?: number;
}

export const ERROR_MESSAGES = {
  NETWORK_ERROR: '网络连接失败，请检查网络后重试',
  SERVER_ERROR: '服务器暂时不可用，请稍后重试',
  UNAUTHORIZED: '登录已过期，请重新登录',
  FORBIDDEN: '您没有权限执行此操作',
  NOT_FOUND: '请求的资源不存在',
  VALIDATION_ERROR: '输入信息有误，请检查后重试',
  RATE_LIMITED: '请求过于频繁，请稍后重试',
  DEFAULT: '操作失败，请稍后重试',
};

export function getErrorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message || ERROR_MESSAGES.DEFAULT;
  }
  if (typeof error === 'string') {
    return error;
  }
  if (typeof error === 'object' && error !== null) {
    const err = error as ApiError;
    if (err.message) return err.message;
    if (err.status === 401) return ERROR_MESSAGES.UNAUTHORIZED;
    if (err.status === 403) return ERROR_MESSAGES.FORBIDDEN;
    if (err.status === 404) return ERROR_MESSAGES.NOT_FOUND;
    if (err.status === 422) return ERROR_MESSAGES.VALIDATION_ERROR;
    if (err.status === 429) return ERROR_MESSAGES.RATE_LIMITED;
    if (err.status && err.status >= 500) return ERROR_MESSAGES.SERVER_ERROR;
  }
  return ERROR_MESSAGES.DEFAULT;
}

export function showError(error: unknown, defaultMessage?: string) {
  const msg = defaultMessage || getErrorMessage(error);
  message.error(msg, 3);
}

export function showSuccess(content: string) {
  message.success(content, 2);
}

export function showInfo(content: string) {
  message.info(content, 2);
}

export async function safeRequest<T>(
  request: () => Promise<T>,
  fallback: T,
  options?: {
    onError?: (error: unknown) => void;
    showErrorMessage?: boolean;
    errorMessage?: string;
  }
): Promise<T> {
  try {
    return await request();
  } catch (error) {
    if (options?.showErrorMessage !== false) {
      showError(error, options?.errorMessage);
    }
    options?.onError?.(error);
    return fallback;
  }
}

export async function safeRequestWithEmptyArray<T>(
  request: () => Promise<T[]>,
  options?: {
    onError?: (error: unknown) => void;
    showErrorMessage?: boolean;
    errorMessage?: string;
  }
): Promise<T[]> {
  return safeRequest(request, [] as T[], options);
}
