const API_BASE_URL = (import.meta.env?.VITE_API_BASE_URL || '').replace(/\/$/, '');

async function request(path, options = {}) {
  const { signal, timeoutMs = 60_000, responseType, ...fetchOptions } = options;
  const controller = new AbortController();
  const forwardAbort = () => controller.abort(signal.reason);
  if (signal?.aborted) forwardAbort();
  else signal?.addEventListener('abort', forwardAbort, { once: true });
  const timer = timeoutMs > 0 ? setTimeout(() => {
    controller.abort(new DOMException('请求超时，请检查连接后重试', 'TimeoutError'));
  }, timeoutMs) : null;
  try {
    const response = await fetch(`${API_BASE_URL}${path}`, {
      credentials: 'include',
      headers: fetchOptions.headers ?? { 'Content-Type': 'application/json' },
      ...fetchOptions,
      signal: controller.signal,
    });
    if (response.ok && responseType === 'blob') return await response.blob();
    const contentType = response.headers.get('content-type') || '';
    const isJSON = contentType.includes('application/json');
    const payload = isJSON ? await response.json() : await response.text();
    if (!response.ok) {
      const message =
        (isJSON && payload?.error) || (typeof payload === 'string' && payload) || 'Request failed';
      if (response.status === 401 && typeof window !== 'undefined') {
        window.dispatchEvent(new CustomEvent('perseus:unauthorized'));
      }
      const error = new Error(message);
      error.status = response.status;
      error.payload = payload;
      if (isJSON && payload?.diagnostic_log_id) error.diagnosticLogId = payload.diagnostic_log_id;
      throw error;
    }
    return payload;
  } finally {
    if (timer !== null) clearTimeout(timer);
    signal?.removeEventListener('abort', forwardAbort);
  }
}

export function apiGet(path, options = {}) {
  return request(path, options);
}

export function apiPost(path, body, options = {}) {
  return request(path, {
    method: 'POST',
    body: JSON.stringify(body),
    ...options,
  });
}

export function apiPostForm(path, formData, options = {}) {
  return request(path, {
    method: 'POST',
    headers: {},
    body: formData,
    ...options,
  });
}

export function apiPostFormWithSignal(path, formData, signal) {
  return request(path, {
    method: 'POST',
    headers: {},
    body: formData,
    signal,
  });
}

export function apiPatch(path, body) {
  return request(path, {
    method: 'PATCH',
    body: JSON.stringify(body),
  });
}

export function apiDelete(path) {
  return request(path, {
    method: 'DELETE',
  });
}

export function apiDeleteWithBody(path, body) {
  return request(path, {
    method: 'DELETE',
    body: JSON.stringify(body),
  });
}

export async function apiUploadFile(path, file, overwrite = false) {
  const formData = new FormData();
  formData.append('path', path);
  formData.append('overwrite', overwrite ? 'true' : 'false');
  formData.append('file', file);

  return request('/api/files/upload', {
    method: 'POST',
    headers: {},
    body: formData,
  });
}

export { API_BASE_URL };

export function apiGetBlob(path, options = {}) {
  return request(path, { ...options, responseType: 'blob' });
}
