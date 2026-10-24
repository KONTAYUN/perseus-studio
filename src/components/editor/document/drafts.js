const DATABASE = 'perseus-document-drafts-v1';
let opening;
function database() {
  if (!opening) opening = new Promise((resolve, reject) => {
    const request = indexedDB.open(DATABASE, 2);
    request.onupgradeneeded = () => {
      if (!request.result.objectStoreNames.contains('drafts')) request.result.createObjectStore('drafts');
      if (!request.result.objectStoreNames.contains('archives')) request.result.createObjectStore('archives');
    };
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => { opening = null; reject(request.error); };
  });
  return opening;
}
export async function draftOperation(user, path, operation, value) {
  if (!user || !path) return null;
  const db = await database();
  return new Promise((resolve, reject) => {
    const archive = operation === 'archive' || operation === 'archives';
    const transaction = db.transaction(archive ? 'archives' : 'drafts', ['get', 'archives'].includes(operation) ? 'readonly' : 'readwrite');
    const store = transaction.objectStore(archive ? 'archives' : 'drafts'), key = JSON.stringify([user, path]);
    const request = operation === 'archive' ? store.put({ ...value, user, path }, JSON.stringify([user, path, value.at]))
      : operation === 'archives' ? store.getAll() : operation === 'put' ? store.put(value, key) : store[operation](key);
    transaction.oncomplete = () => resolve(operation === 'archives' ? request.result.filter(item => item.user === user && item.path === path).sort((a, b) => b.at - a.at) : request.result);
    transaction.onerror = () => reject(transaction.error);
    transaction.onabort = () => reject(transaction.error || new Error('草稿保存已中断'));
  });
}
