const assert=require('node:assert/strict');
const {chromium}=require(process.env.ELP_PLAYWRIGHT_MODULE || 'playwright');
const base=process.env.ELP_TEST_URL;
const snapshot=async()=> (await fetch(base+'/api/data')).json();
(async()=>{
 const initial=await snapshot();
 const response=await fetch(base+'/api/data',{method:'PUT',headers:{'Content-Type':'application/json','If-Match':initial.revision,'X-ELP-Directory':initial.storage.directoryId},body:JSON.stringify({data:{writings:[{id:'a',essay:'old a'},{id:'b',essay:'old b'},{id:'timer',type:'Task 2',minutes:40,prompt:'Timer recovery',essay:'An unfinished answer.',status:'paused',elapsedSeconds:600,timerState:{elapsedMilliseconds:600000,anchor:null,paused:true}}],speaking:[],mistakes:[]}})});
 assert.equal(response.status,200);
 const browser=await chromium.launch({...(process.env.ELP_BROWSER_CHANNEL==='bundled'?{}:{channel:'chrome'}),headless:true});
 try {
  const context=await browser.newContext({timezoneId:'Asia/Shanghai'});
  const a=await context.newPage(),b=await context.newPage();
  for(const page of [a,b]) {
   await page.goto(base+'/#home');
   await page.waitForFunction(()=>window.ELPStorage);
   await page.evaluate(async()=>{const result=await(await fetch('/api/data')).json();window.auditClient=ELPStorage.create();window.auditState=auditClient.adopt(result.data,result);});
  }
  let dropped=false,calls=0;
  await a.route('**/api/data',async route=>{
   if(route.request().method()!=='PATCH')return route.continue();
   calls++;
   if(!dropped){dropped=true;const saved=await route.fetch();assert.equal(saved.status(),200);return route.abort('failed');}
   return route.continue();
  });
  await a.evaluate(async()=>{auditState.writings[0].essay='first page';await auditClient.save();});
  assert.equal(calls,2,'lost acknowledgment must retry the same durable commit');
  await b.evaluate(async()=>{auditState.writings[1].essay='second page';await auditClient.save();});
  const conflict=await b.evaluate(async()=>{auditState.writings[0].essay='unseen overwrite';try{await auditClient.save();return '';}catch(error){return error.message;}});
  assert.match(conflict,/其他页面|更新/);
  let saved=await snapshot();assert.equal(saved.data.writings[0].essay,'first page');assert.equal(saved.data.writings[1].essay,'second page');
  await a.unroute('**/api/data');await b.close();
  await a.reload();await a.locator('.nav-item[data-route="writing"]').click();await a.locator('[data-writing-id="timer"]').click();
  assert.equal(await a.locator('#writingTimer').innerText(),'30:00');assert.equal(await a.locator('#writingEssay').getAttribute('readonly'),'');
  await a.reload();await a.locator('[data-writing-id="timer"]').click();assert.equal(await a.locator('#writingTimer').innerText(),'30:00');
  // A real MediaRecorder encodes a synthetic tone, then transcription is hung.
  // Closing this page simulates losing the renderer after the capture commit.
  await a.route('**/api/transcription/status',route=>route.fulfill({json:{ready:true}}));
  await a.reload();await a.locator('.nav-item[data-route="speaking"]').click();await a.locator('#newSpeaking').click();
  await a.evaluate(()=>{
   window.localWhisper.transcribe=()=>{window.transcriptionStarted=true;return new Promise(()=>{});};
   navigator.mediaDevices.getUserMedia=async()=>{const audio=new AudioContext();const tone=audio.createOscillator();const destination=audio.createMediaStreamDestination();tone.connect(destination);tone.start();window.auditAudio=audio;return destination.stream;};
  });
  await a.locator('#recordButton').click();await a.waitForTimeout(350);await a.locator('#recordButton').click();
  await a.waitForFunction(()=>window.transcriptionStarted);
  saved=await snapshot();const voice=saved.data.speaking[0];assert.equal(voice.transcriptionStatus,'pending');assert.match(voice.audio,/^\/api\/study\/media\//);
  const bytes=await(await fetch(base+voice.audio)).arrayBuffer();assert.ok(bytes.byteLength>100,'audio must be on disk before transcription finishes');
  await a.close();
  const restored=await context.newPage();await restored.goto(base+'/#speaking');await restored.locator(`[data-speaking-id="${voice.id}"]`).click();
  await restored.waitForFunction(()=>Boolean(document.getElementById('speakingPlayback').getAttribute('src')));
  assert.equal(await restored.evaluate(()=>ELPStudy.dateOf('2026-10-02T16:30:00Z')),'2026-10-03');
  console.log('Real browser/API/disk acceptance: response loss, disjoint writes, true conflict, paused timer reload, recording persisted before hung transcription, renderer loss and local date.');
 } finally {await browser.close();}
})().catch(error=>{console.error(error);process.exitCode=1;});
