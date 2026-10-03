// Runs only against the isolated Go/browser fixtures, never a user's archive.
const assert = require('node:assert/strict');
const path = require('node:path');
module.exports = async function uxAcceptance(page, artifacts) {
  const seed = {
    writings: [{id:'ux-writing',type:'Task 2',minutes:40,prompt:'Should parks be free?',essay:'Parks help everyone.',status:'completed',review:'### 总体评价\n\nGive a specific example.',reviewInput:{original:'Parks help everyone.'},updatedAt:'2026-10-01T01:00:00Z'}],
    speaking: [{id:'ux-speaking',part:'p1',prompt:'What do you do outdoors?',transcript:'I walk in the park.',review:'### 总体评价\n\nExplain why you enjoy it.',updatedAt:'2026-10-01T01:00:00Z'}],
    languageBank: {speaking:[{title:'公园散步',personalCore:'I walk in the park.',reusableTopics:['户外活动'],expressions:['get some fresh air｜呼吸新鲜空气'],answerFrames:['I enjoy it because...｜我喜欢它是因为……']}],writing:[{domain:'环境',collocations:['public green spaces｜公共绿地'],sentencePatterns:[]}],sourceKeys:[]}
  };
  await page.evaluate(async seed => {
    const current = await (await fetch('/api/data')).json();
    const response = await fetch('/api/data', {method:'PUT',headers:{'Content-Type':'application/json','X-ELP-Directory':current.storage.directoryId,'If-Match':current.revision},body:JSON.stringify({data:{...current.data,...seed,languagePractice:{}}})});
    if (!response.ok) throw new Error(await response.text());
  }, seed);
  await page.goto(`${new URL(page.url()).origin}/#language`);
  await page.reload();
  await page.locator('#languageBankDetail .language-recall-card').first().waitFor();
  const card = page.locator('#languageBankDetail .language-recall-card').first();
  assert.equal(await card.locator('.recall-answer').isVisible(), false);
  await card.locator('textarea').fill('I go out to get some fresh air.');
  await card.locator('[data-reveal]').click();
  await page.route('**/api/data', route => route.request().method() === 'PATCH' ? route.fulfill({status:503,contentType:'application/json',body:'{"error":"Synthetic recall failure"}'}) : route.continue());
  await card.locator('[data-rating="good"]').click();
  await card.locator('.recall-status').filter({hasText:'保存失败'}).waitFor();
  assert.equal(await card.locator('textarea').inputValue(), 'I go out to get some fresh air.');
  await page.unroute('**/api/data');
  await card.locator('[data-rating="good"]').click();
  await card.locator('.recall-status').filter({hasText:'已记录'}).waitFor();
  const languageState=await page.evaluate(async()=>(await(await fetch('/api/data')).json()).data.languagePractice);
  assert.ok(Object.values(languageState).some(item=>item.attempt==='I go out to get some fresh air.' && item.draftPending===false));
  await card.locator('[data-use]').click();
  await page.locator('#speakingPracticeView:not(.hidden)').waitFor();
  assert.match(await page.locator('#speakingLanguageReminder').textContent(), /fresh air/);
  await page.locator('.nav-item[data-route="speaking"]').click();
  await page.locator('[data-speaking-id="ux-speaking"]').click();
  await page.locator('#retryReviewSource').click();
  assert.equal(await page.locator('#speakingTranscript').inputValue(), '');
  await page.locator('#speakingTranscript').fill('I walk there to get some fresh air.');
  assert.equal(await page.locator('.speaking-history-sidebar').isVisible(), false);
  for (const width of [1440, 1280, 900, 390]) {
    await page.setViewportSize({width, height:900});
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), true, `speaking workspace overflows at ${width}px`);
    assert.equal(await page.locator('#recordButton').isVisible(), true);
    if (width === 1440) await page.screenshot({path:path.join(artifacts,'desktop-speaking.png'),fullPage:true});
  }
  await page.setViewportSize({width:1440,height:900});
  await page.locator('.nav-item[data-route="home"]').click();
  const saved = await page.evaluate(async () => (await (await fetch('/api/data')).json()).data);
  const child = saved.speaking.find(item => item.parentSessionId === 'ux-speaking');
  assert.ok(child && child.id !== 'ux-speaking');
  assert.equal(saved.speaking.find(item => item.id === 'ux-speaking').transcript, seed.speaking[0].transcript);
  await page.goto(`${new URL(page.url()).origin}/#review/speaking/${child.id}`);
  await page.locator('#reviewPreviousAttempt > summary').click();
  assert.match(await page.locator('#reviewAttemptComparison').textContent(), /I walk in the park/);
  assert.match(await page.locator('#reviewAttemptComparison').textContent(), /fresh air/);
  await page.locator('.nav-item[data-route="writing"]').click();
  await page.locator('[data-writing-id="ux-writing"]').click();
  await page.locator('#retryReviewSource').click();
  for (const width of [1440,1280,900,390]) {
    await page.setViewportSize({width,height:900});
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1),true,`writing session overflows at ${width}px`);
  }
  await page.setViewportSize({width:1440,height:900});
  await page.locator('#exitFocusMode').click();
  assert.equal(await page.locator('.writing-sidebar').first().isVisible(), false);
  for (const width of [1440, 1280, 900, 390]) {
    await page.setViewportSize({width,height:900});
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), true, `normal writing workspace overflows at ${width}px`);
    if (width >= 1280) {
      const layout = await page.evaluate(() => {
        const rect = selector => { const r = document.querySelector(selector).getBoundingClientRect(); return {x:r.x,y:r.y,right:r.right,bottom:r.bottom,width:r.width}; };
        return {question:rect('.writing-question-card'),answer:rect('.writing-answer-card'),finish:rect('#finishWritingSession'),height:innerHeight};
      });
      assert.ok(layout.question.width >= 350 && layout.answer.width >= 400, `writing panes too narrow at ${width}px`);
      assert.ok(Math.abs(layout.question.y - layout.answer.y) < 2, 'question and answer must stay side by side');
      assert.ok(layout.answer.bottom <= layout.height && layout.finish.bottom <= layout.height, 'writing controls and editor must fit in the window');
    }
    if (width === 1440) await page.screenshot({path:path.join(artifacts,'desktop-writing.png'),fullPage:true});
  }
  await page.setViewportSize({width:1440,height:900});
  await page.locator('.nav-item[data-route="home"]').click();
  await page.locator('[data-page="home"] [data-route="guide"]').click();
  await page.locator('#startBeginnerPractice').click();
  await page.locator('.objective-answer').waitFor();
  assert.equal(await page.locator('.objective-question').count(), 3);
  assert.equal(await page.locator('#objectiveProgress').textContent(), '已答 0 / 3');
  for (const width of [1440, 1280, 900, 390]) {
    await page.setViewportSize({width,height:900});
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), true, `reading workspace overflows at ${width}px`);
    if (width >= 1280) {
      const fits = await page.evaluate(() => ['.objective-question-nav','.objective-passage','.objective-answer'].every(selector => {
        const rect = document.querySelector(selector).getBoundingClientRect(); return rect.top >= 0 && rect.bottom <= innerHeight;
      }));
      assert.ok(fits, 'reading panes and question navigation must fit in the window');
    }
    if (width === 1440) await page.screenshot({path:path.join(artifacts,'desktop-reading.png'),fullPage:true});
  }
  await page.setViewportSize({width:1440,height:900});
  await page.getByRole('button',{name:'完成作答',exact:true}).click();
  await page.locator('.objective-score').waitFor();
  assert.equal(await page.locator('.objective-source-disclosure').getAttribute('open'), null);
  await page.getByRole('button',{name:'查看原文依据',exact:true}).first().click();
  assert.equal(await page.locator('.objective-source-disclosure').getAttribute('open'), '');
  await page.locator('.objective-return').click();
  assert.equal(await page.locator('.objective-correction:focus').count(), 1);
  await page.locator('.objective-next').getByRole('button',{name:'安排学习时间'}).click();
  await page.locator('#currentPlan:not(.hidden)').waitFor();
  assert.equal(await page.locator('#planEditor').getAttribute('open'), null);
  for (const width of [1440, 900, 390]) {
    await page.setViewportSize({width,height:900});
    for (const route of ['home','guide','language','writing','speaking','settings','plan']) {
      await page.evaluate(route => { location.hash = route; }, route);
      await page.locator(`.page.is-active[data-page="${route}"]`).waitFor();
      await page.evaluate(() => document.getAnimations().forEach(animation => animation.finish()));
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1);
      const overflowing = overflow ? await page.evaluate(() => [...document.querySelectorAll('.page.is-active *')].filter(node=>node.getBoundingClientRect().right>innerWidth+1).slice(0,12).map(node=>({tag:node.tagName,id:node.id,class:node.className,width:node.getBoundingClientRect().width}))) : [];
      assert.equal(overflow, false, `${route} overflows at ${width}px: ${JSON.stringify(overflowing)}`);
      if (width === 1440 && ['home','guide','language'].includes(route)) await page.screenshot({path:path.join(artifacts,`ux-${route}.png`),fullPage:true});
    }
  }
  await page.setViewportSize({width:1440,height:900});
  await page.evaluate(() => {location.hash='settings';});
  await page.locator('.page.is-active[data-page="settings"]').waitFor();
  assert.equal(await page.locator('#writeDataNow').isVisible(), false);
  console.log('UX acceptance passed: disk-backed recall, independent retries, previous-answer comparison, original warmup, evidence return, plan handoff and responsive pages.');
};
