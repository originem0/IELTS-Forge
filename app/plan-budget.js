(function (scope) {
  "use strict";
  const costs = Object.freeze({ writing: 40, speaking: 10, reading: 20, listening: 15, writingReview: 15, writingRewrite: 20, speakingReview: 10, readingReview: 15, listeningReview: 15 });
  const names = Object.freeze({ writing: "新写作", speaking: "新口语", reading: "新阅读", listening: "新听力", writingReview: "写作精改", writingRewrite: "写作重写", speakingReview: "口语回听", readingReview: "阅读复盘", listeningReview: "听力复盘", languageMinutes: "语料记忆", reviewMinutes: "错题与单词复盘" });
  const count = (value, max) => Math.max(0, Math.min(max, Math.round(Number(value) || 0)));
  function allocate(requested, minutes, date) {
    const budget = count(minutes, 720);
    const day = { note: typeof requested.note === "string" ? requested.note : "" };
    for (const key of Object.keys(costs)) day[key] = 0;
    day.languageMinutes = day.reviewMinutes = 0;
    let remaining = budget;
    const ordinal = Math.floor(Date.parse(`${date}T00:00:00Z`) / 86400000) || 0;
    const rotate = values => values.map((_, index) => values[(index + Math.abs(ordinal) % values.length) % values.length]);
    const reviews = rotate(["writingReview", "writingRewrite", "speakingReview", "readingReview", "listeningReview"].filter(key => count(requested[key], 1)));
    for (const key of reviews) if (remaining >= costs[key]) { day[key] = 1; remaining -= costs[key]; }
    for (const [key, limit] of [["languageMinutes", 60], ["reviewMinutes", 240]]) {
      day[key] = Math.min(count(requested[key], limit), remaining); remaining -= day[key];
    }
    const newKeys = rotate(["writing", "speaking", "reading", "listening"].filter(key => count(requested[key], key === "speaking" ? 2 : 1)));
    const desiredNewMinutes = newKeys.reduce((sum, key) => sum + count(requested[key], key === "speaking" ? 2 : 1) * costs[key], 0);
    // Reserve review time before adding indivisible practice units. A 40-minute
    // writing task is never squeezed into a fictitious 15-minute slot. Short days
    // can consist entirely of review; rotation avoids starving the other skills.
    const reserve = Math.min(Math.ceil(budget * .4), Math.ceil(desiredNewMinutes * 2 / 3));
    const padding = Math.min(Math.max(0, reserve - (budget - remaining)), remaining, 240 - day.reviewMinutes);
    day.reviewMinutes += padding; remaining -= padding;
    const reviewSpent = budget - remaining;
    let newAllowance = Math.min(remaining, Math.floor(reviewSpent * 1.5));
    for (const key of newKeys) for (let index = 0; index < count(requested[key], key === "speaking" ? 2 : 1); index++) {
      if (newAllowance >= costs[key]) { day[key]++; newAllowance -= costs[key]; remaining -= costs[key]; }
    }
    const deferred = Object.keys(names).filter(key => Number(requested[key] || 0) > day[key]).map(key => names[key]);
    return { ...day, usedMinutes: budget - remaining, reviewMinutesTotal: reviewSpent, budgetMinutes: budget, deferred,
      budgetNote: deferred.length ? `今日安排 ${budget - remaining}/${budget} 分钟；未排入或缩短：${deferred.join("、")}。` : `今日安排 ${budget - remaining}/${budget} 分钟，已预留复盘时间。` };
  }
  const api = Object.freeze({ allocate, costs, names });
  if (typeof module !== "undefined" && module.exports) module.exports = api;
  else scope.ELPPlanBudget = api;
})(typeof window === "undefined" ? globalThis : window);
