(function (scope) {
  "use strict";
  function create({ getState, save, today, languageEntries, title, open, changed }) {
    const E = scope.ELPStudy;
    let cached = null, syncing = null, objectiveCache = null;
    const state = () => getState();
    const learning = () => state().learning;
    const clone = value => JSON.parse(JSON.stringify(value));
    const planKey = () => `${today()}|${state().studyPlan?.createdAt || "free"}`;
    const event = value => {
      const entry = { date: today(), ...value };
      entry.id ||= E.eventId(entry.type, entry.key || entry.sourceId, entry.date);
      if (!learning().events.some(item => item.id === entry.id)) learning().events.push(entry);
    };
    function queue() {
      const notes = state().mistakes.flatMap(item => {
        if (item.kind === "correction" && item.correction?.original && item.correction?.corrected) return [{ key: `correction:${item.id}`, kind: "correction", skill: item.module, id: item.id, title: item.correction.original, review: item.correctionReview, minutes: 2 }];
        if (item.module === "vocabulary" && item.title && item.title !== "未命名单词" && (item.text || item.images?.length)) return [{ key: `vocabulary:${item.id}`, kind: "vocabulary", skill: "vocabulary", id: item.id, title: item.title, review: item.vocabularyReview, minutes: 1 }];
        return item.reviewEnabled ? [{ key: `note:${item.id}`, kind: "note", skill: item.module, id: item.id, title: item.title, review: item.noteReview, minutes: 3 }] : [];
      });
      const language = ["writing", "speaking"].flatMap(skill => E.languageSelection(languageEntries(skill), state().languagePractice, today(), { limit: 3 }).map(item => ({ ...item, kind: "language", skill, title: item.text, minutes: 2 })));
      const objective = Object.values(objectiveCache?.target === learning() ? objectiveCache.items : learning().objectiveItems).filter(item => !item.retired).map(item => ({ ...item, kind: "objective", review: item.review, minutes: 3 }));
      return [...notes, ...language, ...objective];
    }
    function sourceFor(kind) {
      const skill = kind.startsWith("writing") ? "writing" : "speaking";
      const records = skill === "writing" ? state().writings : state().speaking;
      return records.filter(item => (item.status === "completed" || item.review) && (skill === "writing" ? item.essay?.trim() : item.transcript?.trim() || item.audio) &&
        (!kind.endsWith("rewrite") || item.review) && E.due(learning().recordReviews[`${kind}:${item.id}`], today()))
        .map(item => ({ item, key: `${kind}:${item.id}`, review: learning().recordReviews[`${kind}:${item.id}`] }))
        .sort(E.compare)[0]?.item;
    }
    function summary() { return E.evidence(learning().events, today()); }
    function context() {
      const items = queue().filter(item => E.due(item.review, today()));
      const writing = state().writings.filter(item => item.status === "completed" || item.review);
      const track = state().studyPlan?.profile?.writingTrack || (writing.some(item => item.type === "Task 1 General") ? "General" : "Academic");
      const types = ["Task 2", `Task 1 ${track}`];
      const latest = type => writing.filter(item => item.type === type).map(item => item.completedAt || item.createdAt || item.updatedAt || "").sort().at(-1) || "";
      types.sort((a, b) => latest(a).localeCompare(latest(b)));
      let writingType = types[0];
      if ((state().studyPlan?.profile?.dailyMinutes || 180) < 40) writingType = `Task 1 ${track}`;
      const parts = ["p1", "p2", "p3"];
      parts.sort((a, b) => state().speaking.filter(item => item.part === a && (item.audio || item.transcript)).length - state().speaking.filter(item => item.part === b && (item.audio || item.transcript)).length);
      const recent = learning().events.filter(item => item.date >= E.addDays(today(), -6));
      const history = summary().bySkill;
      const costs = { ...scope.ELPPlanBudget.costs, writing: writingType === "Task 2" ? 40 : 20 };
      costs.writingRewrite = Number(sourceFor("writing-rewrite")?.minutes) || 20;
      for (const skill of ["reading", "listening"]) costs[`${skill}Review`] = Math.min(15, items.filter(item => item.kind === "objective" && item.skill === skill).length * 3) || 15;
      for (const skill of E.skills) {
        const failures = recent.filter(item => item.type === "recall" && item.skill === skill && item.rating === "again").length;
        history[skill] -= Math.min(.5, failures * .1);
        const times = recent.filter(item => item.type === "complete" && item.fresh && item.skill === skill && item.minutes > 0 && (skill !== "writing" || item.taskType === writingType)).map(item => item.minutes).slice(-5).sort((a,b) => a-b);
        if (times.length >= 2) costs[skill] = Math.max(costs[skill], Math.min(costs[skill] * 2, Math.ceil(times[Math.floor(times.length / 2)])));
      }
      const doneToday = Object.fromEntries(E.skills.map(skill => [skill, recent.filter(item => item.type === "complete" && item.fresh && item.skill === skill && item.date === today()).length]));
      return { writingType, speakingPart: parts[0], history, costs, doneToday,
        available: { writingReview: Number(Boolean(sourceFor("writing-review"))), writingRewrite: Number(Boolean(sourceFor("writing-rewrite"))), speakingReview: Number(Boolean(sourceFor("speaking-review"))),
          readingReview: Number(items.some(item => item.kind === "objective" && item.skill === "reading")), listeningReview: Number(items.some(item => item.kind === "objective" && item.skill === "listening")),
          reviewMinutes: items.filter(item => item.kind !== "language").reduce((sum, item) => sum + item.minutes, 0),
          languageMinutes: items.filter(item => item.kind === "language").reduce((sum, item) => sum + item.minutes, 0) } };
    }
    function tasks(day) {
      if (!day) return [];
      const key = planKey(), stored = learning().days[key];
      if (stored) return stored.tasks;
      if (cached?.key === key) return cached.tasks;
      const ctx = context(), rows = [];
      const names = { writing: "写作", speaking: "口语", reading: "阅读", listening: "听力" };
      for (const skill of E.skills) if (day[skill]) rows.push({ id: `${skill}-0`, kind: skill, skill, title: `${names[skill]}主要练习`, detail: skill === "writing" ? ctx.writingType : skill === "speaking" ? `Part ${ctx.speakingPart.slice(1)}` : "完成一组新题并保留首答", minutes: day.costs?.[skill] || scope.ELPPlanBudget.costs[skill], writingType: ctx.writingType, speakingPart: ctx.speakingPart });
      for (const [field, kind, label] of [["writingReview", "writing-review", "核对写作反馈"], ["writingRewrite", "writing-rewrite", "重写与对照"], ["speakingReview", "speaking-review", "回听与重说"]]) {
        if (!day[field]) continue;
        const source = sourceFor(kind); if (!source) continue;
        rows.push({ id: `${kind}-${source.id}`, kind, skill: kind.split("-")[0], sourceId: source.id, title: label, detail: title(source), minutes: day.costs?.[field] || scope.ELPPlanBudget.costs[field] });
      }
      const due = queue();
      const objective = ["reading", "listening"].flatMap(skill => day[`${skill}Review`] ? E.select(due.filter(item => item.kind === "objective" && item.skill === skill), today(), { minutes: day.costs[`${skill}Review`] }) : []);
      const keys = new Set(objective.map(item => item.key));
      const selected = [...objective, ...E.select(due.filter(item => item.kind !== "language" && !keys.has(item.key)), today(), { minutes: day.reviewMinutes }), ...E.select(due.filter(item => item.kind === "language"), today(), { minutes: day.languageMinutes, limit: 6 })];
      for (const item of selected) rows.push({ id: `recall:${item.key}`, kind: "recall", skill: item.skill, material: { key: item.key, kind: item.kind, skill: item.skill, id: item.id, recordId: item.recordId, questionId: item.questionId }, title: ({ correction: "改正原句", vocabulary: "回忆单词", note: "回顾笔记", language: "回忆表达", objective: "重练错题" })[item.kind], detail: item.title, minutes: item.minutes });
      // Keep the concrete material stable during a day; rating one item must not
      // silently replace a completed task with another item under the same id.
      cached = { key, tasks: rows, day: { ...day, usedMinutes: rows.reduce((sum, row) => sum + row.minutes, 0) } };
      return rows;
    }
    function snapshot(day) {
      const key = planKey();
      if (!learning().days[key]) learning().days[key] = { date: today(), planCreatedAt: state().studyPlan?.createdAt || "", tasks: clone(tasks(day)), progress: {}, started: {}, day: clone(cached?.day || day || {}) };
      return learning().days[key];
    }
    function progress() { return { ...(learning().days[planKey()]?.progress || {}) }; }
    async function start(task, day) {
      const saved = snapshot(day);
      const previous = saved.started[task.id];
      saved.started[task.id] ||= new Date().toISOString();
      try { await save(); } catch (error) { saved.started[task.id] = previous; throw error; }
      await open(task);
    }
    async function mark(id, done, day) {
      const saved = snapshot(day), task = saved.tasks.find(item => item.id === id);
      if (!task) throw new Error("今天没有这项任务");
      const previous = saved.progress[id]; saved.progress[id] = done;
      const reviewKey = `${task.kind}:${task.sourceId}`, previousReview = learning().recordReviews[reviewKey];
      const manualKey = E.eventId("manual", `${planKey()}:${id}`, today()), hadEvent = learning().events.some(item => item.id === manualKey);
      if (done && task.sourceId && task.kind.endsWith("review")) learning().recordReviews[reviewKey] = E.rate(previousReview, "good", today());
      if (done) event({ type: "manual", key: `${planKey()}:${id}`, skill: task.skill });
      try { await save(done); } catch (error) {
        saved.progress[id] = previous;
        if (previousReview) learning().recordReviews[reviewKey] = previousReview; else delete learning().recordReviews[reviewKey];
        if (!hadEvent) learning().events = learning().events.filter(item => item.id !== manualKey);
        throw error;
      }
      changed();
    }
    function recalled(item, previous, rating) {
      event({ id: `${E.eventId("recall", item.key, today())}:${Date.now()}`, type: "recall", key: item.key, skill: item.skill, rating,
        delayed: Boolean(previous?.lastReviewedDate && previous.lastReviewedDate < today()), repeatedFailure: Number(previous?.failures) > 0 });
      for (const saved of Object.values(learning().days).filter(item => item.date === today())) for (const task of saved.tasks) if (task.material?.key === item.key) saved.progress[task.id] = true;
    }
    function completed(skill, record, minutes = 0) {
      const fresh = !record.parentSessionId && !record.reviewOf;
      const identity = `complete:${skill}:${record.id}`;
      if (learning().events.some(item => item.id === identity)) return;
      const kind = fresh ? skill : skill === "writing" ? "writing-rewrite" : `${skill}-review`;
      let plannedMinutes = 0;
      for (const saved of Object.values(learning().days).filter(item => item.date === today())) {
        const task = saved.tasks.find(item => item.kind === kind && !saved.progress[item.id] && (!fresh || skill !== "writing" || !item.writingType || item.writingType === record.type) && (!fresh || skill !== "speaking" || !item.speakingPart || item.speakingPart === record.part) && (!item.sourceId || item.sourceId === record.parentSessionId || item.sourceId === record.reviewOf));
        if (task) { saved.progress[task.id] = true; plannedMinutes = task.minutes; }
      }
      if (!fresh && ["writing", "speaking"].includes(skill)) learning().recordReviews[`${kind}:${record.parentSessionId}`] = E.rate(learning().recordReviews[`${kind}:${record.parentSessionId}`], "good", today());
      event({ id: identity, type: "complete", skill, taskType: record.type || record.part || "", sourceId: record.id, fresh, minutes, plannedMinutes });
    }
    function commitPending() {
      if (!objectiveCache || objectiveCache.target !== learning() || !objectiveCache.dirty) return;
      learning().objectiveItems = clone(objectiveCache.items); learning().observed = { ...objectiveCache.observed };
      for (const entry of objectiveCache.recalls) recalled(entry.item, entry.previous, entry.rating);
      for (const entry of objectiveCache.completions) completed(entry.skill, entry.record, entry.record.elapsedSeconds / 60);
      objectiveCache.recalls = []; objectiveCache.completions = []; objectiveCache.dirty = false;
    }
    async function syncObjective(records, { persist = false } = {}) {
      if (syncing) return syncing.then(() => syncObjective(records, { persist }));
      const directory = scope.ELPLibrary.directoryId, target = learning();
      const submitted = records.filter(item => item.status === "submitted");
      const live = new Set(submitted.map(item => item.id));
      syncing = (async () => {
        const buffer = objectiveCache?.target === target ? objectiveCache : { target, items: clone(target.objectiveItems), observed: { ...target.observed }, recalls: [], completions: [], dirty: false };
        const fingerprint = item => JSON.stringify([item.answers, item.status, item.reviewOf || ""]);
        for (const item of Object.values(buffer.items)) if (!live.has(item.recordId) && !item.retired) { item.retired = true; buffer.dirty = true; }
        const pending = submitted.filter(item => buffer.observed[item.id] !== fingerprint(item)).sort((a, b) => String(a.updatedAt).localeCompare(String(b.updatedAt)));
        for (const record of pending) {
          const [pack, score] = await Promise.all([scope.ELPLibrary.loadPack(record.packId), scope.ELPLibrary.request(`attempts/${record.id}/result`)]);
          if (scope.ELPLibrary.directoryId !== directory || learning() !== target) return;
          const unit = record.examId ? pack.exams?.find(item => item.id === record.examId) : pack.units.find(item => item.id === record.unitId);
          if (!unit) continue;
          for (const answer of score.items || []) {
            const examQuestion = record.examId && /^s(\d+)-(.+)$/.exec(answer.id);
            const sourceUnitId = examQuestion ? unit.unitIds[Number(examQuestion[1]) - 1] : record.unitId;
            const sourceQuestionId = examQuestion ? examQuestion[2] : answer.id;
            const sourceUnit = pack.units.find(item => item.id === sourceUnitId);
            const question = sourceUnit?.groups?.flatMap(group => group.questions).find(item => item.id === sourceQuestionId);
            const key = `objective:${record.packId}:${sourceUnitId}:${sourceQuestionId}`, previous = buffer.items[key];
            if (answer.correct && !previous) continue;
            const date = E.dateOf(record.updatedAt) || today();
            const review = E.rate(previous?.review, answer.correct ? "good" : "again", date, Date.parse(record.updatedAt) || Date.now());
            // The retry API accepts only questions that were wrong in its
            // source. Keep that immutable source after a successful retry.
            const reference = answer.correct ? previous : { recordId: record.id, packId: record.packId, unitId: record.unitId, examId: record.examId, questionId: answer.id };
            buffer.items[key] = { ...reference, key, skill: unit.skill, title: `${sourceUnit?.title || unit.title} · 第 ${question?.label || sourceQuestionId} 题`, review, retired: answer.correct ? Boolean(previous.retired) : false };
            if (previous && E.dateOf(record.updatedAt) === today()) buffer.recalls.push({ item: buffer.items[key], previous: previous.review, rating: answer.correct ? "good" : "again" });
          }
          if (E.dateOf(record.updatedAt) === today()) buffer.completions.push({ skill: unit.skill, record });
          buffer.observed[record.id] = fingerprint(record); buffer.dirty = true;
        }
        // Hydration is read-only. A previous tab must never write a background
        // migration after a new tab has loaded its revision. Commit derived
        // markers with the next explicit save, or the awaited submission flow.
        objectiveCache = buffer;
        if (persist) { commitPending(); await save(); }
        cached = null; changed();
      })().finally(() => { syncing = null; });
      return syncing;
    }
    return { queue, context, tasks, snapshot, progress, start, mark, recalled, completed, summary, event, syncObjective, commitPending,
      reset: ({ directory = false } = {}) => { cached = null; if (directory) objectiveCache = null; },
      day: fallback => learning().days[planKey()]?.day || (cached?.key === planKey() ? cached.day : null) || fallback,
      find: key => queue().find(item => item.key === key) };
  }
  scope.ELPStudyCoordinator = Object.freeze({ create });
})(window);
