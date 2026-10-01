const assert=require('node:assert/strict');
const {allocate,costs}=require('../app/plan-budget.js');
const requested={writing:1,speaking:2,reading:1,listening:1,writingReview:1,writingRewrite:1,speakingReview:1,readingReview:1,listeningReview:1,languageMinutes:15,reviewMinutes:30};
for(let budget=15;budget<=720;budget+=5)for(let day=1;day<=7;day++){
  const date=`2026-10-0${day}`;const plan=allocate(requested,budget,date);
  const sum=Object.entries(costs).reduce((total,[key,cost])=>total+plan[key]*cost,plan.languageMinutes+plan.reviewMinutes);
  assert.equal(sum,plan.usedMinutes);assert.ok(sum<=budget,`budget overrun: ${budget}`);
  assert.ok(plan.reviewMinutesTotal>=sum*.4,`review underallocated: ${budget}`);
  for(const key of ['writing','reading','listening'])assert.ok(plan[key]<=1);
  assert.ok(plan.speaking<=2);
  assert.deepEqual(allocate(requested,budget,date),plan,'schedule not deterministic');
}
assert.equal(allocate({},180,'2026-10-01').usedMinutes,0,'rest day gained unsolicited tasks');
const short=allocate({writing:1},15,'2026-10-01');assert.equal(short.writing,0);assert.ok(short.deferred.includes('新写作'));
const original=structuredClone(requested);allocate(requested,60,'2026-10-01');assert.deepEqual(requested,original,'allocator mutated saved targets');
const seen=new Set();for(let day=1;day<=7;day++){const plan=allocate({writing:1,speaking:1,reading:1,listening:1},90,`2026-10-0${day}`);for(const key of ['writing','speaking','reading','listening'])if(plan[key])seen.add(key);}
assert.equal(seen.size,4,'rotation starved a skill');
console.log('Four-skill budget tests passed: all 15–720 minute budgets, review share, volume caps, rotation, rest days and immutable targets.');
