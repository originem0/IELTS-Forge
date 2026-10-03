(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  if (root) root.ELPStorage = api;
})(typeof window === "object" ? window : null, () => {
  "use strict";
  const collections = new Set(["writings", "speaking", "mistakes"]);
  function create({ request = (...args) => fetch(...args), onStatus = () => {}, onError = () => {} } = {}) {
    let root, revision = "", directory = "", queue = Promise.resolve(), pending = 0;
    let outstanding = null;
    let baseline = {};
    const copy = value => value === undefined ? null : JSON.parse(JSON.stringify(value));
    function remember(patch, result) {
      for (const [key,value] of Object.entries(patch.metadata || {})) baseline[key] = copy(value);
      for (const [key,change] of Object.entries(patch.collections || {})) {
        const entries = new Map((baseline[key] || []).map(item => [item.id,item]));
        for (const item of change.upsert || []) entries.set(item.id,copy(item));
        for (const item of result.records?.[key] || []) entries.set(item.id,copy(item));
        baseline[key] = (change.order || [...entries.keys()]).map(id => entries.get(id));
      }
    }
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
      outstanding = null;
      baseline = copy(response.data ?? root);
      return wrap(root);
    }
    async function send(method, payload) {
      let body = JSON.stringify(payload);
      async function deliver(operation) {
        for (let attempt=0; attempt<2; attempt++) {
          try {
            const response = await request("/api/data", { method:operation.method, headers:operation.headers, body:operation.body });
            if (!response.ok) {
              const result = await response.json().catch(() => ({}));
              const error = new Error(result.error || "无法写入本地数据文件"); error.definitive = true; throw error;
            }
            const result = await response.json();
            if (operation.method === "PATCH") remember(JSON.parse(operation.body), result);
            revision = result.revision || revision; outstanding = null; onStatus(result.storage); return result;
          } catch (error) {
            if (error.definitive) { outstanding = null; throw error; }
            if (attempt) throw error;
          }
        }
      }
      // Resolve an uncertain operation before sending edits made since it.
      if (outstanding) {
        const prior = outstanding, result = await deliver(prior);
        if (prior.method === method && prior.body === body) return result;
        if (method === "PATCH" && payload.base) {
          for (const key of Object.keys(payload.base.metadata)) payload.base.metadata[key] = baseline[key] ?? null;
          for (const [key,records] of Object.entries(payload.base.records)) {
            const entries = new Map((baseline[key] || []).map(item => [item.id,item]));
            for (const id of Object.keys(records)) records[id] = entries.get(id) ?? null;
            if (payload.base.orders[key]) payload.base.orders[key] = [...entries.keys()];
          }
          body = JSON.stringify(payload);
        }
      }
      outstanding = {method,body,headers:{"Content-Type":"application/json","X-ELP-Directory":directory,"If-Match":revision,"X-ELP-Commit":globalThis.crypto.randomUUID()}};
      return deliver(outstanding);
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
          const patch = { metadata: {}, collections: {}, base:{metadata:{},records:{},orders:{}} }, sentVersions = new Map();
          for (const key of keys) { patch.metadata[key] = root[key] ?? null; patch.base.metadata[key] = baseline[key] ?? null; }
          for (const collection of new Set([...lists, ...changed.keys()])) {
            const entries = root[collection] || [];
            const ids = changed.get(collection) || new Set();
            const upsert = entries.filter(item => ids.has(item.id));
            for (const record of upsert) sentVersions.set(`${collection}/${record.id}`, versions.get(`${collection}/${record.id}`));
            patch.collections[collection] = { upsert };
            const previous = new Map((baseline[collection] || []).map(item => [item.id,item]));
            patch.base.records[collection] = Object.fromEntries(upsert.map(item => [item.id,previous.get(item.id) ?? null]));
            if (lists.has(collection)) {
              patch.collections[collection].order = entries.map(item => item.id);
              patch.base.orders[collection] = [...previous.keys()];
              const retained = new Set(entries.map(item => item.id));
              for (const [id,item] of previous) if (!retained.has(id)) patch.base.records[collection][id] = item;
            }
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
  function createRemoval({save,onError}) {
    let deletionBusy = false;
  async function persistRemoval({ prepare = async () => true, remove, restore, after, draft }) {
    if (deletionBusy) return false;
    deletionBusy = true;
    let release = () => {}, changed = false;
    try {
      if (!await prepare()) return false;
      if (draft) release = draft.hold();
      remove(); changed = true;
      try { await save(); } catch (error) { restore(); changed = false; throw error; }
      await after();
      return true;
    } catch (error) {
      onError(changed ? `删除已保存，页面更新失败：${error.message}` : `删除未保存，记录已保留：${error.message}`);
      return false;
    } finally { release(); deletionBusy = false; }
  }

    return persistRemoval;
  }
  return Object.freeze({ create, createRemoval });
});
