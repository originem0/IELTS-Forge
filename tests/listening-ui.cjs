const assert = require('node:assert/strict');
const path = require('node:path');
const fs = require('node:fs/promises');

function silentWave(seconds=16) {
  const samples=8000*seconds; const buffer=Buffer.alloc(44+samples*2);
  buffer.write('RIFF',0);buffer.writeUInt32LE(buffer.length-8,4);buffer.write('WAVEfmt ',8);
  buffer.writeUInt32LE(16,16);buffer.writeUInt16LE(1,20);buffer.writeUInt16LE(1,22);
  buffer.writeUInt32LE(8000,24);buffer.writeUInt32LE(16000,28);buffer.writeUInt16LE(2,32);buffer.writeUInt16LE(16,34);
  buffer.write('data',36);buffer.writeUInt32LE(samples*2,40);return buffer;
}

module.exports=async function testListening(page, base, artifacts) {
  const response=await page.request.post(`${base}/api/library/media`,{headers:{'Content-Type':'audio/wav'},data:silentWave()});
  assert.equal(response.status(),201);const media=await response.json();
  const pack={version:1,title:'Listening fixture',source:{name:'Synthetic listening',status:'generated'},units:[{
    id:'listen-1',skill:'listening',part:'1',title:'Booking a library room',prompt:'Listen and complete the booking.',minutes:0,
    audio:media.id,transcript:'The booking is for Monday. The speaker says Monday.',
    groups:[{id:'g1',kind:'text',instruction:'ONE WORD ONLY',maxWords:1,questions:[{id:'lq1',label:'1',text:'Booking day: ____',answers:['Monday']}]}]
  }]};
  const imported=await page.request.post(`${base}/api/library/packs`,{data:pack});assert.equal(imported.status(),201);
  await page.locator('.nav-item[data-route="listening"]').click();
  await page.getByRole('button',{name:'＋ 新建练习',exact:true}).click();
  await page.getByRole('button',{name:'开始练习',exact:true}).click();
  await page.waitForFunction(()=>document.querySelector('.objective-audio audio')?.readyState>=1);
  assert.equal(await page.locator('.objective-transcript').count(),0,'transcript revealed before submission');
  await page.locator('[data-question="lq1"]').fill('Monday');
  await page.getByRole('button',{name:'0.75×',exact:true}).click();
  await page.evaluate(()=>{document.querySelector('.objective-audio audio').currentTime=7;});
  await page.waitForFunction(()=>document.querySelector('.objective-audio audio').currentTime>=7);
  await page.evaluate(()=>document.querySelector('.objective-audio audio').play());
  await page.getByRole('button',{name:'暂停',exact:true}).click();
  assert.equal(await page.locator('.objective-audio audio').evaluate(a=>a.paused),true);
  assert.equal(await page.locator('[data-question="lq1"]').isDisabled(),true);
  await page.getByRole('button',{name:'继续',exact:true}).click();
  await page.waitForFunction(()=>!document.querySelector('.objective-audio audio').paused);
  await page.getByRole('button',{name:'保存并返回',exact:true}).click();
  await page.getByRole('button',{name:'继续作答',exact:true}).waitFor();
  await page.reload();
  await page.getByRole('button',{name:'继续作答',exact:true}).click();
  await page.waitForFunction(()=>document.querySelector('.objective-audio audio')?.currentTime>=7);
  assert.equal(await page.locator('.objective-audio audio').evaluate(a=>a.playbackRate),.75);
  assert.equal(await page.locator('[data-question="lq1"]').inputValue(),'Monday');
  assert.equal(await page.locator('.objective-audio audio').evaluate(a=>a.paused),true,'restoring history must not autoplay');
  await page.getByRole('button',{name:'后退 10 秒',exact:true}).click();
  await page.waitForFunction(()=>document.querySelector('.objective-audio audio').currentTime<1);
  await page.screenshot({path:path.join(artifacts,'listening-desktop.png'),fullPage:true});
  await page.setViewportSize({width:390,height:844});
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true,'listening mobile overflow');
  await page.screenshot({path:path.join(artifacts,'listening-mobile.png'),fullPage:true});
  await page.getByRole('button',{name:'完成作答',exact:true}).click();
  await page.locator('.objective-source-disclosure > summary').click();
  await page.locator('.objective-transcript').waitFor();
  assert.match(await page.locator('.objective-transcript').textContent(),/speaker says Monday/);
  assert.equal(await page.locator('.objective-score strong').textContent(),'1 / 1');
  await page.evaluate(()=>document.querySelector('.objective-audio audio').play());
  const oldAudio=await page.locator('.objective-audio audio').elementHandle();
  await page.locator('#menuButton').click();
  await page.locator('.nav-item[data-route="reading"]').click();
  await page.waitForFunction(()=>document.querySelector('[data-page="reading"]').classList.contains('is-active'));
  assert.equal(await oldAudio.evaluate(a=>a.paused),true,'leaving report did not stop audio');
  assert.equal(await page.locator('[data-page="listening"] #objectiveStatus').count(),0,'inactive module kept duplicate control ids');
  await page.setViewportSize({width:1440,height:1000});

  await page.route(`**/api/library/media/${media.id}`,route=>route.fulfill({status:404,body:'missing'}));
  await page.locator('.nav-item[data-route="listening"]').click();
  await page.getByRole('button',{name:'＋ 新建练习',exact:true}).click();
  await page.getByRole('button',{name:'开始练习',exact:true}).click();
  await page.getByRole('button',{name:'重试加载音频',exact:true}).waitFor();
  await page.locator('[data-question="lq1"]').fill('Tuesday');
  await page.getByRole('button',{name:'完成作答',exact:true}).click();
  await page.locator('#objectiveError').filter({hasText:'音频尚未恢复'}).waitFor();
  assert.equal(await page.locator('.objective-score').count(),0,'missing audio was submitted as a completed session');
  await page.unroute(`**/api/library/media/${media.id}`);
  await page.getByRole('button',{name:'重试加载音频',exact:true}).click();
  await page.waitForFunction(()=>document.querySelector('.objective-audio audio')?.readyState>=3);
  assert.equal(await page.locator('[data-question="lq1"]').inputValue(),'Tuesday','audio recovery lost the answer');
  await page.getByRole('button',{name:'完成作答',exact:true}).click();
  await page.locator('.objective-score').waitFor();
  assert.equal(await page.locator('.objective-score strong').textContent(),'0 / 1');

  // A listening unit with no question groups is a listen-only resource: it plays audio and
  // reveals the transcript on demand, but shows no answer sheet and is never graded.
  const resourcePack={version:1,title:'Listen-only fixture',source:{name:'Cambridge-style resource',status:'verified'},units:[{
    id:'listen-resource-1',skill:'listening',part:'2',title:'Listen-only Part 2',minutes:0,
    prompt:'Listen; the questions are in the paper book.',audio:media.id,transcript:'This recording is a listen-only resource.'
  }]};
  assert.equal((await page.request.post(`${base}/api/library/packs`,{data:resourcePack})).status(),201,'listen-only pack (no groups) rejected by importer');
  await page.evaluate(()=>{location.hash='listening/new';});
  const resourceRow=page.locator('.objective-library-row').filter({has:page.getByRole('heading',{name:'Listen-only Part 2',exact:true})});
  assert.match(await resourceRow.locator('p').textContent(),/听音资源/,'listen-only unit not labelled as a resource');
  await resourceRow.getByRole('button',{name:'开始收听',exact:true}).click();
  await page.waitForFunction(()=>document.querySelector('.objective-audio audio')?.readyState>=1);
  assert.equal(await page.locator('.objective-answer').count(),0,'listen-only resource must not show an answer sheet');
  assert.equal(await page.getByRole('button',{name:'完成作答',exact:true}).count(),0,'listen-only resource must not offer grading');
  assert.equal(await page.locator('.objective-transcript').isVisible(),false,'listen-only transcript shown before it is expanded');
  await page.locator('.objective-source-disclosure > summary').click();
  await page.locator('.objective-transcript').waitFor();
  assert.match(await page.locator('.objective-transcript').textContent(),/listen-only resource/);

  if(process.env.ELP_LISTENING_PACKS) {
    const directory=process.env.ELP_LISTENING_PACKS;
    const packPath=path.join(directory,'listening-starter.json');
    const actual=JSON.parse(await fs.readFile(packPath,'utf8'));
    await page.locator('.nav-item[data-route="settings"]').click();
    await page.locator('#libraryFiles').setInputFiles([packPath,...[...new Set(actual.units.map(unit=>unit.audio))].map(name=>path.join(directory,name))]);
    await page.locator('#importLibrary:not([disabled])').waitFor();await page.locator('#importLibrary').click();
    await page.waitForFunction(()=>document.getElementById('libraryFiles').value==='');
    for(const unit of actual.units) {
      await page.evaluate(()=>{location.hash='listening/new';});
      const row=page.locator('.objective-library-row').filter({has:page.getByRole('heading',{name:unit.title,exact:true})});
      await row.getByRole('button',{name:'开始练习',exact:true}).click();
      await page.waitForFunction(()=>document.querySelector('.objective-audio audio')?.readyState>=1);
      assert.ok(await page.locator('.objective-audio audio').evaluate(a=>Number.isFinite(a.duration)&&a.duration>5),'source audio failed decoding');
      const midpoint=await page.locator('.objective-audio audio').evaluate(a=>{a.currentTime=a.duration/2;return a.currentTime;});
      await page.locator('.objective-audio audio').evaluate(a=>a.play());
      await page.waitForFunction(value=>document.querySelector('.objective-audio audio').currentTime>value+.1,midpoint);
      await page.locator('.objective-audio audio').evaluate(a=>a.pause());
      for(const group of unit.groups)for(const q of group.questions) {
        const field=page.locator(`#question-${q.id}`);
        if(group.kind==='text') await field.locator('input').fill(q.answers[0]);
        else await field.locator(`input[value="${q.answers[0]}"]`).check();
      }
      await page.getByRole('button',{name:'完成作答',exact:true}).click();await page.locator('.objective-source-disclosure > summary').click();await page.locator('.objective-transcript').waitFor();
      const count=unit.groups.reduce((sum,g)=>sum+g.questions.length,0);
      assert.equal(await page.locator('.objective-score strong').textContent(),`${count} / ${count}`,`source answer rules disagree: ${unit.title}`);
    }
    console.log(`Validated ${actual.units.length} local listening sections, actual audio decoding and source-key marking.`);
  }
  console.log('Listening acceptance passed: playback, pause, saved cursor/rate/answers, reload, transcript gating, marking, navigation cleanup and responsive layout.');
};
module.exports.silentWave = silentWave;
