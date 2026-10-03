const assert=require('node:assert/strict');
module.exports=async function testPlan(page) {
  await page.locator('.nav-item[data-route="home"]').click();
  await page.locator('[data-page="home"] [data-route="plan"]').click();
  const exam=new Date();exam.setDate(exam.getDate()+20);
  const date=`${exam.getFullYear()}-${String(exam.getMonth()+1).padStart(2,'0')}-${String(exam.getDate()).padStart(2,'0')}`;
  await page.locator('#planExamDate').fill(date);await page.locator('#planDailyMinutes').fill('360');
  await page.locator('#planCurrentLevel').fill('四科 6.0');await page.locator('#planTargetLevel').fill('四科 7.0');
  await page.locator('#saveManualPlan').click();
  await page.locator('#planSourceBadge').filter({hasText:'手动计划'}).waitFor();
  const plan=await page.evaluate(()=>window.ELPStudyPlan.today());
  const main=plan.tasks.filter(task=>['writing','speaking','reading','listening'].includes(task.kind));
  assert.equal(main.length,1,'one principal practice per day');
  assert.ok(plan.day.usedMinutes<=360);
  const task=main[0];
  await page.locator(`.nav-item[data-route="${task.skill}"]`).click();
  const checkbox=page.locator(`[data-objective-plan="${task.id}"], [data-overview-plan-check="${task.id}"]`);
  await checkbox.check();
  await page.waitForFunction(id=>window.ELPStudyPlan.today().progress[id]===true,task.id);
  await page.locator('.nav-item[data-route="home"]').click();
  assert.equal(await page.locator('#todayPlanSummary .today-module-summary').count(),4);
  await page.reload();
  await page.waitForFunction(id=>window.ELPStudyPlan?.today().progress[id]===true,task.id);
  const before=await page.evaluate(async()=>Object.keys((await(await fetch('/api/data')).json()).data.learning.days));
  await page.locator('[data-page="home"] [data-route="plan"]').click();
  await page.locator('#planEditor').evaluate(node => { node.open = true; });
  await page.locator('#planDailyMinutes').fill('15');await page.locator('#saveManualPlan').click();
  try { await page.waitForFunction(()=>window.ELPStudyPlan.today().day.budgetMinutes===15, null, {timeout:5000}); }
  catch(error) { console.log('Plan update diagnostic',await page.evaluate(()=>({toast:document.querySelector('#toast')?.textContent,storage:document.querySelector('#storageResult')?.textContent,input:document.querySelector('#planDailyMinutes').value,plan:window.ELPStudyPlan.today()})));throw error; }
  const limited=await page.evaluate(()=>window.ELPStudyPlan.today());
  assert.ok(limited.day.usedMinutes<=15);
  assert.equal(limited.day.writing,0,'40-minute writing squeezed into 15 minutes');
  assert.ok(limited.day.deferred.includes('新写作'));
  await page.locator('.nav-item[data-route="reading"]').click();
  await page.locator('.objective-plan').waitFor();
  const after=await page.evaluate(async()=>Object.keys((await(await fetch('/api/data')).json()).data.learning.days));
  for(const key of before)assert.ok(after.includes(key),'plan edit lost prior progress');
  console.log('Four-skill plan UI passed: one main task, persisted module progress, preserved plan history and short-day budgets.');
};
