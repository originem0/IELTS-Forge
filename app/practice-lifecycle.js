(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  if (root) root.ELPPractice = api;
})(typeof window === "object" ? window : null, () => {
  "use strict";

  function createAutosave({ write, delay = 700, schedule = setTimeout, cancel = clearTimeout }) {
    let version = 0, saved = 0, timer = null, pending = null, error = null, held = 0;
    let options = { silent: true };
    function clearTimer() { if (timer !== null) cancel(timer); timer = null; }
    function flush() {
      clearTimer();
      if (held) return Promise.resolve(false);
      if (pending) return pending;
      if (version === saved) return Promise.resolve(true);
      // Keep dirty changes through a failed write, including changes made while
      // an older snapshot is in flight. Navigation waits for the whole drain.
      pending = Promise.resolve().then(async () => {
        while (saved < version) {
          const target = version;
          try {
            if (await write(options) === false) throw new Error("练习尚未保存");
            saved = target; error = null;
          } catch (reason) { error = reason; return false; }
        }
        return true;
      }).finally(() => { pending = null; });
      return pending;
    }
    return {
      mark() { version++; clearTimer(); if (!held) timer = schedule(() => { timer = null; void flush(); }, delay); },
      hold() {
        if (pending) throw new Error("请先等待正在保存的练习");
        held++; clearTimer();
        let released = false;
        return () => {
          if (released) return; released = true; held--;
          if (!held && saved < version) timer = schedule(() => { timer = null; void flush(); }, delay);
        };
      },
      save(value = { silent: true }) { version++; options = value; return flush(); },
      flush,
      reset() {
        if (pending) throw new Error("不能清除尚在保存的练习");
        clearTimer(); version = saved = 0; error = null;
      },
      get dirty() { return version !== saved; },
      get status() { return pending ? "saving" : error ? "error" : version !== saved ? "dirty" : "saved"; }
    };
  }

  // Wall time includes suspended/background time; intervals only repaint it.
  function createClock(now = Date.now) {
    let accumulated = 0, anchor = null;
    return {
      start() { if (anchor === null) anchor = now(); },
      pause() { if (anchor !== null) { accumulated += Math.max(0, now() - anchor); anchor = null; } },
      reset(seconds = 0) { accumulated = seconds * 1000; anchor = null; },
      snapshot() { return { elapsedMilliseconds: accumulated, anchor }; },
      restore(value = {}) {
        accumulated = Number.isFinite(value.elapsedMilliseconds) && value.elapsedMilliseconds >= 0 ? value.elapsedMilliseconds : 0;
        anchor = Number.isFinite(value.anchor) && value.anchor >= 0 ? Math.min(value.anchor, now()) : null;
      },
      get seconds() { return Math.floor((accumulated + (anchor === null ? 0 : Math.max(0, now() - anchor))) / 1000); },
      get running() { return anchor !== null; }
    };
  }

  function createTaskGate() {
    let count = 0;
    return { async run(action) { count++; try { return await action(); } finally { count--; } }, get busy() { return count > 0; } };
  }
  return Object.freeze({ createAutosave, createClock, createTaskGate });
});
