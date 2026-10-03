(function (scope) {
  "use strict";
  const skills = ["writing", "speaking", "reading", "listening"];
  const intervals = [1, 3, 7, 14, 30];
  function dateOf(value) {
    if (!value) return "";
    // Calendar dates already denote a local study day; timestamps denote instants.
    if (typeof value === "string" && /^\d{4}-\d{2}-\d{2}$/.test(value)) return value;
    const date = new Date(value);
    if (!Number.isFinite(date.getTime())) return "";
    return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
  }
  function addDays(day, count) {
    const date = new Date(`${day}T12:00:00`); date.setDate(date.getDate() + count);
    return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
  }
  function due(review, day, now = Date.now()) {
    return !review?.paused && (!review?.dueDate || review.dueDate <= day) &&
      (!review?.retryAt || Date.parse(review.retryAt) <= now) &&
      !(review?.lastReviewedDate === day && review.attemptsToday >= 2);
  }
  function rate(previous = {}, rating, day, now = Date.now()) {
    if (!["again", "hard", "good"].includes(rating)) throw new Error("无效的复习评价");
    const sameDay = previous.lastReviewedDate === day;
    const attemptsToday = (sameDay ? Number(previous.attemptsToday) || 0 : 0) + 1;
    // Same-day recognition must not advance several long-term intervals.
    const level = rating === "good" ? Math.min(5, Math.max(1, (Number(previous.level) || 0) + (sameDay ? 0 : 1))) : rating === "hard" ? Math.max(0, (Number(previous.level) || 0) - 1) : 0;
    const days = rating === "good" ? intervals[level - 1] : rating === "hard" || attemptsToday >= 2 ? 1 : 0;
    return { ...previous, level, dueDate: addDays(day, days), lastReviewedDate: day, rating, performance: rating,
      attemptsToday, failures: (Number(previous.failures) || 0) + (rating === "again" ? 1 : 0),
      retryAt: days ? "" : new Date(now + 5 * 60000).toISOString(), reviewedAt: new Date(now).toISOString() };
  }
  function compare(a, b) {
    return String(a.review?.dueDate || "9999").localeCompare(String(b.review?.dueDate || "9999")) ||
      (Number(b.review?.failures) || 0) - (Number(a.review?.failures) || 0) || String(a.key).localeCompare(String(b.key));
  }
  function select(candidates, day, { minutes = 10, limit = 20, now = Date.now() } = {}) {
    let remaining = minutes;
    return candidates.filter(item => due(item.review, day, now)).sort(compare).filter(item => {
      const cost = item.minutes || 1;
      if (remaining < cost || limit <= 0) return false;
      remaining -= cost; limit--; return true;
    });
  }
  function languageSelection(entries, progress, day, { limit = 3, activeLimit = 12, now = Date.now() } = {}) {
    const candidates = entries.map(item => ({ ...item, review: progress[item.key] || {} }));
    const learned = candidates.filter(item => item.review.lastReviewedDate && !item.review.paused);
    const active = learned.filter(item => (item.review.level || 0) < 3);
    const scheduled = learned.filter(item => due(item.review, day, now)).sort(compare);
    const introducedToday = candidates.filter(item => item.review.introducedDate === day).length;
    const newCount = Math.max(0, Math.min(limit - scheduled.length, 3 - introducedToday, activeLimit - active.length));
    const fresh = candidates.filter(item => !item.review.lastReviewedDate && !item.review.paused).slice(0, newCount);
    return [...scheduled, ...fresh].slice(0, limit);
  }
  function evidence(events, day) {
    const since = addDays(day, -6);
    const recent = events.filter(event => event.date >= since && event.date <= day);
    const delayed = recent.filter(event => event.type === "recall" && event.delayed);
    const completed = recent.filter(event => event.type === "complete");
    return {
      bySkill: Object.fromEntries(skills.map(skill => [skill, completed.filter(event => event.skill === skill && event.fresh).length])),
      delayedTotal: delayed.length, delayedGood: delayed.filter(event => event.rating === "good").length,
      recurring: recent.filter(event => event.type === "recall" && event.rating === "again" && event.repeatedFailure).length,
      usedLanguage: recent.filter(event => event.type === "language-use" && event.rating === "good").length,
      overruns: completed.filter(event => event.plannedMinutes > 0 && event.minutes > event.plannedMinutes * 1.25).length,
      completed: completed.length
    };
  }
  const eventId = (type, key, date) => JSON.stringify([type, key, date]);
  const api = Object.freeze({ skills, intervals, addDays, dateOf, due, rate, select, compare, languageSelection, evidence, eventId });
  if (typeof module !== "undefined" && module.exports) module.exports = api;
  else scope.ELPStudy = api;
})(typeof window === "undefined" ? globalThis : window);
