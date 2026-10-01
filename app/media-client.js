(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  if (root) root.ELPMedia = api;
})(typeof window === "object" ? window : null, () => {
  "use strict";
  const isStored = value => typeof value === "string" && /^\/api\/study\/media\/[a-f0-9]{64}\.(png|jpg|webp|gif|mp3|wav|ogg|webm|m4a)$/.test(value);
  function dataURL(blob) {
    return new Promise((resolve, reject) => { const reader = new FileReader(); reader.onload = () => resolve(String(reader.result)); reader.onerror = () => reject(new Error("无法读取媒体")); reader.readAsDataURL(blob); });
  }
  async function inline(value, media = false) {
    if (media && isStored(value)) { const response = await fetch(value); if (!response.ok) throw new Error("本地媒体读取失败，请检查档案完整性"); return dataURL(await response.blob()); }
    if (Array.isArray(value)) { const result = []; for (const item of value) result.push(await inline(item, media)); return result; }
    if (value && typeof value === "object") { const result = {}; for (const [key, item] of Object.entries(value)) result[key] = await inline(item, ["audio","images","promptImages"].includes(key)); return result; }
    return value;
  }
  async function images(values) {
    const result = await inline(values,true);
    if (result.some(value => typeof value !== "string" || !/^data:image\/(png|jpe?g|gif|webp);base64,/i.test(value))) throw new Error("题图无法读取，已停止批改以免遗漏图片");
    return result;
  }
  return Object.freeze({ isStored, dataURL, inline, images });
});
