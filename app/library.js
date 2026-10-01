(() => {
  "use strict";
  const $ = id => document.getElementById(id);
  const statusLabels = { verified: "来源标注已核验", unverified: "待核验素材", generated: "生成练习题" };
  const AUTHORED_SOURCE_NAME = "我的题目"; // must match the authored-bank source name in app.js
  let busy = false;
  let refreshVersion = 0;
  let directoryId = "";
  let returnSkill = "";
  const skillNames = { reading: "阅读", listening: "听力", writing: "写作", speaking: "口语" };
  const packCache = new Map();

  // Immutable content IDs allow sharing downloads across all four modules.
  // Cache promises as well as results so simultaneous callers share one request.
  function loadPack(id) {
    if (packCache.has(id)) return packCache.get(id);
    const selectedDirectory = directoryId;
    const pending = request(`packs/${id}`).then(pack => {
      if (selectedDirectory !== directoryId) throw new Error("数据目录已更换，请重新选题");
      return pack;
    }).catch(error => {
      if (packCache.get(id) === pending) packCache.delete(id);
      throw error;
    });
    packCache.set(id, pending);
    if (packCache.size > 64) packCache.delete(packCache.keys().next().value);
    return pending;
  }

  async function request(path, options) {
    const headers = new Headers(options?.headers || {});
    if (!headers.has("X-ELP-Directory")) headers.set("X-ELP-Directory", directoryId);
    const response = await fetch(`/api/library/${path}`, { ...options, headers });
    const result = await response.json().catch(() => ({}));
    if (!response.ok) { const error = new Error(result.error || "题库服务不可用，请确认启动了新版程序"); error.status = response.status; throw error; }
    return result;
  }

  function report(message, error = false) {
    $("libraryStatus").textContent = message;
    $("libraryStatus").classList.toggle("is-error", error);
  }

  async function refresh() {
    const version = ++refreshVersion;
    try {
      const result = await request("packs");
      if (version !== refreshVersion) return;
      const list = $("libraryList"); list.replaceChildren();
      // Show one row per source (newest version); re-saved/edited banks accumulate versions.
      const seenSources = new Set();
      for (const pack of newestPacks(result.packs)) {
        const sourceKey = pack.source.url || pack.source.name;
        if (seenSources.has(sourceKey)) continue;
        seenSources.add(sourceKey);
        const row = document.createElement("details"); row.className = "library-row";
        const heading = document.createElement("summary");
        const title = document.createElement("strong"); title.textContent = pack.title;
        const detail = document.createElement("span");
        detail.textContent = `${pack.count} 组题目 · ${pack.source.name} · ${statusLabels[pack.source.status] || "待核验素材"}`;
        heading.append(title, detail); row.append(heading); list.append(row);
        row.addEventListener("toggle", async () => {
          if (!row.open || row.dataset.loaded) return;
          list.querySelectorAll("details").forEach(other => { if (other !== row) other.open = false; });
          row.dataset.loaded = "loading";
          const content = document.createElement("div"); content.className = "library-pack-questions";
          content.textContent = "正在读取题目……"; row.append(content);
          try { renderPackQuestions(content, pack.id, await loadPack(pack.id)); row.dataset.loaded = "true"; }
          catch (error) { content.textContent = `${error.message}，收起后重新打开可重试。`; delete row.dataset.loaded; row.addEventListener("toggle", () => { if (!row.open) content.remove(); }, { once: true }); }
        });
      }
      if (!result.packs.length) list.textContent = "还没有导入题包。";
    } catch (error) {
      if (version !== refreshVersion) return;
      const notice = document.createElement("p");
      notice.setAttribute("role", "alert");
      notice.textContent = `列表刷新失败：${error.message}。已保存的题库不受影响，请点击刷新。`;
      $("libraryList").replaceChildren(notice);
    }
  }

  function openImport(skill) {
    returnSkill = Object.hasOwn(skillNames, skill) ? skill : "";
    const back = $("libraryReturn");
    if (back) { back.classList.toggle("hidden", !returnSkill); back.textContent = `返回${skillNames[returnSkill] || ""}练习`; }
    location.hash = "library";
  }

  async function setAuthoredUnitHidden(packId, unitId, hidden) {
    // Retire/restore an authored unit without deleting the immutable pack (which would break
    // history): re-POST a new version with the unit's hidden flag toggled. Newest wins in listUnits.
    const pack = await loadPack(packId);
    const units = pack.units.map(unit => unit.id === unitId ? { ...unit, hidden: hidden || undefined } : unit);
    await request("packs", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ version: pack.version, title: pack.title, source: pack.source, units }) });
    packCache.clear();
  }

  function renderPackQuestions(content, packId, pack) {
    content.replaceChildren();
    const search = document.createElement("input"); search.type = "search";
    search.placeholder = "搜索这个题库的题目……"; search.setAttribute("aria-label", "搜索这个题库的题目");
    const list = document.createElement("div"), pages = document.createElement("div"); pages.className = "button-row";
    let page = 0;
    const render = () => {
      list.replaceChildren(); pages.replaceChildren();
      const matches = pack.units.filter(unit => `${unit.title} ${unit.prompt}`.toLowerCase().includes(search.value.trim().toLowerCase()));
      page = Math.min(page, Math.max(0, Math.ceil(matches.length / 20) - 1));
      for (const unit of matches.slice(page * 20, (page + 1) * 20)) {
        const row = document.createElement("article"); row.className = "library-question-choice";
        const text = document.createElement("div"), title = document.createElement("strong"), meta = document.createElement("small");
        title.textContent = unit.title; meta.textContent = `${skillNames[unit.skill]} · ${unit.part.replaceAll("-", " ")} · ${unit.minutes || 20} 分钟${unit.hidden ? " · 已隐藏" : ""}`;
        text.append(title, meta); row.append(text);
        if (!unit.hidden) {
          const choose = document.createElement("button"); choose.type = "button"; choose.className = "button button-secondary"; choose.textContent = "使用本题";
          choose.addEventListener("click", async () => {
            choose.disabled = true;
            try { await window.ELPPracticeActions.question({ packId, unit }); }
            catch (error) { meta.textContent = error.message; meta.setAttribute("role", "alert"); }
            finally { choose.disabled = false; }
          });
          row.append(choose);
        }
        if (pack.source.name === AUTHORED_SOURCE_NAME) {
          const toggle = document.createElement("button"); toggle.type = "button"; toggle.className = "button button-quiet"; toggle.textContent = unit.hidden ? "恢复" : "移除";
          toggle.addEventListener("click", async () => {
            toggle.disabled = true;
            try { await setAuthoredUnitHidden(packId, unit.id, !unit.hidden); report(unit.hidden ? "已恢复到选题" : "已隐藏，不再出现在选题"); refresh(); }
            catch (error) { toggle.textContent = error.message; toggle.disabled = false; }
          });
          row.append(toggle);
        }
        list.append(row);
      }
      if (!matches.length) list.textContent = "没有匹配题目，试试更短的关键词。";
      if (matches.length > 20) {
        const previous = document.createElement("button"), next = document.createElement("button"), label = document.createElement("span");
        previous.textContent = "上一页"; next.textContent = "下一页";
        for (const button of [previous, next]) { button.type = "button"; button.className = "button button-quiet"; }
        previous.disabled = page === 0; next.disabled = (page + 1) * 20 >= matches.length;
        previous.addEventListener("click", () => { page--; render(); }); next.addEventListener("click", () => { page++; render(); });
        label.textContent = `${page + 1} / ${Math.ceil(matches.length / 20)}`; pages.append(previous, label, next);
      }
    };
    search.addEventListener("input", () => { page = 0; render(); }); content.append(search, list, pages); render();
  }

  $("refreshLibrary").addEventListener("click", () => { report(""); refresh(); });
  window.addEventListener("elp:route", event => { if (event.detail === "library" && !busy) refresh(); });
  window.addEventListener("elp:storage-changed", () => {
    refreshVersion++; $("libraryList").replaceChildren();
    document.querySelectorAll(".question-picker-content").forEach(content => {content.replaceChildren();content.classList.add("hidden");});
    if (location.hash === "#library") refresh();
  });
  async function openPicker(host, skill, onPick) {
    const selectedDirectory = directoryId;
    const content = host.querySelector(".question-picker-content");
    if (!content.classList.contains("hidden")) { content.classList.add("hidden"); return; }
    content.classList.remove("hidden"); content.textContent = "正在读取题库……";
    const node = (tag, text) => { const result = document.createElement(tag); if (text !== undefined) result.textContent = text; return result; };
    try {
      const items = await listUnits(skill);
      if (selectedDirectory !== directoryId) return;
      content.replaceChildren();
      const search = node("input"); search.type = "search"; search.placeholder = "搜索题目"; search.setAttribute("aria-label", "搜索题目"); content.append(search);
      const filters = node("div"); filters.className = "button-row"; content.append(filters);
      const list = node("div"); list.className = "question-picker-list"; content.append(list);
      const pagination = node("div"); pagination.className = "button-row"; content.append(pagination);
      let filter = "all", page = 0;
      const pageSize = 30;
      const searchable = items.map(item => ({item, text: `${item.unit.title} ${item.unit.prompt}`.toLowerCase()}));
      const render = () => {
        list.replaceChildren(); pagination.replaceChildren();
        filters.querySelectorAll("button").forEach(button => button.setAttribute("aria-pressed", String(button.dataset.part === filter)));
        const query = search.value.toLowerCase();
        const matches = searchable.filter(({item, text}) => (filter === "all" || item.unit.part === filter) && text.includes(query)).map(({item}) => item);
        page = Math.min(page, Math.max(0, Math.ceil(matches.length / pageSize) - 1));
        for (const item of matches.slice(page * pageSize, (page + 1) * pageSize)) {
          const row = node("details"); row.className = "question-picker-row";
          const summary = node("summary"); summary.append(node("strong", item.unit.title), node("small", `${item.unit.part.replaceAll("-", " ")} · ${statusLabels[item.source.status]}`)); row.append(summary);
          row.addEventListener("toggle", () => {
            if (!row.open) return;
            list.querySelectorAll("details").forEach(other => { if (other !== row) other.open = false; });
            if (!row.querySelector("p")) row.insertBefore(node("p", item.unit.prompt), row.querySelector("button"));
          });
          const choose = node("button", "使用本题"); choose.type = "button"; choose.className = "button button-secondary";
          choose.addEventListener("click", async () => {
            const controls = [...content.querySelectorAll("button, input")]; controls.forEach(control => { control.disabled = true; });
            try { await onPick(item); content.classList.add("hidden"); }
            catch (error) { const feedback = node("p", error.message); feedback.setAttribute("role", "alert"); row.append(feedback); }
            finally { controls.forEach(control => { control.disabled = false; }); }
          }); row.append(choose); list.append(row);
        }
        if (!matches.length) {
          list.append(node("p", items.length ? "没有匹配的题目。" : "还没有这类题目，先导入题库就能直接选题练习。"));
          if (!items.length) { const add = node("button", "去导入题库"); add.type = "button"; add.className = "button button-primary"; add.addEventListener("click", () => openImport(skill)); list.append(add); }
        }
        if (matches.length > pageSize) {
          const previous = node("button", "上一页"), next = node("button", "下一页");
          for (const control of [previous, next]) { control.type = "button"; control.className = "button button-quiet"; }
          previous.disabled = page === 0; next.disabled = (page + 1) * pageSize >= matches.length;
          previous.addEventListener("click", () => { page--; render(); });
          next.addEventListener("click", () => { page++; render(); });
          pagination.append(previous, node("span", `${page + 1} / ${Math.ceil(matches.length / pageSize)} · ${matches.length} 题`), next);
        }
      };
      const parts = skill === "writing" ? ["Task-1-Academic", "Task-1-General", "Task-2"] : ["p1", "p2", "p3"];
      for (const part of ["all", ...parts]) {
        const control = node("button", part === "all" ? "全部" : part.replaceAll("-", " ").replace(/^p/, "Part ")); control.type = "button"; control.className = "button button-quiet"; control.dataset.part = part;
        control.addEventListener("click", () => { filter = part; page = 0; render(); }); filters.append(control);
      }
      search.addEventListener("input", () => { page = 0; render(); }); render();
    } catch (error) { content.textContent = error.message; }
  }
  async function listUnits(skill) {
    const selectedDirectory = directoryId;
    const index = await request("packs"); const units = new Map();
    const entries = newestPacks(index.packs).filter(entry => !entry.skills || entry.skills[skill]);
    // Bound concurrency to avoid flooding the local disk with large banks.
    for (let offset = 0; offset < entries.length; offset += 4) {
      const batch = entries.slice(offset, offset + 4);
      const packs = await Promise.all(batch.map(entry => loadPack(entry.id)));
      if (selectedDirectory !== directoryId) throw new Error("数据目录已更换，请重新选题");
      for (const [position, pack] of packs.entries()) {
        const entry = batch[position];
        for (const unit of pack.units.filter(unit => unit.skill === skill && !unit.hidden)) {
          // Retain old packs for history, but offer the latest source revision once.
          const key = JSON.stringify([pack.source.url || pack.source.name, unit.id]);
          if (!units.has(key)) units.set(key, { packId: entry.id, unit, source: pack.source });
        }
      }
    }
    return [...units.values()];
  }
  function newestPacks(packs) {
    const key = value => String(value || "").replace(/(?:\.(\d+))?Z$/, (_, fraction = "") => `.${fraction.padEnd(9,"0")}Z`);
    return [...packs].sort((a,b) => key(a.importedAt) < key(b.importedAt) ? 1 : key(a.importedAt) > key(b.importedAt) ? -1 : 0);
  }
  window.ELPLibrary = Object.freeze({ request, refresh, openPicker, openImport, listUnits, loadPack, newestPacks, get returnSkill() { return returnSkill; }, setBusy(value) { busy = value; }, get directoryId() { return directoryId; }, get busy() { return busy; }, setDirectory(value) { if (value !== directoryId) packCache.clear(); directoryId = value; } });
})();
