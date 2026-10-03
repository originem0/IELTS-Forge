const assert=require('node:assert/strict');
const E=require('../app/study-engine.js');
const {execFileSync}=require('node:child_process');
for (const [zone, stamp, expected] of [
 ['Asia/Shanghai','2026-10-02T16:30:00Z','2026-10-03'],
 ['America/Los_Angeles','2026-10-03T00:30:00Z','2026-10-02'],
 ['America/New_York','2026-11-01T06:30:00Z','2026-11-01']
]) {
 const result=execFileSync(process.execPath,['-e',`process.stdout.write(require('./app/study-engine.js').dateOf(${JSON.stringify(stamp)}))`],{env:{...process.env,TZ:zone},encoding:'utf8'});
 assert.equal(result,expected,`wrong study day in ${zone}`);
}
assert.equal(E.dateOf('2026-10-03'),'2026-10-03');
assert.equal(E.dateOf('invalid'),'');
assert.equal(E.dateOf(null),'');
const day='2026-10-02',now=Date.parse(`${day}T12:00:00`);
let review=E.rate({},'again',day,now);
assert.equal(E.due(review,day,now),false,'failure immediately repeated');
assert.equal(E.due(review,day,now+300001),true);
review=E.rate(review,'again',day,now+300001);
assert.equal(review.dueDate,'2026-10-03');assert.equal(E.due(review,day,now+600002),false);
let good=E.rate({},'good',day,now);
assert.equal(good.dueDate,'2026-10-03');good=E.rate(good,'good',day,now+60000);
assert.equal(good.level,1,'same-day practice advanced memory interval');
good=E.rate(good,'good','2026-10-03',now+86400000);assert.equal(good.dueDate,'2026-10-06');
assert.equal(E.due({...good,paused:true},'2027-01-01'),false);
assert.equal(E.addDays('2026-12-31',1),'2027-01-01');
const entries=Array.from({length:90},(_,i)=>({key:`k${i}`,text:`expression ${i}`}));
const progress={k89:{lastReviewedDate:'2026-10-01',dueDate:day,level:0,failures:1}};
assert.equal(E.languageSelection(entries,progress,day,{now})[0].key,'k89','failed expression lost to calendar rotation');
const active=Object.fromEntries(entries.slice(0,12).map(item=>[item.key,{lastReviewedDate:'2026-10-01',level:1,dueDate:'2026-10-06'}]));
assert.equal(E.languageSelection(entries,active,day,{now}).length,0,'active-set cap exceeded');
active.k0.level=3;assert.equal(E.languageSelection(entries,active,day,{now}).length,1,'mastered items never freed an active slot');
const selected=E.select([{key:'a',minutes:3,review:{}},{key:'b',minutes:3,review:{}},{key:'c',minutes:3,review:{}}],day,{minutes:5,now});
assert.equal(selected.length,1,'round exceeded time budget');
const events=[{type:'complete',date:day,skill:'writing',fresh:true,minutes:50,plannedMinutes:40},{type:'recall',date:day,delayed:true,rating:'good'},{type:'recall',date:day,delayed:false,rating:'good'},{type:'recall',date:day,delayed:true,rating:'again',repeatedFailure:true}];
const evidence=E.evidence(events,day);assert.equal(evidence.delayedTotal,2);assert.equal(evidence.delayedGood,1);assert.equal(evidence.recurring,1);assert.equal(evidence.bySkill.writing,1);
console.log('Study engine passed: cooldown, attempt/time limits, delayed recall, same-day caps, priority, active archive and weekly evidence.');
