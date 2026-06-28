// Khởi tạo cấu hình và Định nghĩa Store
const DB_NAME = "mini-banking";
const DB_VERSION = 1;

export const STORES = {
  pki: "pki",
} as const;

export type StoreName = (typeof STORES)[keyof typeof STORES];
let dbPromise: Promise<IDBDatabase> | null = null;

// Hàm khởi tạo kết nối
function openDB(): Promise<IDBDatabase> {
  if (dbPromise) return dbPromise;

  dbPromise = new Promise<IDBDatabase>((resolve, reject) => {
    if (typeof indexedDB === "undefined") {
      reject(new Error("IndexedDB is not available in this environment"));
      return;
    }

    const request = indexedDB.open(DB_NAME, DB_VERSION);

    request.onupgradeneeded = () => {
      const db = request.result;
      for (const store of Object.values(STORES)) {
        if (!db.objectStoreNames.contains(store)) {
          db.createObjectStore(store);
        }
      }
    };

    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error ?? new Error("Failed to open IndexedDB"));
  });

  return dbPromise;
}

// Hàm thực hiện transaction
function withStore<T>(
  store: StoreName,
  mode: IDBTransactionMode,
  fn: (store: IDBObjectStore) => IDBRequest<T>,
): Promise<T> {
  return openDB().then(
    (db) =>
      new Promise<T>((resolve, reject) => {
        const tx = db.transaction(store, mode);
        const request = fn(tx.objectStore(store));
        tx.oncomplete = () => resolve(request.result);
        tx.onabort = () => reject(tx.error ?? new Error("IndexedDB transaction aborted"));
        tx.onerror = () => reject(tx.error ?? request.error ?? new Error("IndexedDB error"));
      }),
  );
}

// CRUD
/** Read a value by key. Resolves to `undefined` when the key is absent. */
export function idbGet<T>(store: StoreName, key: string): Promise<T | undefined> {
  return withStore<T | undefined>(store, "readonly", (s) => s.get(key) as IDBRequest<T | undefined>);
}

/** Insert or overwrite a value at `key`. */
export function idbPut<T>(store: StoreName, key: string, value: T): Promise<void> {
  return withStore<IDBValidKey>(store, "readwrite", (s) => s.put(value as unknown, key)).then(() => undefined);
}

/** Insert or overwrite multiple values atomically in one object-store transaction. */
export function idbPutMany(
  store: StoreName,
  entries: ReadonlyArray<{ key: string; value: unknown }>,
): Promise<void> {
  return openDB().then(
    (db) =>
      new Promise<void>((resolve, reject) => {
        const tx = db.transaction(store, "readwrite");
        const objectStore = tx.objectStore(store);

        for (const entry of entries) {
          objectStore.put(entry.value, entry.key);
        }

        tx.oncomplete = () => resolve();
        tx.onabort = () => reject(tx.error ?? new Error("IndexedDB transaction aborted"));
        tx.onerror = () => reject(tx.error ?? new Error("IndexedDB error"));
      }),
  );
}

/** Delete a value by key (no-op if absent). */
export function idbDelete(store: StoreName, key: string): Promise<void> {
  return withStore<undefined>(store, "readwrite", (s) => s.delete(key) as IDBRequest<undefined>).then(() => undefined);
}

/** Whether a key exists in the store. */
export function idbHas(store: StoreName, key: string): Promise<boolean> {
  return withStore<IDBValidKey | undefined>(store, "readonly", (s) => s.getKey(key)).then((k) => k !== undefined);
}
