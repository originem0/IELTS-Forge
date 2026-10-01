const assert=require('node:assert/strict');
const path=require('node:path');
const {silentWave}=require('./listening-ui.cjs');

function examPack(skill,audios=[]) {
  const counts=skill==='reading'?[13,13,14]:[10,10,10,10];let number=1;
  const units=counts.map((count,index)=>({
    id:`mock-${skill}-${index+1}`,skill,part:skill==='reading'?'academic':String(index+1),title:`${skill} part ${index+1}`,prompt:'Answer the questions.',minutes:skill==='reading'?20:0,
    ...(skill==='reading'?{passages:[{id:'A',text:'The library opens on Monday.'}]}:{audio:audios[index],transcript:`Section ${index+1}: The day is Monday.`}),
    groups:[{id:'g1',kind:'text',instruction:'ONE WORD ONLY',maxWords:1,questions:Array.from({length:count},(_,i)=>({id:`q${i+1}`,label:String(number++),text:'Day: ____',answers:['Monday']}))}]
  }));
  return {version:1,title:`${skill} complete fixture`,source:{name:'Synthetic complete-test fixture',status:'generated'},units,
    exams:[{id:`exam-${skill}`,title:`Complete ${skill} simulation`,skill,unitIds:units.map(unit=>unit.id),minutes:skill==='reading'?60:30}]};
}

module.exports=async function mockUI(page,base,artifacts) {
  const upload=async pack=>page.evaluate(value=>window.ELPLibrary.request('packs',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(value)}),pack);
  await upload(examPack('reading'));
  await page.evaluate(()=>{location.hash='reading/mock';});
  await page.getByRole('button',{name:'开始模拟',exact:true}).click();
  await page.locator('[data-question="s1-q1"]').waitFor();
  assert.equal(await page.getByRole('button',{name:'暂停',exact:true}).count(),0,'simulation exposed pause');
  assert.equal(await page.locator('.objective-question').count(),40);
  assert.match(await page.locator('#objectiveTimer').textContent(),/^(60:00|59:\d\d)$/);
  await page.locator('[data-question="s1-q1"]').fill('Monday');
  const id=await page.evaluate(()=>location.hash.split('/').pop());
  const before=await page.evaluate(id=>window.ELPLibrary.request(`attempts/${id}`),id);
  await page.getByRole('button',{name:'保存并离开（计时继续）',exact:true}).click();
  await page.getByRole('button',{name:'继续作答',exact:true}).first().waitFor();
  await page.evaluate(id=>{location.hash=`reading/session/${id}`;},id);
  await page.locator('[data-question="s1-q1"]').waitFor();
  const after=await page.evaluate(id=>window.ELPLibrary.request(`attempts/${id}`),id);
  assert.equal(after.deadlineAt,before.deadlineAt,'reopening reset deadline');
  assert.equal(await page.locator('[data-question="s1-q1"]').inputValue(),'Monday');
  await page.getByRole('button',{name:'完成作答',exact:true}).click();await page.locator('.objective-score').waitFor();
  assert.equal(await page.locator('.objective-score strong').textContent(),'1 / 40');
  await page.getByRole('button',{name:'只重练错题',exact:true}).click();await page.locator('.objective-question').first().waitFor();
  assert.equal(await page.locator('.objective-question').count(),39);
  assert.equal(await page.getByRole('button',{name:'暂停',exact:true}).count(),1,'wrong-only practice should allow normal study controls');
  await page.getByRole('button',{name:'保存并返回',exact:true}).click();

  const directory=await page.evaluate(()=>window.ELPLibrary.directoryId);const audioIDs=[];
  for(let i=0;i<4;i++) {
    const wav=silentWave(1);wav[44]=i;
    const response=await page.request.post(`${base}/api/library/media`,{headers:{'Content-Type':'audio/wav','X-ELP-Directory':directory},data:wav});
    assert.equal(response.status(),201);audioIDs.push((await response.json()).id);
  }
  await upload(examPack('listening',audioIDs));
  await page.evaluate(()=>{location.hash='listening/mock';});
  await page.getByRole('button',{name:'开始模拟',exact:true}).click();await page.locator('.objective-mock-audio').waitFor();
  assert.equal(await page.locator('.objective-audio audio').evaluate(audio=>audio.controls),false,'mock exposed seek controls');
  assert.equal(await page.getByRole('button',{name:'0.75×',exact:true}).count(),0);
  await page.getByRole('button',{name:'开始或继续播放',exact:true}).click();
  await page.getByRole('button',{name:'录音已播放完毕',exact:true}).waitFor();
  assert.equal(await page.getByRole('button',{name:'录音已播放完毕',exact:true}).isDisabled(),true);
  const listeningID=await page.evaluate(()=>location.hash.split('/').pop());
  const listened=await page.evaluate(id=>window.ELPLibrary.request(`attempts/${id}`),listeningID);
  assert.equal(listened.audioTrack,3);assert.equal(listened.audioEnded,true);
  await page.getByRole('button',{name:'保存并离开（计时继续）',exact:true}).click();
  await page.getByRole('button',{name:'继续作答',exact:true}).first().waitFor();
  await page.reload();
  await page.getByRole('button',{name:'继续作答',exact:true}).first().waitFor();
  await page.evaluate(id=>{location.hash=`listening/session/${id}`;},listeningID);
  await page.getByRole('button',{name:'录音已播放完毕',exact:true}).waitFor();
  assert.equal(await page.getByRole('button',{name:'录音已播放完毕',exact:true}).isDisabled(),true,'reload enabled a second playback');
  await page.getByRole('button',{name:'完成作答',exact:true}).click();await page.locator('.objective-source-disclosure > summary').click();await page.locator('.objective-transcript').waitFor();
  assert.equal(await page.locator('.objective-score strong').textContent(),'0 / 40');
  assert.equal(await page.locator('.objective-audio [aria-pressed]').filter({hasText:/Section/}).count(),4,'review lost per-section replay');
  await page.screenshot({path:path.join(artifacts,'mock-report.png'),fullPage:true});

  await page.locator('.nav-item[data-route="home"]').click();
  await page.waitForFunction(()=>document.getElementById('metricLibraryStatus').textContent==='含重练与模拟');
  const metrics=await page.evaluate(async()=>{
    const summary=await window.ELPLibrary.request('summary');const data=(await(await fetch('/api/data')).json()).data;
    return {summary,data,reading:document.getElementById('metricReading').textContent,listening:document.getElementById('metricListening').textContent,total:document.getElementById('metricTotal').textContent,streak:document.getElementById('metricStreak').textContent};
  });
  assert.equal(Number(metrics.reading),metrics.summary.counts.reading);assert.equal(Number(metrics.listening),metrics.summary.counts.listening);
  assert.equal(Number(metrics.total),metrics.data.writings.length+metrics.data.speaking.length+metrics.summary.counts.reading+metrics.summary.counts.listening);
  assert.ok(Number(metrics.streak)>=1,'local activity date did not contribute to streak');
  console.log('Mock UI passed: 40 questions, persistent deadlines, separate wrong-only practice, four-track single playback, review replay and complete home metrics.');
};
