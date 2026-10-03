const assert=require('node:assert/strict');
const {allocate,costs}=require('../app/plan-budget.js');
const requested={writing:1,speaking:2,reading:1,listening:1,writingReview:1,writingRewrite:1,speakingReview:1,readingReview:1,listeningReview:1,languageMinutes:15,reviewMinutes:30};
for(let budget=15;budget<=720;budget+=5)for(let day=1;day<=7;day++){
  const date=`2026-10-0${day}`;const plan=allocate(requested,budget,date);
  const sum=Object.entries(costs).reduce((total,[key,cost])=>total+plan[key]*cost,plan.languageMinutes+plan.reviewMinutes);
  assert.equal(sum,plan.usedMinutes);assert.ok(sum<=budget,`budget overrun: ${budget}`);
  assert.ok(['writing','speaking','reading','listening'].reduce((sum,key)=>sum+plan[key],0)<=1,'more than one main practice');
  for(const key of ['writing','reading','listening'])assert.ok(plan[key]<=1);
  assert.ok(plan.speaking<=2);
  assert.deepEqual(allocate(requested,budget,date),plan,'schedule not deterministic');
}
assert.equal(allocate({},180,'2026-10-01').usedMinutes,0,'rest day gained unsolicited tasks');
const short=allocate({writing:1},15,'2026-10-01');assert.equal(short.writing,0);assert.ok(short.deferred.includes('新写作'));
const original=structuredClone(requested);allocate(requested,60,'2026-10-01');assert.deepEqual(requested,original,'allocator mutated saved targets');
const seen=new Set();for(let day=1;day<=7;day++){const plan=allocate({writing:1,speaking:1,reading:1,listening:1},90,`2026-10-0${day}`);for(const key of ['writing','speaking','reading','listening'])if(plan[key])seen.add(key);}
assert.equal(seen.size,4,'rotation starved a skill');
for(const budget of [60,90,120]) {
  const skills=new Set();
  for(let day=1;day<=7;day++) {
    const plan=allocate(requested,budget,`2026-10-0${day}`);
    for(const key of ['writing','speaking','reading','listening'])if(plan[key])skills.add(key);
  }
  assert.equal(skills.size,4,`${budget}-minute week starved new learning`);
}
const empty=allocate(requested,90,'2026-10-01',{available:{}});
assert.equal(empty.reviewMinutesTotal,0,'scheduled nonexistent review material');
const task1=allocate({writing:1},30,'2026-10-01',{costs:{writing:20},available:{}});
assert.equal(task1.writing,1);assert.equal(task1.usedMinutes,20);
const weak=allocate(requested,60,'2026-10-01',{history:{writing:0,speaking:3,reading:2,listening:1}});
assert.equal(weak.mainSkill,'writing','weekly coverage did not prioritize the neglected skill');
assert.equal(allocate({writing:1},90,'2026-10-01',{doneToday:{writing:1}}).writing,0,'editing a plan demanded a second new essay on the same day');
console.log('Plan budget passed: bounded time, one main task, actual costs, weekly coverage at 60/90/120 minutes, available reviews, rest days and immutable targets.');
