import type { DNSCheck } from './dnsCheck';

export interface DNSHistory {
  previous: DNSCheck | null;
  current: DNSCheck | null;
}

export const emptyDNSHistory: DNSHistory = { previous: null, current: null };

/** 一个事务里移动最近两次成功结果，多个标签页同时复测时也只保留相邻两次。 */
export async function dnsHistory(next?: DNSCheck, lastInPage?: DNSCheck | null): Promise<DNSHistory> {
  const db = await new Promise<IDBDatabase>((resolve, reject) => {
    const open = indexedDB.open('detectradar-dns', 1);
    open.onupgradeneeded = () => open.result.createObjectStore('checks');
    open.onsuccess = () => resolve(open.result);
    open.onerror = () => reject(open.error);
    open.onblocked = () => reject(new Error('DNS 记录暂不可用'));
  });

  try {
    return await new Promise<DNSHistory>((resolve, reject) => {
      const tx = db.transaction('checks', next ? 'readwrite' : 'readonly');
      const store = tx.objectStore('checks');
      const read = store.get('recent');
      let history = emptyDNSHistory;
      read.onsuccess = () => {
        history = read.result ?? emptyDNSHistory;
        if (next) {
          // 若上次写入因存储受限而失败，本页的结果可能比数据库里的更新。
          const previous = lastInPage && (!history.current || lastInPage.time >= history.current.time)
            ? lastInPage : history.current;
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
