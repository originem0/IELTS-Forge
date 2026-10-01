(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  if (root) root.ELPStorage = api;
})(typeof window === "object" ? window : null, () => {
  "use strict";
  const collections = new Set(["writings", "speaking", "mistakes"]);
  function create({ request = (...args) => fetch(...args), onStatus = () => {}, onError = () => {} } = {}) {
    let root, revision = "", directory = "", queue = Promise.resolve(), pending = 0;
    let metadata = new Set(), orders = new Set(), records = new Map(), serial = 0;
    const versions = new Map(), rawValues = new WeakMap(), proxies = new WeakMap();
    const raw = value => rawValues.get(value) || value;
    const hasChanges = () => metadata.size || orders.size || records.size;
    function markRecord(collection, record) {
      if (!record || typeof record.id !== "string") return;
      const key = `${collection}/${record.id}`;
      versions.set(key, ++serial);
      if (!records.has(collection)) records.set(collection, new Set());
      records.get(collection).add(record.id);
    }
    function wrap(value, scope = {}) {
      value = raw(value);
      if (!value || typeof value !== "object") return value;
      if (proxies.has(value)) return proxies.get(value);
      const changed = (target, key, next) => {
        if (!scope.key) {
          if (collections.has(key)) { orders.add(key); for (const record of next || []) markRecord(key, record); }
          else metadata.add(key);
        } else if (!collections.has(scope.key)) metadata.add(scope.key);
        else if (scope.record) { markRecord(scope.key, scope.record); if (key === "id") orders.add(scope.key); }
        else { orders.add(scope.key); if (key !== "length") markRecord(scope.key, next); }
      };
      const proxy = new Proxy(value, {
        get(target, key) {
          const child = Reflect.get(target, key);
          if (typeof key === "symbol") return child;
          const next = !scope.key ? { key } : collections.has(scope.key) && Array.isArray(target) && !scope.record && child && typeof child === "object" ? { key: scope.key, record: raw(child) } : scope;
          return wrap(child, next);
        },
        set(target, key, next) { next = raw(next); const same = target[key] === next; Reflect.set(target, key, next); if (!same) changed(target, key, next); return true; },
        deleteProperty(target, key) { if (Object.hasOwn(target, key)) { delete target[key]; changed(target, key, null); } return true; }
      });
      proxies.set(value, proxy); rawValues.set(proxy, value); return proxy;
    }
    function adopt(value, response = {}) {
      if (pending) throw new Error("仍有数据正在保存，不能替换档案");
      root = raw(value); revision = response.revision || ""; directory = response.storage?.directoryId || "";
      metadata = new Set(); orders = new Set(); records = new Map(); versions.clear();
      return wrap(root);
    }
    async function send(method, payload) {
      const response = await request("/api/data", { method, headers: { "Content-Type": "application/json", "X-ELP-Directory": directory, "If-Match": revision }, body: JSON.stringify(payload) });
      const result = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(result.error || "无法写入本地数据文件");
      revision = result.revision || revision; onStatus(result.storage); return result;
    }
    function enqueue(action) {
      pending++;
      const task = queue.catch(() => {}).then(action).catch(error => { onError(error.message); throw error; }).finally(() => { pending--; });
      queue = task; task.catch(() => {}); return task;
    }
    function save() {
      return enqueue(async () => {
        if (!hasChanges()) return send("PATCH", {metadata:{},collections:{}});
        let result = { saved: true, revision };
        while (hasChanges()) {
          const keys = metadata, lists = orders, changed = records;
          metadata = new Set(); orders = new Set(); records = new Map();
          const patch = { metadata: {}, collections: {} }, sentVersions = new Map();
          for (const key of keys) patch.metadata[key] = root[key] ?? null;
          for (const collection of new Set([...lists, ...changed.keys()])) {
            const entries = root[collection] || [];
            const ids = changed.get(collection) || new Set();
            const upsert = entries.filter(item => ids.has(item.id));
            for (const record of upsert) sentVersions.set(`${collection}/${record.id}`, versions.get(`${collection}/${record.id}`));
            patch.collections[collection] = { upsert };
            if (lists.has(collection)) patch.collections[collection].order = entries.map(item => item.id);
          }
          try {
            result = await send("PATCH", patch);
            for (const [collection, entries] of Object.entries(result.records || {})) for (const record of entries) {
              const key = `${collection}/${record.id}`;
              if (versions.get(key) === sentVersions.get(key)) {
                const current = root[collection].find(item => item.id === record.id);
                if (current) Object.assign(current, record);
              }
            }
          } catch (error) {
            for (const key of keys) metadata.add(key);
            for (const key of lists) orders.add(key);
            for (const [collection, ids] of changed) { if (!records.has(collection)) records.set(collection, new Set()); for (const id of ids) records.get(collection).add(id); }
            throw error;
          }
        }
        return result;
      });
    }
    return {
      adopt, save,
      replace: value => enqueue(() => send("PUT", { data: value })),
      clone: value => JSON.parse(JSON.stringify(value)),
      get revision() { return revision; },
      get dirty() { return Boolean(pending || hasChanges()); }
    };
  }
  return Object.freeze({ create });
});
