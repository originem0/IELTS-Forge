// A small wire-protocol mock for UI tests; real persistence is tested in Go and
// library-ui.cjs against the actual HTTP handlers and filesystem.
module.exports = function applyStateRequest(data, payload, method) {
  if (method === 'PUT') return payload.data;
  const result = {...data, ...payload.metadata};
  for (const [key, patch] of Object.entries(payload.collections || {})) {
    const records = new Map((result[key] || []).map(record => [record.id, record]));
    for (const record of patch.upsert || []) records.set(record.id, record);
    result[key] = patch.order ? patch.order.map(id => records.get(id)) : [...records.values()];
  }
  return result;
};
