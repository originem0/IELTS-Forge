(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  if (root) root.ELPStateSchema = api;
})(typeof window === "object" ? window : null, () => {
  "use strict";
  const object = value => value !== null && typeof value === "object" && !Array.isArray(value);
  function requireType(valid, path) { if (!valid) throw new Error(`备份字段无效：${path}`); }
  function optional(value, key, check, path) { if (value[key] !== undefined && value[key] !== null) requireType(check(value[key]), `${path}.${key}`); }
  const text = value => typeof value === "string";
  const number = value => typeof value === "number" && Number.isFinite(value);
  const strings = value => Array.isArray(value) && value.every(text);
  function record(value, path) {
    requireType(object(value), path);
    requireType(text(value.id) && value.id.trim().length > 0, `${path}.id`);
    for (const key of ["type", "part", "prompt", "essay", "transcript", "audio", "review", "topicTitle", "status", "updatedAt", "createdAt", "reviewedAt", "original", "correction", "explanation", "module", "category", "text", "translation", "example"]) optional(value, key, text, path);
    for (const key of ["minutes", "duration", "attemptNumber"]) optional(value, key, number, path);
    for (const key of ["images", "promptImages"]) optional(value, key, strings, path);
    for (const key of ["reviewInput", "questionRef", "chartExtraction"]) optional(value, key, object, path);
    if (value.chartExtraction) for (const key of ["text", "sourceKey", "confirmedAt", "model"]) optional(value.chartExtraction, key, text, `${path}.chartExtraction`);
    if (value.reviewInput) {
      for (const key of ["original", "prompt", "type", "part", "chartText", "chartConfirmedAt"]) optional(value.reviewInput, key, text, `${path}.reviewInput`);
      optional(value.reviewInput, "promptImages", strings, `${path}.reviewInput`);
    }
  }
  function validateBackup(parsed) {
    requireType(object(parsed), "根对象");
    if (Object.hasOwn(parsed, "data")) requireType(parsed.version === 1, "version（仅接受版本 1 的 JSON 备份）");
    const data = Object.hasOwn(parsed, "data") ? parsed.data : parsed;
    requireType(object(data), "data");
    for (const name of ["writings", "speaking", "mistakes"]) {
      if (name === "mistakes" && data[name] === undefined) continue;
      requireType(Array.isArray(data[name]), name);
      const ids = new Set();
      data[name].forEach((value, index) => { record(value, `${name}[${index}]`); requireType(!ids.has(value.id), `${name}.id 重复`); ids.add(value.id); });
    }
    optional(data, "activityDates", strings, "data");
    for (const name of ["studyPlan", "languageBank", "preferences", "planProgress"]) optional(data, name, object, "data");
    if (data.languageBank) for (const name of ["writing", "speaking"]) optional(data.languageBank, name, value => Array.isArray(value) && value.every(object), "languageBank");
    if (data.planProgress) for (const [day, tasks] of Object.entries(data.planProgress)) requireType(object(tasks) && Object.values(tasks).every(value => typeof value === "boolean"), `planProgress.${day}`);
    if (data.studyPlan) {
      optional(data.studyPlan, "profile", object, "studyPlan");
      optional(data.studyPlan, "priorities", strings, "studyPlan");
      optional(data.studyPlan, "summary", text, "studyPlan");
      const days = (values, path) => {
        requireType(Array.isArray(values), path);
        values.forEach((day, index) => {
          requireType(object(day), `${path}[${index}]`);
          for (const key of ["writing","speaking","reading","listening","writingReview","writingRewrite","speakingReview","readingReview","listeningReview","languageMinutes","reviewMinutes"]) optional(day, key, value => number(value) && value >= 0, `${path}[${index}]`);
          optional(day, "note", text, `${path}[${index}]`);
        });
      };
      if (data.studyPlan.days) days(data.studyPlan.days, "studyPlan.days");
      if (data.studyPlan.phases !== undefined) {
        requireType(Array.isArray(data.studyPlan.phases), "studyPlan.phases");
        data.studyPlan.phases.forEach((phase, index) => {
          requireType(object(phase), `studyPlan.phases[${index}]`);
          for (const key of ["name","focus","startDate","endDate"]) optional(phase, key, text, `studyPlan.phases[${index}]`);
          if (phase.days !== undefined) days(phase.days, `studyPlan.phases[${index}].days`);
        });
      }
    }
    // Unknown legacy fields survive validation and round trips untouched.
    return data;
  }
  return Object.freeze({ validateBackup });
});
