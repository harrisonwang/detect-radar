export interface RecentHistory<T> {
  previous: T | null;
  current: T | null;
}

export function emptyHistory<T>(): RecentHistory<T> {
  return { previous: null, current: null };
}

export async function recentHistory<T extends { time: number }>(
  dbName: string,
  next?: T,
  lastInPage?: T | null,
): Promise<RecentHistory<T>> {
  const db = await new Promise<IDBDatabase>((resolve, reject) => {
    const open = indexedDB.open(dbName, 1);
    open.onupgradeneeded = () => open.result.createObjectStore('checks');
    open.onsuccess = () => resolve(open.result);
    open.onerror = () => reject(open.error);
    open.onblocked = () => reject(new Error('IndexedDB history unavailable'));
  });

  try {
    return await new Promise<RecentHistory<T>>((resolve, reject) => {
      const tx = db.transaction('checks', next ? 'readwrite' : 'readonly');
      const store = tx.objectStore('checks');
      const read = store.get('recent');
      let history = emptyHistory<T>();
      read.onsuccess = () => {
        history = (read.result as RecentHistory<T> | undefined) ?? emptyHistory<T>();
        if (next) {
          const previous =
            lastInPage && (!history.current || lastInPage.time >= history.current.time)
              ? lastInPage
              : history.current;
          history = { previous, current: next };
          store.put(history, 'recent');
        }
      };
      tx.oncomplete = () => resolve(history);
      tx.onabort = () => reject(tx.error);
    });
  } finally {
    db.close();
  }
}
