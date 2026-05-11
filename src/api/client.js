const API_BASE_URL = (import.meta.env.VITE_API_BASE_URL || '').replace(/\/$/, '');

async function request(path, options = {}) {
  try {
    const { signal, ...fetchOptions } = options;
    const response = await fetch(`${API_BASE_URL}${path}`, {
      credentials: 'include',
      headers: fetchOptions.headers ?? { 'Content-Type': 'application/json' },
      ...fetchOptions,
      signal,
    });

    const contentType = response.headers.get('content-type') || '';
    const isJSON = contentType.includes('application/json');
    const payload = isJSON ? await response.json() : await response.text();

    if (!response.ok) {
      const message =
        (isJSON && payload?.error) || (typeof payload === 'string' && payload) || 'Request failed';
      console.error(`[api] ${options.method || 'GET'} ${path} -> ${response.status}`, payload);
      if (response.status === 401) {
        window.dispatchEvent(new CustomEvent('perseus:unauthorized'));
      }
      const error = new Error(message);
      error.status = response.status;
      error.payload = payload;
      if (isJSON && payload?.diagnostic_log_id) {
        error.diagnosticLogId = payload.diagnostic_log_id;
      }
      throw error;
    }

    return payload;
  } catch (error) {
    console.error(`[api] ${options.method || 'GET'} ${path} failed`, error);
    throw error;
  }
}

export function apiGet(path) {
  return request(path);
}

export function apiPost(path, body) {
  return request(path, {
    method: 'POST',
    body: JSON.stringify(body),
  });
}

export function apiPostForm(path, formData) {
  return request(path, {
    method: 'POST',
    headers: {},
    body: formData,
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
