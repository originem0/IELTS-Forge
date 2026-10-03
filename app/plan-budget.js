(function (scope) {
  "use strict";
  const costs = Object.freeze({ writing: 40, speaking: 10, reading: 20, listening: 15, writingReview: 15, writingRewrite: 20, speakingReview: 10, readingReview: 15, listeningReview: 15 });
  const names = Object.freeze({ writing: "新写作", speaking: "新口语", reading: "新阅读", listening: "新听力", writingReview: "写作精改", writingRewrite: "写作重写", speakingReview: "口语回听", readingReview: "阅读复盘", listeningReview: "听力复盘", languageMinutes: "语料记忆", reviewMinutes: "错题与单词复盘" });
  const count = (value, max) => Math.max(0, Math.min(max, Math.round(Number(value) || 0)));
  function allocate(requested, minutes, date, context = {}) {
    const budget = count(minutes, 720);
    const day = { note: typeof requested.note === "string" ? requested.note : "" };
    for (const key of Object.keys(costs)) day[key] = 0;
    day.languageMinutes = day.reviewMinutes = 0;
    let remaining = budget;
    const unitCosts = { ...costs, ...context.costs };
    const ordinal = Math.floor(Date.parse(`${date}T12:00:00Z`) / 86400000) || 0;
    const rotation = ["writing", "speaking", "reading", "listening"];
    const enabled = rotation.filter(key => count(requested[key], key === "speaking" ? 2 : 1) > (context.doneToday?.[key] || 0));
    enabled.sort((a, b) => (context.history?.[a] || 0) - (context.history?.[b] || 0) ||
      ((rotation.indexOf(a) - ordinal % 4 + 4) % 4) - ((rotation.indexOf(b) - ordinal % 4 + 4) % 4));
    // Reserve one real practice before allocating existing review work. Review
    // percentages must not manufacture a backlog or starve new learning.
    const main = enabled.find(key => unitCosts[key] <= remaining);
    if (main) { day[main] = 1; remaining -= unitCosts[main]; }
    const available = key => context.available ? context.available[key] || 0 : requested[key] || 0;
    day.reviewMinutes = Math.min(count(requested.reviewMinutes, 120), available("reviewMinutes"), remaining, Math.max(5, Math.floor(budget * .2)));
    remaining -= day.reviewMinutes;
    const associated = { writing: ["writingReview", "writingRewrite"], speaking: ["speakingReview"], reading: ["readingReview"], listening: ["listeningReview"] };
    const reviewKeys = [...(associated[main] || []), "writingReview", "writingRewrite", "speakingReview", "readingReview", "listeningReview"];
    for (const key of new Set(reviewKeys)) {
      if (count(requested[key], 1) && available(key) && remaining >= unitCosts[key]) { day[key] = 1; remaining -= unitCosts[key]; }
    }
    day.languageMinutes = Math.min(count(requested.languageMinutes, 60), available("languageMinutes"), remaining, 10);
    remaining -= day.languageMinutes;
    const reviewSpent = budget - remaining - (main ? unitCosts[main] : 0);
    const deferred = Object.keys(names).filter(key => Number(requested[key] || 0) > day[key]).map(key => names[key]);
    return { ...day, mainSkill: main || "", costs: unitCosts, usedMinutes: budget - remaining, reviewMinutesTotal: reviewSpent, budgetMinutes: budget, deferred,
      budgetNote: `今日安排 ${budget - remaining}/${budget} 分钟。${main ? "围绕一个主要练习安排，其余科目按周轮换。" : "今天以到期复习为主。"}${remaining ? `剩余 ${remaining} 分钟可休息或自由学习。` : ""}` };
  }
  const api = Object.freeze({ allocate, costs, names });
  if (typeof module !== "undefined" && module.exports) module.exports = api;
  else scope.ELPPlanBudget = api;
})(typeof window === "undefined" ? globalThis : window);
