// Real browser + real Go API. Invoked by TestLibraryBrowser with isolated storage.
const { chromium } = require(process.env.ELP_PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
const path = require('node:path');
const fs = require('node:fs/promises');
const base = process.env.ELP_TEST_URL;
if (!base) throw new Error('Run through ELP_BROWSER_TEST=1 go test -run TestLibraryBrowser');

const pack = {
  version: 1, title: 'Reading foundation <img src=x onerror=alert(1)>',
  source: { name: 'Synthetic browser fixture', status: 'generated' },
  units: [{ id: 'reading-1', skill: 'reading', part: 'academic', title: 'The community library',
    prompt: 'Read and answer the question.', minutes: 20,
    passages: [{ id: 'A', text: 'The community library opens on Monday.' }],
    groups: [{ id: 'g1', kind: 'text', instruction: 'ONE WORD ONLY', maxWords: 1, table:[['Booking','Day'],['Library','[1] ____']],
      questions: [{ id: 'q1', label: '1', text: 'The library opens on ____.', answers: ['Monday'], evidence: 'A' }] },
      { id: 'g2', kind: 'single', instruction: 'Choose TRUE or FALSE.', options: [{id:'TRUE',text:'TRUE'},{id:'FALSE',text:'FALSE'}], questions:[{id:'q2',label:'2',text:'The library opens on Monday.',answers:['TRUE']}] },
      { id: 'g3', kind: 'matching', instruction: 'Match the opening day.', options: [{id:'A',text:'Tuesday'},{id:'B',text:'Monday'}], questions:[{id:'q3',label:'3',text:'Opening day',answers:['B']}] },
      { id: 'g4', kind: 'multiple', instruction: 'Choose TWO weekdays.', options: [{id:'A',text:'Monday'},{id:'B',text:'Sunday'},{id:'C',text:'Tuesday'}], questions:[{id:'q4',label:'4–5',text:'Weekdays',answers:['A','C']}] }
    ] }]
};
const upload = data => ({ name: 'fixture.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(data)) });

(async () => {
  const browser = await chromium.launch({ ...(process.env.ELP_BROWSER_CHANNEL === 'bundled' ? {} : {channel:'chrome'}), headless: true });
  try {
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
    const page = await context.newPage();
    const errors = []; page.on('pageerror', error => errors.push(error.message));
    await page.goto(`${base}/#library`);
    await page.locator('#libraryList').filter({ hasText: '还没有导入题包' }).waitFor().catch(async error => {
      console.error('Initial library state', {url:page.url(), errors, text:await page.locator('body').innerText()});
      throw error;
    });
    assert.deepEqual(await page.evaluate(()=>window.ELPLibrary.newestPacks([{id:'older',importedAt:'2026-09-30T01:00:00.2Z'},{id:'newer',importedAt:'2026-09-30T01:00:00.23Z'}]).map(item=>item.id)),['newer','older'],'fractional timestamp order changed latest selection');
    await page.locator('#libraryFiles').setInputFiles(upload({ writings: [] }));
    await page.locator('#libraryStatus').filter({ hasText: '没有识别到可导入题目' }).waitFor();
    assert.equal(await page.locator('#importLibrary').isDisabled(), true);
    await page.locator('#libraryFiles').setInputFiles(upload(pack));
    await page.locator('#libraryPreview:not(.hidden)').waitFor();
    assert.equal(await page.locator('#libraryPreview img').count(), 0);
    // Lose the acknowledgement after the server commits. Retry must retrieve
    // the original receipt, rather than upload again or create duplicates.
    await page.route(/\/api\/library\/import\/[a-f0-9]+$/, async route => {
      await route.fetch();
      await route.abort('failed');
    }, { times: 1 });
    await page.locator('#importLibrary').click();
    await page.getByRole('button', {name:'重试并确认导入结果'}).waitFor();
    await page.route('**/api/library/packs', route => route.fulfill({status:503,contentType:'application/json',body:JSON.stringify({error:'Synthetic refresh failure'})}), {times:1});
    await page.getByRole('button', {name:'重试并确认导入结果'}).click();
    await page.locator('#librarySuccess').filter({hasText:'导入成功'}).waitFor();
    await page.locator('#libraryList').filter({hasText:'列表刷新失败'}).waitFor();
    assert.match(await page.locator('#librarySuccess').textContent(), /新增 1 个题库/);
    await page.locator('#refreshLibrary').click();
    await page.locator('.library-row').waitFor();
    assert.equal(await page.locator('.library-row').count(), 1);
    assert.equal(await page.locator('.library-row img').count(), 0);
    const packRequests = [];
    const trackPack = request => { if (/\/api\/library\/packs\/[a-f0-9]+$/.test(request.url())) packRequests.push(request.url()); };
    page.on('request', trackPack);
    await page.evaluate(async () => {
      const index = await window.ELPLibrary.request('packs');
      await Promise.all(Array.from({length:8}, () => window.ELPLibrary.loadPack(index.packs[0].id)));
      await window.ELPLibrary.loadPack(index.packs[0].id);
    });
    page.off('request', trackPack);
    assert.equal(packRequests.length, 1, 'concurrent and repeated reads downloaded the same immutable pack');
    await page.getByRole('button', {name:'继续导入其他题库'}).click();
    await page.locator('#libraryFiles').setInputFiles(upload(pack));
    await page.locator('#importLibrary:not([disabled])').waitFor();
    await page.locator('#importLibrary').click();
    await page.locator('#importLibrary[disabled]').waitFor();
    await page.waitForFunction(() => document.getElementById('libraryFiles').value === '');
    assert.equal(await page.locator('.library-row').count(), 1, 'repeat import duplicated pack');
    await page.reload();
    await page.locator('.library-row').waitFor();
    const invalid = structuredClone(pack); invalid.units[0].groups[0].questions[0].answers = [];
    await page.locator('#libraryFiles').setInputFiles(upload(invalid));
    await page.locator('#libraryStatus.is-error').waitFor();
    assert.equal(await page.locator('.library-row').count(), 1, 'failed import damaged previous library');
    assert.equal(await page.locator('#libraryFiles').isEnabled(), true, 'cannot retry failed preview');
    await page.evaluate(() => window.dispatchEvent(new CustomEvent('elp:storage-changed')));
    assert.equal(await page.locator('#importLibrary').isDisabled(), true, 'directory switch kept a pending import');
    assert.equal(await page.locator('#libraryFiles').inputValue(), '', 'directory switch kept old files');
    await page.locator('.library-row').waitFor();
    await page.locator('#libraryHeading').scrollIntoViewIfNeeded();
    const artifacts = path.resolve(__dirname, '../dist-test/library-v1'); await fs.mkdir(artifacts, { recursive: true });
    await page.screenshot({ path: path.join(artifacts, 'desktop.png'), fullPage: true });
    await page.setViewportSize({ width: 390, height: 844 });
    await page.locator('#libraryHeading').scrollIntoViewIfNeeded();
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, 'mobile horizontal overflow');
    await page.screenshot({ path: path.join(artifacts, 'mobile.png'), fullPage: true });
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.locator('.nav-item[data-route="reading"]').click();
    await page.getByRole('button', {name:'＋ 新建练习',exact:true}).click();
    await page.getByRole('button', {name:'开始练习',exact:true}).click();
    await page.locator('[data-question="q1"]').fill('Monday');
    assert.equal(await page.locator('.objective-answer table').count(),1,'question table was not rendered');
    await page.locator('[data-question="q2"][value="FALSE"]').check();
    await page.locator('select[data-question="q3"]').selectOption('B');
    await page.locator('[data-question="q4"][value="A"]').check();
    await page.locator('[data-question="q4"][value="B"]').check();
    await page.locator('#question-q1').getByRole('button', {name:'标记待检查'}).click();
    assert.equal(await page.locator('.objective-note input').isVisible(), false, 'passage notes stay collapsed until needed');
    await page.locator('.objective-note-disclosure summary').click();
    await page.locator('.objective-note input').fill('Opening day = Monday');
    await page.evaluate(() => {
      const paragraph = document.querySelector('#passage-A > p');
      const range = document.createRange(); range.setStart(paragraph.firstChild,4); range.setEnd(paragraph.firstChild,13);
      const selection = getSelection(); selection.removeAllRanges(); selection.addRange(range);
      paragraph.dispatchEvent(new MouseEvent('mouseup'));
    });
    await page.getByRole('button',{name:'划线选中文字',exact:true}).click();
    assert.equal(await page.locator('#passage-A mark').textContent(),'community');
    await page.getByRole('button',{name:'暂停',exact:true}).click();
    assert.equal(await page.locator('[data-question="q1"]').isDisabled(), true, 'pause must disable answers');
    await page.getByRole('button',{name:'继续',exact:true}).click();
    let releaseSave, saveStarted;
    const saving=new Promise(resolve=>{saveStarted=resolve;});
    const holdSave=new Promise(resolve=>{releaseSave=resolve;});
    await page.route('**/api/library/attempts/*',async route=>{
      if(route.request().method()==='PUT'){saveStarted();await holdSave;}
      await route.continue();
    });
    await page.locator('[data-question="q1"]').fill('Monday');
    await page.getByRole('button',{name:'保存并返回',exact:true}).click();
    await saving;
    await page.locator('.nav-item[data-route="listening"]').click();
    releaseSave();
    await page.locator('[data-page="listening"] .objective-heading h2').filter({hasText:'听力练习'}).waitFor();
    assert.ok(page.url().endsWith('#listening'),'late save navigation overrode the latest user route');
    await page.unroute('**/api/library/attempts/*');
    await page.locator('.nav-item[data-route="reading"]').click();
    await page.getByRole('button',{name:'继续作答',exact:true}).waitFor();
    await page.reload();
    await page.getByRole('button',{name:'继续作答',exact:true}).click();
    assert.equal(await page.locator('[data-question="q1"]').inputValue(), 'Monday');
    assert.equal(await page.locator('.objective-note input').inputValue(), 'Opening day = Monday');
    assert.equal(await page.locator('#passage-A mark').textContent(),'community','highlight did not persist');
    assert.equal(await page.locator('#question-q1').getByRole('button',{name:'已标记',exact:true}).count(), 1);
    await page.route('**/api/library/attempts/*', async route => {
      if (route.request().method()==='PUT') await route.fulfill({status:500,contentType:'application/json',body:JSON.stringify({error:'Synthetic disk failure'})});
      else await route.continue();
    });
    await page.locator('[data-question="q1"]').fill('MONDAY');
    await page.locator('.nav-item[data-route="writing"]').click();
    await page.waitForFunction(()=>location.hash.startsWith('#reading/session/'));
    await page.locator('#objectiveError').filter({hasText:'Synthetic disk failure'}).waitFor();
    assert.equal(await page.locator('[data-question="q1"]').inputValue(),'MONDAY','failed save lost the draft');
    await page.unroute('**/api/library/attempts/*');
    await page.getByRole('button',{name:'保存并返回',exact:true}).click();
    await page.getByRole('button',{name:'继续作答',exact:true}).click();
    await page.screenshot({path:path.join(artifacts,'reading-desktop.png'),fullPage:true});
    await page.setViewportSize({width:390,height:844});
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),true,'reading mobile overflow');
    await page.screenshot({path:path.join(artifacts,'reading-mobile.png'),fullPage:true});
    await page.getByRole('button',{name:'完成作答',exact:true}).click();
    await page.locator('.objective-score').waitFor();
    assert.equal(await page.locator('.objective-score strong').textContent(),'3 / 5');
    assert.equal(await page.locator('.objective-review table').count(),1,'report lost question table context');
    assert.equal(await page.locator('#review-q1').isVisible(),false,'correct answers stay secondary to mistakes');
    await page.getByRole('button',{name:'全部 4 项',exact:true}).click();
    await page.getByRole('button',{name:'查看原文依据',exact:true}).click();
    assert.equal(await page.locator('#passage-A').evaluate(node=>node.classList.contains('is-evidence')),true);
    await page.getByRole('button',{name:'返回第 1 题',exact:true}).click();
    assert.equal(await page.evaluate(()=>document.activeElement.id),'review-q1');
    await page.reload();
    await page.locator('.objective-score').waitFor();
    assert.equal(await page.locator('.objective-score strong').textContent(),'3 / 5','report must restore');
    await page.getByRole('button',{name:'重练本篇',exact:true}).click();
    await page.locator('[data-question="q1"]').waitFor();
    assert.equal(await page.locator('[data-question="q1"]').inputValue(),'','review must hide prior answers');
    await page.getByRole('button',{name:'保存并返回',exact:true}).click();
    await page.getByRole('button',{name:'查看复盘',exact:true}).waitFor();
    const records = await page.evaluate(async()=> (await (await fetch('/api/library/attempts')).json()).attempts);
    assert.equal(records.length,2); assert.equal(records.filter(item=>item.reviewOf).length,1);
    await page.getByRole('button',{name:'查看复盘',exact:true}).click();
    await page.getByRole('button',{name:'只重练错题',exact:true}).click();
    await page.locator('#question-q2').waitFor();
    assert.equal(await page.locator('#question-q1').count(),0,'correct answers leaked into wrong-only review');
    await page.getByRole('button',{name:'完成作答',exact:true}).click();
    await page.locator('.objective-score').waitFor();
    assert.equal(await page.locator('.objective-score strong').textContent(),'0 / 3','wrong-only score used full test denominator');

    const imageId = await page.evaluate(async () => {
      const bytes=Uint8Array.from(atob('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aD1sAAAAASUVORK5CYII='),c=>c.charCodeAt(0));
      return (await (await fetch('/api/library/media',{method:'POST',headers:{'Content-Type':'image/png'},body:bytes})).json()).id;
    });
    const languagePack={version:1,title:'Writing and speaking fixtures',source:{name:'Synthetic',status:'generated'},units:[
      {id:'chart',skill:'writing',part:'Task-1-Academic',title:'Chart fixture',prompt:'Describe this chart.',minutes:20,images:[imageId]},
      {id:'talk',skill:'speaking',part:'p2',title:'Describe a library',prompt:'Describe a library.\nYou should say:\nWhere it is\nWhen you visited\nAnd explain why you remember it.',minutes:2},
      {id:'map-reading',skill:'reading',part:'academic',title:'Reading image fixture',prompt:'Read the diagram.',minutes:20,images:[imageId],passages:[{id:'A',text:'The library opens on Monday.'}],groups:pack.units[0].groups}
    ]};
    const imported=await page.evaluate(async pack=> (await fetch('/api/library/packs',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(pack)})).status,languagePack);
    assert.equal(imported,201);
    await page.setViewportSize({width:1440,height:1000});
    await page.locator('.nav-item[data-route="writing"]').click();
    await page.locator('#newWriting').click();
    await page.locator('#pickWritingQuestion').click();
    await page.locator('#writingQuestionPicker summary').click();
    await page.locator('#writingQuestionPicker').getByRole('button',{name:'使用本题',exact:true}).click();
    await page.waitForFunction(()=>document.getElementById('writingPrompt').value==='Describe this chart.');
    assert.equal(await page.locator('#writingType').inputValue(),'Task 1 Academic');
    assert.equal(await page.locator('#writingMinutes').inputValue(),'20');
    assert.equal(await page.locator('#writingPromptImagePreview img').count(),1);
    await page.locator('.nav-item[data-route="speaking"]').click();
    await page.locator('#newSpeaking').click();
    await page.locator('#pickSpeakingQuestion').click();
    await page.locator('#speakingQuestionPicker summary').click();
    await page.locator('#speakingQuestionPicker').getByRole('button',{name:'使用本题',exact:true}).click();
    await page.waitForFunction(()=>document.getElementById('speakingPart').value==='p2');
    assert.match(await page.locator('#speakingPrompt').inputValue(),/Where it is/);
    assert.match(await page.locator('#recordButton').textContent(),/1 分钟准备/);
    const data=await page.evaluate(async()=> (await (await fetch('/api/data')).json()).data);
    assert.equal(data.writings[0].questionRef.unitId,'chart');
    assert.match(data.writings[0].promptImages[0],/^\/api\/study\/media\//);
    assert.equal(await page.evaluate(async url => {const response=await fetch(url);return response.ok && (await response.blob()).size>0;},data.writings[0].promptImages[0]),true,'saved question image is unavailable');
    assert.equal(data.speaking[0].questionRef.unitId,'talk');
    await page.evaluate(()=>{location.hash='reading/new';});
    await page.locator('[data-unit-id="map-reading"]').getByRole('button',{name:'开始练习',exact:true}).click();
    await page.getByRole('button',{name:'放大题图',exact:true}).click();
    await page.locator('#reviewImageLightbox:not(.hidden)').waitFor();
    await page.locator('#closeReviewImageLightbox').click();
    await page.getByRole('button',{name:'保存并返回',exact:true}).click();
    if (process.env.ELP_CONVERTED_PACKS) {
      const sourcePack = JSON.parse(await fs.readFile(path.join(process.env.ELP_CONVERTED_PACKS,'reading-starter.json'),'utf8'));
      const response = await page.evaluate(async pack => {
        const response=await fetch('/api/library/packs',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(pack)});
        return {status:response.status,body:await response.json()};
      },sourcePack);
      assert.equal(response.status,201,JSON.stringify(response.body));
      for (const unit of sourcePack.units) {
        await page.evaluate(()=>{location.hash='reading/new';});
        const row=page.locator(`.objective-library-row[data-unit-id="${unit.id}"]`);
        await row.getByRole('button',{name:'开始练习',exact:true}).click();
        await page.locator('.objective-answer').waitFor();
        assert.equal(await page.locator('.objective-paragraph').count(),unit.passages.length);
        assert.equal(await page.locator('.objective-answer table').count(),unit.groups.filter(group=>group.table?.length).length,'source question table missing');
        for (const paragraph of unit.passages) assert.equal(await page.locator(`#passage-${paragraph.id} > p`).first().textContent(),paragraph.text,'passage text changed during rendering');
        for (const group of unit.groups) for (const question of group.questions) {
          const field=page.locator(`#question-${question.id}`);
          if(group.kind==='text') await field.locator('input').fill(question.answers[0]);
          else if(group.kind==='matching') await field.locator('select').selectOption(question.answers[0]);
          else for(const answer of group.kind==='multiple'?question.answers:[question.answers[0]]) await field.locator(`input[value="${answer}"]`).check();
        }
        if(unit===sourcePack.units[0]) await page.screenshot({path:path.join(artifacts,'reading-local-source.png'),fullPage:true});
        await page.getByRole('button',{name:'完成作答',exact:true}).click();
        await page.locator('.objective-score').waitFor();
        const total=unit.groups.reduce((sum,group)=>sum+group.questions.reduce((n,q)=>n+(group.kind==='multiple'?q.answers.length:1),0),0);
        assert.equal(await page.locator('.objective-score strong').textContent(),`${total} / ${total}`,`source UI marking failed for ${unit.id}`);
      }
      console.log(`Validated all ${sourcePack.units.length} converted reading passages through the actual browser and disk API.`);
    }
    const revised=structuredClone(pack);revised.units[0].title='Updated community library';
    await page.evaluate(async value => { const response=await fetch('/api/library/packs',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(value)});if(!response.ok)throw new Error('update import failed'); },revised);
    const latest=await page.evaluate(()=>window.ELPLibrary.listUnits('reading'));
    assert.equal(latest.filter(item=>item.unit.id==='reading-1').length,1,'updated banks duplicated selectable units');
    assert.equal(latest.find(item=>item.unit.id==='reading-1').unit.title,'Updated community library');
    await page.evaluate(id=>{location.hash=`reading/report/${id}`;},records.find(item=>item.status==='submitted').id);
    await page.waitForFunction(()=>document.querySelector('.objective-heading h2')?.textContent==='The community library' && document.querySelector('.objective-score'));
    assert.equal(await page.locator('.objective-heading h2').textContent(),'The community library','updating bank changed historical question');
    await require('./listening-ui.cjs')(page,base,artifacts);
    await require('./backup-ui.cjs')(page,base);
    await require('./four-skill-plan-ui.cjs')(page);
    await require('./mock-ui.cjs')(page,base,artifacts);
    await require('./storage-ui.cjs')(page,base);
    await require('./ux-acceptance.cjs')(page,artifacts);
    await require('./authored-bank-ui.cjs')(page);
    assert.deepEqual(errors, []);
    console.log('Library/reading acceptance passed: real disk import, duplicates, validation, four question types, pause, autosave, reload, evidence navigation, partial credit, separate review, desktop/mobile.');
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
