// IndexedDB device identity helper

const DB_NAME = "chiban_pwa";
const STORE_NAME = "device";
const KEY_DEVICE_ID = "device_id";

function openDB(): Promise<IDBDatabase> {
  const { promise, resolve, reject } = Promise.withResolvers<IDBDatabase>();
  const req = indexedDB.open(DB_NAME, 1);
  req.onupgradeneeded = () => {
    const db = req.result;
    if (!db.objectStoreNames.contains(STORE_NAME)) {
      db.createObjectStore(STORE_NAME);
    }
  };
  req.onsuccess = () => resolve(req.result);
  req.onerror = () => reject(req.error);
  return promise;
}

export async function getOrCreateDeviceId(): Promise<string> {
  if (typeof window === "undefined" || !("indexedDB" in window)) {
    return "fallback-device-id";
  }

  try {
    const db = await openDB();
    const { promise, resolve, reject } = Promise.withResolvers<string>();
    const tx = db.transaction(STORE_NAME, "readwrite");
    const store = tx.objectStore(STORE_NAME);
    const getReq = store.get(KEY_DEVICE_ID);

    getReq.onsuccess = () => {
      if (getReq.result) {
        resolve(getReq.result);
      } else {
        const newId = crypto.randomUUID();
        store.put(newId, KEY_DEVICE_ID);
        resolve(newId);
      }
    };
    getReq.onerror = () => reject(getReq.error);
    return await promise;
  } catch {
    // Fallback if IndexedDB is blocked
    let localId = localStorage.getItem(KEY_DEVICE_ID);
    if (!localId) {
      localId = crypto.randomUUID();
      localStorage.setItem(KEY_DEVICE_ID, localId);
    }
    return localId;
  }
}
