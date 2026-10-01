(() => {
  "use strict";
  const api = (...args) => window.ELPLibrary.request(...args);
  let activeSkill = "reading";
  const root = () => document.getElementById(`${activeSkill}Workspace`);
  const skillName = () => activeSkill === "reading" ? "阅读" : "听力";
  let playingAudio = null;
  let generation = 0;
  let session = null;
  const statusNames = { verified: "来源标注已核验", unverified: "待核验素材", generated: "生成练习题" };
  const kindNames = { text: "填空", single: "选择 / 判断", matching: "匹配", multiple: "多选" };
  const el = (tag, text, className) => {
    const node = document.createElement(tag);
    if (text !== undefined) node.textContent = text;
    if (className) node.className = className;
    return node;
  };
  const button = (label, action, style = "button-secondary") => {
    const node = el("button", label, `button ${style}`); node.type = "button";
    node.addEventListener("click", async () => {
      if (node.disabled) return; node.disabled = true;
      try { await action(); } catch (error) { showError(error); } finally { node.disabled = node.dataset.busyLocked === "true"; }
    });
    return node;
  };
  const go = route => { location.hash = route; };
  const clock = seconds => `${Math.floor(seconds / 60).toString().padStart(2, "0")}:${(seconds % 60).toString().padStart(2, "0")}`;
  const scopedUnit = (unit, record) => !record.reviewQuestions?.length ? unit : { ...unit, groups: unit.groups.map(group => ({ ...group, questions: group.questions.filter(q => record.reviewQuestions.includes(q.id)) })).filter(group => group.questions.length) };
  function showError(error) {
    const target = document.getElementById("objectiveStatus") || root();
    let message = document.getElementById("objectiveError");
    if (!message) { message = el("p", "", "objective-error"); message.id = "objectiveError"; message.setAttribute("role", "alert"); target.append(message); }
    message.textContent = error.message || String(error);
  }
  function status(message) { const target = document.getElementById("objectiveSave"); if (target) target.textContent = message; }
  function questionImages(unit) {
    const row=el("div",undefined,"objective-images");
    for(const [index,id] of (unit.images||[]).entries()){
      const image=el("img");image.src=`/api/library/media/${id}`;image.alt=`题图 ${index+1}`;
      const zoom=button("放大题图",()=>window.dispatchEvent(new CustomEvent("elp:question-image",{detail:{src:image.src,alt:image.alt}})),"button-quiet");
      const item=el("figure");item.append(image,zoom);row.append(item);
    }
    return row;
  }
  function questionTable(group) {
    const scroll=el("div",undefined,"objective-table-scroll");const table=el("table");table.setAttribute("aria-label","题目表格");
    for(const [index,row] of group.table.entries()){
      const tr=el("tr");for(const cell of row)tr.append(el(index===0?"th":"td",cell));table.append(tr);
    }
    scroll.append(table);return scroll;
  }
  async function examCatalogue(token) {
    const top = shell(`${skillName()}整套模拟`, "只有结构完整的试卷出现在这里。生成素材会明确标记，结果只显示答对题数。");
    top.append(button("返回概览", () => go(activeSkill), "button-quiet"));
    const list = el("section", undefined, "panel objective-library"); root().append(list);
    const index = await api("packs"); const seen = new Set();
    if (token !== generation) return;
    if (!index.packs.some(entry => !entry.skills || entry.skills[activeSkill])) return window.ELPLibrary.openImport(activeSkill);
    for (const entry of window.ELPLibrary.newestPacks(index.packs)) {
      const data = await pack(entry.id);
      if (token !== generation) return;
      for (const exam of (data.exams || []).filter(exam => exam.skill === activeSkill)) {
        const key = `${data.source.url || data.source.name}/${exam.id}`; if (seen.has(key)) continue; seen.add(key);
        const row = el("article", undefined, "objective-library-row"); const copy = el("div");
        copy.append(el("h3", exam.title), el("p", `${exam.unitIds.length} 部分 · 40 个计分点 · ${exam.minutes} 分钟 · ${statusNames[data.source.status]}`));
        row.append(copy, button("开始模拟", () => start(entry.id, exam.id, "", [], exam.id), "button-primary")); list.append(row);
      }
    }
    if (!list.children.length) list.append(el("p", "还没有完整试卷。可以先做单篇练习，或导入包含完整试卷的题库。"), button("管理题库", () => window.ELPLibrary.openImport(activeSkill), "button-quiet"));
  }
  function mockAudioPanel(unit, s) {
    const panel = el("section", undefined, "panel objective-audio objective-mock-audio");
    const heading = el("h3"); const audio = el("audio"); audio.preload = "metadata";
    audio.setAttribute("aria-label", "模拟听力录音"); s.audio = audio; playingAudio = audio;
    const notice = el("p", "按顺序原速播放一次。离开后可从已保存位置继续，模拟计时不会暂停。", "muted");
    const play = button(s.record.audioEnded ? "录音已播放完毕" : "开始或继续播放", async () => {
      if (s.record.audioEnded) return;
      await save(s); loadTrack(); await audio.play();
    }, "button-primary");
    const loadTrack = () => {
      const track = unit.tracks[s.record.audioTrack || 0]; heading.textContent = `Section ${track.part} · ${track.title}`;
      const source = `/api/library/media/${track.audio}`;
      if (audio.getAttribute("src") !== source || audio.error) { s.switchingAudio = true; audio.src = source; audio.load(); }
    };
    const refreshButton = () => { play.disabled = Boolean(s.record.audioEnded || !audio.paused); play.dataset.busyLocked = String(play.disabled); play.textContent = s.record.audioEnded ? "录音已播放完毕" : audio.paused ? "开始或继续播放" : "录音播放中"; };
    audio.addEventListener("loadedmetadata", () => { audio.currentTime = Math.min(s.record.audioSeconds || 0, Math.max(0, audio.duration-.1)); s.switchingAudio = false; });
    audio.addEventListener("canplay", () => { s.audioFailed = false; });
    audio.addEventListener("error", () => { if (playingAudio !== audio) return; s.audioFailed = true; play.disabled = false; play.dataset.busyLocked = "false"; notice.textContent = "音频加载失败，答案仍保留。请恢复附件后继续播放，计时仍在进行。"; });
    audio.addEventListener("play", refreshButton);
    audio.addEventListener("pause", refreshButton);
    audio.addEventListener("timeupdate", () => {
      if (s !== session || s.record.status !== "draft" || s.submitting || s.switchingAudio) return;
      const seconds = Math.floor(audio.currentTime);
      if (seconds !== s.record.audioSeconds) { s.record.audioSeconds = seconds; s.version++; save(s).catch(error => { audio.pause(); showError(error); }); }
    });
    audio.addEventListener("ended", async () => {
      if (s !== session || s.record.status !== "draft" || s.submitting) return;
      s.switchingAudio = true;
      if ((s.record.audioTrack || 0) === unit.tracks.length-1) s.record.audioEnded = true;
      else { s.record.audioTrack = (s.record.audioTrack || 0)+1; s.record.audioSeconds = 0; }
      s.version++;
      try { await save(s); if (!s.record.audioEnded && s === session) { loadTrack(); await audio.play(); } }
      catch (error) { showError(error); }
      refreshButton();
    });
    panel.append(heading, audio, play, notice); loadTrack(); refreshButton(); return panel;
  }
  function audioPanel(unit, s = null, headingText) {
    if (s?.record.mode === "simulation" && unit.tracks?.length) return mockAudioPanel(unit, s);
    const panel = el("section", undefined, "panel objective-audio");
    panel.append(el("h3", headingText || `Section ${unit.part} · ${s ? "听音作答" : "回听复盘"}`));
    const audio = el("audio"); audio.controls = true; audio.preload = "metadata";
    audio.src = `/api/library/media/${unit.tracks?.[s?.record.audioTrack || 0]?.audio || unit.audio}`; audio.setAttribute("aria-label", "听力音频");
    playingAudio = audio; if (s) s.audio = audio;
    const mediaStatus = el("p", "", "objective-error hidden"); mediaStatus.setAttribute("role", "status");
    const retry = button("重试加载音频", () => audio.load(), "button-secondary"); retry.classList.add("hidden");
    audio.addEventListener("loadedmetadata", () => {
      if (s && Number.isFinite(audio.duration)) audio.currentTime = Math.min(s.record.audioSeconds || 0, Math.max(0, audio.duration - .1));
      audio.playbackRate = s?.record.audioRate || 1;
    });
    audio.addEventListener("error", () => {
      if (audio !== playingAudio) return;
      if (s) s.audioFailed = true;
      mediaStatus.textContent = "音频无法读取，答案仍保留。请检查题包附件后重试。";
      mediaStatus.classList.remove("hidden"); retry.classList.remove("hidden");
    });
    audio.addEventListener("canplay", () => { if (s) s.audioFailed = false; mediaStatus.classList.add("hidden"); retry.classList.add("hidden"); });
    const persistPosition = () => {
      if (!s || s !== session || s.record.status !== "draft" || s.submitting) return;
      const seconds = Math.floor(audio.currentTime);
      if (seconds !== s.record.audioSeconds) { s.record.audioSeconds = seconds; s.version++; }
    };
    audio.addEventListener("timeupdate", persistPosition);
    audio.addEventListener("ratechange", () => {
      if (s && s === session && s.record.status === "draft" && !s.submitting && audio.playbackRate !== (s.record.audioRate || 1)) { s.record.audioRate = audio.playbackRate; dirty(s); }
    });
    for (const event of ["pause", "seeked", "ended"]) audio.addEventListener(event, () => {
      persistPosition();
      if (s && s === session && s.record.status === "draft" && !s.submitting) save(s).catch(showError);
    });
    audio.addEventListener("play", () => { if (s?.paused || s?.submitting) { audio.pause(); status("练习已暂停，继续后再播放"); } });
    panel.append(audio, mediaStatus, retry);
    if (unit.tracks?.length > 1) {
      const tracks = el("div", undefined, "button-row");
      unit.tracks.forEach((track, index) => {
        const choose = button(`Section ${track.part}`, () => {
          if (s?.paused) return;
          audio.pause(); if (s) { s.record.audioTrack = index; s.record.audioSeconds = 0; dirty(s); }
          audio.src = `/api/library/media/${track.audio}`; audio.load();
          tracks.querySelectorAll("button").forEach((node, i) => node.setAttribute("aria-pressed", String(i === index)));
        }, "button-quiet"); choose.setAttribute("aria-pressed", String(index === (s?.record.audioTrack || 0))); tracks.append(choose);
      }); panel.append(tracks);
    }
    const actions = el("div", undefined, "button-row");
    actions.append(button("后退 10 秒", () => { if (!s?.paused) audio.currentTime = Math.max(0, audio.currentTime - 10); }, "button-quiet"));
    for (const rate of [.75, 1, 1.25]) {
      const control = button(`${rate}×`, () => {
        if (s?.paused) return;
        audio.playbackRate = rate;
        if (s) { s.record.audioRate = rate; dirty(s); }
        actions.querySelectorAll("[data-rate]").forEach(node => node.setAttribute("aria-pressed", String(Number(node.dataset.rate) === rate)));
      }, "button-quiet");
      control.dataset.rate = rate; control.setAttribute("aria-pressed", String(rate === (s?.record.audioRate || 1))); actions.append(control);
    }
    panel.append(actions, el("p", s ? "练习可暂停和回放；提交后显示原文。播放位置会与答案一起保存。" : "可调速回听。原文按来源保留，不生成未经核对的逐句时间戳。", "muted"));
    return panel;
  }
  async function pack(id) {
    return window.ELPLibrary.loadPack(id);
  }
  function shell(title, subtitle) {
    root().classList.remove("objective-active", "objective-report");
    const top = el("section", undefined, "panel objective-heading");
    const copy = el("div"); copy.append(el("span", activeSkill === "reading" ? "READ · UNDERSTAND · REVIEW" : "LISTEN · UNDERSTAND · REVIEW", "kicker"), el("h2", title), el("p", subtitle));
    top.append(copy); root().replaceChildren(top);
    const feedback = el("div"); feedback.id = "objectiveStatus"; root().append(feedback);
    return top;
  }
  function unitCard(unit, source, action) {
    const row = el("article", undefined, "objective-library-row");
    row.dataset.unitId = unit.id;
    const text = el("div");
    // A listening unit with no question groups is a listen-only resource (audio + transcript);
    // the learner answers from the paper book, so it is never auto-graded.
    if (unit.skill === "listening" && !(unit.groups && unit.groups.length)) {
      text.append(el("h3", unit.title), el("p", `Section ${unit.part} · 听音资源（题目见纸质书，不作答、不判分） · 约 ${unit.minutes || 0} 分钟 · ${statusNames[source.status] || "待核验素材"}`));
      row.append(text, button("开始收听", action, "button-primary")); return row;
    }
    const count = unit.groups.reduce((sum, group) => sum + group.questions.length, 0);
    text.append(el("h3", unit.title), el("p", `${unit.skill === "listening" ? `Section ${unit.part}` : unit.part === "academic" ? "学术类" : "培训类"} · ${count} 个答题项 · 约 ${unit.minutes || 20} 分钟 · ${[...new Set(unit.groups.map(group => kindNames[group.kind]))].join(" / ")} · ${statusNames[source.status]}`));
    row.append(text, button("开始练习", action, "button-primary")); return row;
  }
  async function catalogue(token) {
    const top = shell(activeSkill === "reading" ? "选一篇，认真读完" : "选一段，听懂再复盘", "先完成作答，再对照答案和原文复盘。");
    top.append(button("返回概览", () => go(activeSkill), "button-quiet"));
    const search = el("input"); search.type = "search"; search.placeholder = "搜索题目标题或来源"; search.setAttribute("aria-label", `搜索${skillName()}题库`);
    const list = el("div", undefined, "panel objective-library"); root().append(search, list);
    const entries = await window.ELPLibrary.listUnits(activeSkill);
    if (token !== generation) return;
    if (!entries.length) return window.ELPLibrary.openImport(activeSkill);
    const render = () => {
      list.replaceChildren();
      const query = search.value.trim().toLowerCase();
      for (const item of entries.filter(item => `${item.unit.title} ${item.source.name}`.toLowerCase().includes(query))) {
        const listenOnly = item.unit.skill === "listening" && !(item.unit.groups && item.unit.groups.length);
        const open = listenOnly ? () => go(`${activeSkill}/listen/${item.packId}/${item.unit.id}`) : () => start(item.packId, item.unit.id);
        list.append(unitCard(item.unit, item.source, open));
      }
      if (!list.children.length) list.append(el("p", entries.length ? "没有匹配的题目，试试更短的关键词。" : `还没有${skillName()}题目，导入后就能在这里选题。`), button("管理题库", () => window.ELPLibrary.openImport(activeSkill), "button-quiet"));
    };
    search.addEventListener("input", render); render();
  }
  async function overview(token) {
    const top = shell(`${skillName()}练习`, "首答检验理解，复盘找到依据。重练单独记录。");
    top.append(button("＋ 新建练习", () => go(`${activeSkill}/new`), "button-primary"));
    top.append(button("整套模拟", () => go(`${activeSkill}/mock`)));
    const records = (await api("attempts")).attempts;
    const reading = [];
    for (const record of records) {
      const data = await pack(record.packId); const unit = record.examId ? data.exams?.find(exam => exam.id === record.examId) : data.units.find(unit => unit.id === record.unitId);
      if (unit?.skill === activeSkill) reading.push({ record, unit });
    }
    if (token !== generation) return;
    const plan = window.ELPStudyPlan?.today();
    const planned = el("section", undefined, "panel objective-plan"); planned.append(el("h3", "今日安排"));
    const tasks = (plan?.tasks || []).filter(task => task.kind === activeSkill || task.kind === `${activeSkill}-review`);
    if (plan?.day?.budgetNote) {
      const deferred=(plan.day.deferred||[]).filter(name=>name.includes(skillName()));
      planned.append(el("p",`今日总安排 ${plan.day.usedMinutes}/${plan.day.budgetMinutes} 分钟${deferred.length?`；本科未排入或缩短：${deferred.join("、")}`:""}。`,"muted"));
    }
    if (!tasks.length) planned.append(el("p", plan?.day ? "今天没有安排本科技能任务，可以复盘历史记录或自由练习。" : "还没有学习计划，可以自由练习或先设置目标。"), button("调整计划", () => go("plan"), "button-quiet"));
    for (const task of tasks) {
      const row = el("div", undefined, "objective-plan-task"); const label = el("label"); const check = el("input"); check.type = "checkbox"; check.checked = Boolean(plan.progress[task.id]); check.dataset.objectivePlan = task.id;
      check.addEventListener("change", async () => { check.disabled = true; try { await window.ELPStudyPlan.mark(task.id, check.checked); } catch (error) { check.checked = !check.checked; showError(error); } finally { check.disabled = false; } });
      label.append(check, el("span", `${task.title} · 约 ${task.minutes} 分钟`)); row.append(label, button("开始", () => window.ELPStudyPlan.start(task.id), "button-quiet")); planned.append(row);
    }
    root().append(planned);
    const count = el("div", undefined, "objective-summary");
    count.append(el("p", `首次完成 ${reading.filter(item => item.record.status === "submitted" && !item.record.reviewOf && item.record.mode !== "simulation").length} ${activeSkill === "reading" ? "篇" : "段"}`), el("p", `复习完成 ${reading.filter(item => item.record.status === "submitted" && item.record.reviewOf && item.record.mode !== "simulation").length} 次`), el("p", `整套模拟 ${reading.filter(item => item.record.status === "submitted" && item.record.mode === "simulation").length} 次`));
    root().append(count);
    const history = el("section", undefined, "panel objective-library"); history.append(el("h3", "练习记录"));
    const controls = el("div", undefined, "button-row"); history.append(controls);
    const list = el("div"); history.append(list); root().append(history);
    const render = filter => {
      list.replaceChildren();
      controls.querySelectorAll("button").forEach(node => node.setAttribute("aria-pressed", String(node.dataset.filter === filter)));
      const items = reading.filter(item => filter === "all" || (filter === "simulation" ? item.record.mode === "simulation" : item.record.status === filter)).sort((a, b) => b.record.updatedAt.localeCompare(a.record.updatedAt));
      for (const { record, unit } of items) {
        const row = el("article", undefined, "objective-library-row"); const text = el("div");
        text.append(el("h4", unit.title), el("p", `${record.mode === "simulation" ? "整套模拟 · " : ""}${record.reviewOf ? "复习" : "首次作答"} · ${record.status === "draft" ? "未完成" : "已完成"} · ${clock(record.elapsedSeconds)} · ${new Date(record.updatedAt).toLocaleDateString()}`));
        row.append(text, button(record.status === "draft" ? "继续作答" : "查看复盘", () => go(`${activeSkill}/${record.status === "draft" ? "session" : "report"}/${record.id}`))); list.append(row);
      }
      if (!items.length) list.append(el("p", "还没有这类练习记录。"));
    };
    for (const [value, label] of [["all", "全部"], ["draft", "未完成"], ["submitted", "已完成"], ["simulation", "整套模拟"]]) { const control = button(label, () => render(value), "button-quiet"); control.dataset.filter = value; controls.append(control); }
    render("all");
  }
  async function start(packId, unitId, reviewOf = "", reviewQuestions = [], examId = "") {
    const token = generation; const skill = activeSkill;
    if (session) await save(session);
    // A fresh ID and an immutable question reference distinguish first attempts
    // from explicit reviews; answers are never copied into a new attempt.
    const id = crypto.randomUUID();
    await api(`attempts/${id}`, { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ id, revision: 0, packId, unitId, examId, mode: examId && !reviewQuestions.length ? "simulation" : "practice", status: "draft", reviewOf, reviewQuestions, answers: {}, elapsedSeconds: 0, createdAt: "", updatedAt: "" }) });
    if (token === generation) go(`${skill}/session/${id}`);
  }
  async function startFromLibrary(packId, unitId) {
    const token = generation;
    const data = await pack(packId);
    const unit = data.units.find(item => item.id === unitId);
    if (!unit || !["reading", "listening"].includes(unit.skill)) throw new Error("这道题不属于听读练习");
    if (token !== generation) return;
    activeSkill = unit.skill;
    // Listen-only resources have no questions to answer; open the listening view, not a scored attempt.
    if (unit.skill === "listening" && !(unit.groups && unit.groups.length)) { go(`listening/listen/${packId}/${unitId}`); return; }
    await start(packId, unitId);
  }
  function dirty(s) {
    s.refreshProgress?.();
    s.version++; status("正在保存……"); clearTimeout(s.debounce);
    s.debounce = setTimeout(() => save(s).catch(showError), 450);
  }
  function save(s) {
    clearTimeout(s.debounce);
    s.queue = s.queue.catch(() => undefined).then(async () => {
      if (s.savedVersion === s.version) return;
      if (s.closed) return;
      const version = s.version; const snapshot = structuredClone(s.record);
      try {
        const saved = await api(`attempts/${snapshot.id}`, { method: "PUT", headers: { "Content-Type": "application/json", "X-ELP-Directory": s.directoryId }, body: JSON.stringify(snapshot) });
        s.record.revision = saved.revision; s.record.updatedAt = saved.updatedAt; s.savedVersion = version;
        window.dispatchEvent(new CustomEvent("elp:practice-saved"));
        if (saved.status === "submitted" && snapshot.status !== "submitted") {
          Object.assign(s.record, saved); s.savedVersion = s.version; s.closed = true; s.paused = true; s.audio?.pause();
          if (s === session && location.hash === `#${s.unit.skill}/session/${saved.id}`) go(`${s.unit.skill}/report/${saved.id}`);
        }
        if (s === session) status(s.savedVersion === s.version ? "已保存到本机" : "正在保存……");
      } catch (error) {
        if (error.status === 409 && s.record.mode === "simulation") {
          const latest = await api(`attempts/${s.record.id}`);
          if (latest.status === "submitted") { Object.assign(s.record, latest); s.closed = true; s.savedVersion = s.version; if (s === session && location.hash === `#${s.unit.skill}/session/${latest.id}`) go(`${s.unit.skill}/report/${latest.id}`); return; }
        }
        if (s === session) status("保存失败，答案仍留在本页"); throw error;
      }
    });
    return s.queue;
  }
  function paragraphs(unit, record, editable) {
    const panel = el("section", undefined, "panel objective-passage"); panel.append(el("h3", unit.title));
    for (const passage of unit.passages || []) {
      const block = el("article", undefined, "objective-paragraph"); block.id = `passage-${passage.id}`; block.tabIndex = -1;
      const prose = el("p");
      const paint = () => {
        prose.replaceChildren(); let offset = 0;
        for (const range of (record.highlights || []).filter(range => range.paragraphId === passage.id).sort((a, b) => a.start - b.start)) {
          prose.append(document.createTextNode(passage.text.slice(offset, range.start)), el("mark", passage.text.slice(range.start, range.end))); offset = range.end;
        }
        prose.append(document.createTextNode(passage.text.slice(offset)));
      };
      paint(); block.append(el("strong", passage.label || passage.id), prose);
      if (editable) {
        let selected = null;
        const highlight = button("划线选中文字", () => {
          if (!selected || session?.paused) return;
          let { start, end } = selected; const other = [];
          for (const range of record.highlights || []) {
            if (range.paragraphId === passage.id && range.start <= end && start <= range.end) { start = Math.min(start, range.start); end = Math.max(end, range.end); }
            else other.push(range);
          }
          record.highlights = [...other, { paragraphId: passage.id, start, end }]; selected = null; highlight.classList.add("hidden"); paint(); clear.classList.remove("hidden"); dirty(session);
        }, "button-quiet"); highlight.classList.add("hidden");
        const clear = button("清除本段划线", () => {
          if (session?.paused) return;
          record.highlights = (record.highlights || []).filter(range => range.paragraphId !== passage.id); paint(); clear.classList.add("hidden"); dirty(session);
        }, "button-quiet"); clear.classList.toggle("hidden", !(record.highlights || []).some(range => range.paragraphId === passage.id));
        const capture = () => {
          const selection = window.getSelection();
          if (session?.paused || !selection?.rangeCount || selection.isCollapsed) return;
          const range = selection.getRangeAt(0);
          if (!prose.contains(range.startContainer) || !prose.contains(range.endContainer)) return;
          const prefix = range.cloneRange(); prefix.selectNodeContents(prose); prefix.setEnd(range.startContainer, range.startOffset);
          selected = { start: prefix.toString().length, end: prefix.toString().length + range.toString().length }; highlight.classList.remove("hidden");
        };
        prose.addEventListener("mouseup", capture); prose.addEventListener("keyup", capture);
        block.append(highlight, clear);
        const label = el("label", "本段笔记", "field objective-note");
        const note = el("input"); note.value = record.notes?.[passage.id] || ""; note.placeholder = "关键词、同义替换或疑问";
        note.addEventListener("input", () => { record.notes ||= {}; record.notes[passage.id] = note.value; dirty(session); }); label.append(note);
        // Keep the original passage uninterrupted; saved notes remain discoverable.
        const notes = el("details", undefined, "objective-note-disclosure");
        notes.open = Boolean(note.value);
        notes.append(el("summary", note.value ? "查看笔记" : "添加笔记"), label); block.append(notes);
      } else if (record.notes?.[passage.id]) block.append(el("p", record.notes[passage.id], "objective-note"));
      panel.append(block);
    }
    return panel;
  }
  async function workspace(id, token) {
    const record = await api(`attempts/${id}`); const data = await pack(record.packId); const originalUnit = record.examId ? await api(`attempts/${id}/question`) : data.units.find(unit => unit.id === record.unitId);
    if (token !== generation) return;
    if (!originalUnit || originalUnit.skill !== activeSkill) throw new Error(`${skillName()}题目不存在`);
    const unit = scopedUnit(originalUnit, record);
    if (record.status === "submitted") return go(`${activeSkill}/report/${id}`);
    const s = { record, unit, directoryId: window.ELPLibrary.directoryId, version: 0, savedVersion: 0, queue: Promise.resolve(), paused: false, tick: Date.now() }; session = s;
    const simulation = record.mode === "simulation";
    const top = shell(unit.title, `${statusNames[data.source.status]} · ${record.reviewOf ? "重练记录" : "首次作答"} · ${simulation ? `整套模拟 ${unit.minutes} 分钟 · 计时持续进行` : unit.skill === "listening" ? `Section ${unit.part} · 可暂停回放` : `建议 ${unit.minutes || 20} 分钟`}`);
    root().classList.add("objective-active");
    const actions = el("div", undefined, "button-row objective-session-actions");
    const timer = el("strong", clock(record.elapsedSeconds), "objective-timer"); timer.id = "objectiveTimer";
    const remaining = () => Math.max(0, Math.ceil((Date.parse(record.deadlineAt) - Date.now()) / 1000));
    if (simulation) timer.textContent = clock(remaining());
    const pause = button("暂停", async () => {
      s.paused = !s.paused; pause.textContent = s.paused ? "继续" : "暂停"; s.tick = Date.now();
      if (s.audio) { if (s.paused) { s.resumeAudio = !s.audio.paused; s.audio.pause(); } else if (s.resumeAudio) await s.audio.play(); }
      root().querySelectorAll(".objective-audio").forEach(node => { node.inert = s.paused; });
      root().querySelectorAll(".objective-answer input, .objective-answer select, .objective-note input").forEach(node => { node.disabled = s.paused; });
      status(s.paused ? "已暂停，继续后可作答" : "已继续"); await save(s);
    });
    const finish = async () => {
      const navigation = generation;
      if (s.audioFailed && !(simulation && remaining() === 0)) throw new Error("音频尚未恢复，请重试加载后再完成本次练习");
      if (s.submitting) return; s.submitting = true;
      const wasPaused = s.paused; s.paused = true;
      s.audio?.pause();
      const controls = [...root().querySelectorAll("input, select, button")];
      const previousDisabled = controls.map(control => control.disabled);
      controls.forEach(control => { control.disabled = true; });
      try { await save(s); if (s.record.status !== "submitted") { s.record.status = "submitted"; s.version++; await save(s); } if (navigation === generation) go(`${s.unit.skill}/report/${id}`); }
      catch (error) { s.record.status = "draft"; s.paused = wasPaused; throw error; }
      finally { s.submitting = false; controls.forEach((control, index) => { control.disabled = previousDisabled[index]; }); }
    };
    actions.append(timer); if (!simulation) actions.append(pause);
    actions.append(button("完成作答", finish, "button-primary"), button(simulation ? "保存并离开（计时继续）" : "保存并返回", async () => { const navigation = generation; s.audio?.pause(); await save(s); if (navigation === generation) go(s.unit.skill); }, "button-quiet"));
    top.append(actions);
    const saved = el("span", "已保存到本机"); saved.id = "objectiveSave"; actions.append(saved);
    const navigation = el("div", undefined, "objective-question-nav");
    const progress = el("span", "", "objective-progress"); progress.id = "objectiveProgress";
    const links = el("div", undefined, "objective-question-links"); links.setAttribute("role", "group"); links.setAttribute("aria-label", "跳转题目");
    const questions = unit.groups.flatMap(group => group.questions);
    for (const q of questions) {
      const link = button(q.label, () => {
        const field = document.getElementById(`question-${q.id}`);
        field.scrollIntoView({ block: "nearest", behavior: "smooth" });
        field.querySelector("input,select")?.focus({ preventScroll: true });
      }, "button-quiet"); link.dataset.questionJump = q.id; links.append(link);
    }
    s.refreshProgress = () => {
      const answered = q => (record.answers[q.id] || []).some(value => String(value).trim());
      progress.textContent = `已答 ${questions.filter(answered).length} / ${questions.length}`;
      links.querySelectorAll("button").forEach((link, index) => {
        const q = questions[index], marked = (record.marked || []).includes(q.id);
        link.classList.toggle("is-answered", answered(q)); link.classList.toggle("is-marked", marked);
        link.setAttribute("aria-label", `第 ${q.label} 题，${answered(q) ? "已答" : "未答"}${marked ? "，待检查" : ""}`);
      });
    };
    s.refreshProgress(); navigation.append(progress, links); root().append(navigation);
    if(unit.images?.length)root().append(questionImages(unit));
    const columns = el("div", undefined, unit.skill === "reading" ? "objective-columns" : "objective-listening");
    if (unit.skill === "reading") columns.append(paragraphs(unit, record, true));
    else columns.append(audioPanel(unit, s));
    const answers = el("section", undefined, "panel objective-answer"); answers.append(el("h3", "题目"), el("p", unit.prompt));
    for (const group of unit.groups) {
      const section = el("section", undefined, "objective-group");
      const [heading, ...instructions] = group.instruction.split("\n"); section.append(el("h4", heading));
      if (instructions.length) section.append(el("p", instructions.join("\n"), "objective-context"));
      if (group.context) section.append(el("p", group.context, "objective-context"));
      if (group.table?.length) section.append(questionTable(group));
      for (const q of group.questions) {
        const field = el("fieldset", undefined, "objective-question"); field.id = `question-${q.id}`;
        field.addEventListener("focusin", () => links.querySelectorAll("button").forEach(link => link.classList.toggle("is-current", link.dataset.questionJump === q.id)));
        field.append(el("legend", `${q.label}. ${q.text}`));
        if (group.kind === "text") {
          const input = el("input"); input.type = "text"; input.value = record.answers[q.id]?.[0] || ""; input.autocomplete = "off"; input.spellcheck = false;
          input.setAttribute("aria-label", `第 ${q.label} 题答案`); input.dataset.question = q.id;
          input.addEventListener("input", () => { record.answers[q.id] = [input.value]; dirty(s); }); field.append(input);
        } else if (group.kind === "matching") {
          const select = el("select"); select.setAttribute("aria-label", `第 ${q.label} 题答案`); select.dataset.question = q.id;
          const empty = el("option", "选择对应项"); empty.value = ""; select.append(empty);
          for (const option of q.options?.length ? q.options : group.options) { const node = el("option", option.text); node.value = option.id; select.append(node); }
          select.value = record.answers[q.id]?.[0] || "";
          select.addEventListener("change", () => { record.answers[q.id] = select.value ? [select.value] : []; dirty(s); }); field.append(select);
        } else {
          for (const option of q.options?.length ? q.options : group.options) {
            const label = el("label", undefined, "objective-option"); const input = el("input");
            input.type = group.kind === "multiple" ? "checkbox" : "radio"; input.name = q.id; input.value = option.id; input.checked = (record.answers[q.id] || []).includes(option.id); input.dataset.question = q.id;
            input.addEventListener("change", () => {
              const chosen = [...field.querySelectorAll("input:checked")].map(node => node.value);
              if (group.kind === "multiple" && chosen.length > q.answers.length) { input.checked = false; status(`本题最多选择 ${q.answers.length} 项`); return; }
              record.answers[q.id] = chosen; dirty(s);
            }); label.append(input, el("span", option.text)); field.append(label);
          }
        }
        const mark = button((record.marked || []).includes(q.id) ? "已标记" : "标记待检查", () => {
          record.marked ||= []; record.marked = record.marked.includes(q.id) ? record.marked.filter(id => id !== q.id) : [...record.marked, q.id];
          mark.textContent = record.marked.includes(q.id) ? "已标记" : "标记待检查"; mark.setAttribute("aria-pressed", String(record.marked.includes(q.id))); dirty(s);
        }, "button-quiet"); mark.setAttribute("aria-pressed", String((record.marked || []).includes(q.id))); field.append(mark); section.append(field);
      }
      answers.append(section);
    }
    columns.append(answers); root().append(columns);
    s.interval = setInterval(() => {
      const now = Date.now(); const delta = Math.floor((now - s.tick) / 1000); if (!delta) return; s.tick += delta * 1000;
      if (s.paused || s !== session) return;
      record.elapsedSeconds = simulation ? Math.min(unit.minutes * 60, Math.max(0, Math.floor((now - Date.parse(record.startedAt)) / 1000))) : record.elapsedSeconds + delta;
      s.version++; timer.textContent = clock(simulation ? remaining() : record.elapsedSeconds);
      if (simulation && remaining() === 0) { finish().catch(showError); return; }
      if (record.elapsedSeconds % 15 < delta) save(s).catch(showError);
    }, 1000);
  }
  async function report(id, token) {
    const record = await api(`attempts/${id}`); if (record.status !== "submitted") return go(`${activeSkill}/session/${id}`);
    const [data, score] = await Promise.all([pack(record.packId), api(`attempts/${id}/result`)]);
    if (token !== generation) return;
    const unit = scopedUnit(record.examId ? await api(`attempts/${id}/question`) : data.units.find(unit => unit.id === record.unitId), record);
    if (token !== generation) return;
    if (unit.skill !== activeSkill) throw new Error(`${skillName()}记录不存在`);
    const top = shell(unit.title, `${record.reviewOf ? "重练" : "首次作答"} · ${clock(record.elapsedSeconds)} · ${statusNames[data.source.status]}`);
    root().classList.add("objective-report");
    const actions = el("div", undefined, "button-row"); actions.append(button("返回概览", () => go(activeSkill), "button-quiet"), button(record.examId ? "再次模拟" : unit.skill === "reading" ? "重练本篇" : "重练本段", () => start(record.packId, record.unitId, record.id, [], record.examId || ""), "button-primary")); top.append(actions);
    const wrong = score.items.filter(item => !item.correct).map(item => item.id);
    if (wrong.length) actions.append(button("只重练错题", () => start(record.packId, record.unitId, record.id, wrong, record.examId || "")));
    const summary = el("section", undefined, "panel objective-score"); summary.append(el("strong", `${score.points} / ${score.total}`), el("p", `答对题数 · ${record.examId ? "整套练习结果，不作为官方估分" : `单${unit.skill === "reading" ? "篇" : "段"}练习不换算雅思分数`}`)); root().append(summary);
    if(unit.images?.length)root().append(questionImages(unit));
    if (unit.skill === "listening") root().append(audioPanel(unit));
    const sourceDetails = el("details", undefined, "objective-source-disclosure");
    sourceDetails.append(el("summary", unit.skill === "reading" ? "查看阅读原文与笔记" : "查看听力原文"));
    if (unit.skill === "reading") sourceDetails.append(paragraphs(unit, record, false));
    else {
      const transcript = el("section", undefined, "panel objective-transcript"); transcript.id = "listeningReviewTranscript"; transcript.tabIndex = -1;
      transcript.append(el("h3", "听力原文"), el("p", unit.transcript, "objective-context")); sourceDetails.append(transcript);
    }
    root().append(sourceDetails);
    const review = el("section", undefined, "panel objective-review"); review.append(el("h3", "逐题复盘"), el("p", wrong.length ? "先核对这些题：找出原文依据，再用自己的话说明原因。" : "全部答对了。选一题，说说你是怎样找到依据的。", "muted"));
    const filters = el("div", undefined, "button-row objective-review-filters");
    const showItems = all => {
      review.querySelectorAll(".objective-correction").forEach(card => card.classList.toggle("hidden", !all && card.classList.contains("is-correct")));
      review.querySelectorAll(".objective-group").forEach(group => group.classList.toggle("hidden", !group.querySelector(".objective-correction:not(.hidden)")));
      filters.querySelectorAll("button").forEach(control => control.setAttribute("aria-pressed", String(control.dataset.all === String(all))));
    };
    if (wrong.length) for (const [all, label] of [[false, `需要核对 ${wrong.length} 项`], [true, `全部 ${score.items.length} 项`]]) {
      const control = button(label, () => showItems(all), "button-secondary"); control.dataset.all = String(all); filters.append(control);
    }
    review.append(filters);
    const reason = { correct: "正确", incorrect: "答案不符", unanswered: "未作答", "word-limit": "超出答案格式限制" };
    for (const group of unit.groups) {
      const groupReview=el("section",undefined,"objective-group");groupReview.append(el("p",group.instruction,"objective-context"));
      if(group.context)groupReview.append(el("p",group.context,"objective-context"));
      if(group.table?.length)groupReview.append(questionTable(group));
      for (const q of group.questions) {
      const result = score.items.find(item => item.id === q.id); const card = el("article", undefined, `objective-correction ${result.correct ? "is-correct" : "is-incorrect"}`); card.id = `review-${q.id}`; card.tabIndex = -1;
      const options = q.options?.length ? q.options : group.options || [];
      const answerText = values => values?.length ? values.map(value => options.find(option => option.id === value)?.text || value).join(" / ") : "未作答";
      card.append(el("h4", `${q.label}. ${q.text}`), el("p", `${reason[result.reason]} · ${result.points}/${result.total}`), el("p", `我的答案：${answerText(record.answers[q.id])}`), el("p", `参考答案：${answerText(q.answers)}`));
      if (q.explanation) card.append(el("p", q.explanation, "objective-context"));
      if (q.evidence) {
        card.append(button("查看原文依据", () => {
          sourceDetails.open = true;
          const source = document.getElementById(`passage-${q.evidence}`);
          if (!source) return;
          source.scrollIntoView({ behavior: "smooth", block: "center" }); source.focus({ preventScroll: true });
          root().querySelectorAll(".is-evidence").forEach(node => node.classList.remove("is-evidence")); source.classList.add("is-evidence");
          source.querySelector(".objective-return")?.remove(); const back = button(`返回第 ${q.label} 题`, () => { card.scrollIntoView({ block: "center" }); card.focus({ preventScroll: true }); }, "button-quiet"); back.classList.add("objective-return"); source.append(back);
        }, "button-quiet"));
      } else if (unit.skill === "listening") {
        card.append(el("p", "这道题没有核对过的时间定位，请结合原文回听。", "muted"), button("查看原文并回听", () => {
          sourceDetails.open = true;
          const source = document.getElementById("listeningReviewTranscript");
          source.querySelector(".objective-return")?.remove();
          const back = button(`返回第 ${q.label} 题`, () => { card.scrollIntoView({ block: "center" }); card.focus({ preventScroll: true }); }, "button-quiet"); back.classList.add("objective-return"); source.prepend(back);
          source.scrollIntoView({ block: "start", behavior: "smooth" }); source.focus({ preventScroll: true });
        }, "button-quiet"));
      } else card.append(el("p", "这道题尚无经过核对的原文定位。", "muted"));
      groupReview.append(card);
      }
      review.append(groupReview);
    }
    root().append(review);
    showItems(!wrong.length);
    const next = el("section", undefined, "panel objective-next");
    next.append(el("h3", "下一步"), el("p", "先说清一处答案的依据，再重练巩固。也可以按自己的时间安排后续练习。"));
    const nextActions = el("div", undefined, "button-row");
    nextActions.append(button(wrong.length ? "重练这些错题" : "再练一次", () => start(record.packId, record.unitId, record.id, wrong, record.examId || ""), "button-primary"), button("安排学习时间", () => go("plan")), button("返回入门指南", () => go("guide"), "button-quiet"));
    next.append(nextActions); root().append(next);
  }
  async function listenResource(packId, unitId, token) {
    // Listen-only resource view for found audio without digital questions: play the recording
    // and (after listening) reveal the machine transcript. No answer sheet, no scoring.
    const data = await pack(packId);
    const unit = data.units.find(item => item.id === unitId);
    if (token !== generation) return;
    if (!unit || unit.skill !== "listening") throw new Error("听力资源不存在");
    const top = shell(unit.title, `${statusNames[data.source.status] || "待核验素材"} · 听音资源 · 题目见纸质书，不作答、不判分`);
    top.append(button("返回概览", () => go(activeSkill), "button-quiet"));
    root().append(audioPanel(unit, null, `Section ${unit.part} · 听音`));
    const details = el("details", undefined, "objective-source-disclosure");
    details.append(el("summary", "听完后查看原文（自动转写，可能有误，以纸质书为准）"));
    const transcript = el("section", undefined, "panel objective-transcript"); transcript.tabIndex = -1;
    transcript.append(el("h3", "听力原文"), el("p", unit.transcript, "objective-context"));
    details.append(transcript); root().append(details);
  }
  window.addEventListener("elp:route", async event => {
    const token = ++generation;
    playingAudio?.pause();
    if (session) {
      const old = session;
      try { await save(old); clearInterval(old.interval); if (session === old) session = null; }
      catch (error) { go(`${old.unit.skill}/session/${old.record.id}`); showError(error); return; }
    }
    if (token !== generation) return;
    root().replaceChildren(); playingAudio = null;
    if (!["reading", "listening"].includes(event.detail)) return;
    activeSkill = event.detail;
    try {
      shell(`正在载入${skillName()}练习`, "正在读取本地题目与记录……");
      const parts = location.hash.slice(1).split("/");
      if (parts[1] === "new") await catalogue(token);
      else if (parts[1] === "mock") await examCatalogue(token);
      else if (parts[1] === "listen") await listenResource(parts[2], parts[3], token);
      else if (parts[1] === "session") await workspace(parts[2], token);
      else if (parts[1] === "report") await report(parts[2], token);
      else await overview(token);
    } catch (error) { if (token === generation) showError(error); }
  });
  window.addEventListener("elp:storage-changed", () => { generation++; playingAudio?.pause(); if (session) { clearInterval(session.interval); clearTimeout(session.debounce); } session = null; });
  window.ELPObjective = Object.freeze({ start: startFromLibrary, flush: async () => { playingAudio?.pause(); if (session) await save(session); } });
  window.addEventListener("beforeunload", event => { if (session && session.version !== session.savedVersion) { save(session).catch(() => {}); event.preventDefault(); event.returnValue = ""; } });
})();
