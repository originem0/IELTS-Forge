const assert = require('node:assert/strict');
const { createAutosave, createClock, createTaskGate } = require('../app/practice-lifecycle.js');
const { validateBackup } = require('../app/state-schema.js');

(async () => {
  let release, writes = 0;
  const save = createAutosave({ write: async () => { writes++; if (writes === 1) await new Promise(resolve => { release = resolve; }); return true; } });
  const pending = save.save();
  await new Promise(setImmediate);
  save.mark();
  assert.equal(save.flush(), pending, 'flush must join an active write');
  assert.equal(save.status, 'saving');
  release();
  assert.equal(await pending, true);
  assert.equal(writes, 2, 'edits during a write need another snapshot');
  assert.equal(save.dirty, false);
  let fail = true;
  const retry = createAutosave({write: async () => !fail});
  assert.equal(await retry.save(), false);
  assert.equal(retry.status, 'error');
  assert.equal(retry.dirty, true);
  fail = false;
  assert.equal(await retry.flush(), true);
  assert.equal(retry.status, 'saved');
  const resume = retry.hold();
  retry.mark();
  assert.equal(await retry.flush(),false,'held autosave wrote during a deletion');
  assert.equal(retry.dirty,true);
  resume(); resume();
  assert.equal(await retry.flush(),true,'failed deletion could not resume autosave');
  let now = 1000;
  const clock = createClock(() => now);
  clock.start(); now += 123000;
  assert.equal(clock.seconds, 123, 'time must advance without interval callbacks');
  clock.pause(); now += 500000;
  assert.equal(clock.seconds, 123);
  clock.start(); now += 2000;
  assert.equal(clock.seconds, 125);
  const savedClock = clock.snapshot();
  now += 60000;
  const resumed = createClock(() => now);
  resumed.restore(savedClock);
  assert.equal(resumed.seconds,185,'running clock lost time while closed');
  resumed.pause();
  const pausedClock = resumed.snapshot();
  now += 3600000;
  resumed.restore(pausedClock);
  assert.equal(resumed.seconds,185,'paused clock counted time while closed');
  const gate = createTaskGate();
  await assert.rejects(gate.run(async () => { assert.equal(gate.busy, true); throw new Error('fixture'); }));
  assert.equal(gate.busy, false);
  const legacy = {version:1,data:{writings:[{id:'w',essay:'safe'}],speaking:[],opaque:{keep:1}}};
  assert.deepEqual(validateBackup(legacy).opaque, {keep:1});
  for (const invalid of [
    {version:2,data:legacy.data}, {writings:[null],speaking:[]},
    {writings:[{id:'w',essay:{bad:true}}],speaking:[]},
    {writings:[{id:'same'},{id:'same'}],speaking:[]},
    {writings:[],speaking:[{id:'s',audio:12}]}
  ]) assert.throws(() => validateBackup(invalid));
  console.log('Practice lifecycle passed: in-flight drain, dirty retry, elapsed clocks, task gate and backup validation.');
})().catch(error => { console.error(error); process.exitCode = 1; });
