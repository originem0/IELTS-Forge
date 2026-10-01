// Browser journeys with synthetic records. Real disk/API coverage lives in library-ui.cjs.
const {chromium}=require(process.env.ELP_PLAYWRIGHT_MODULE || 'playwright');
const assert=require('node:assert/strict');
const fs=require('node:fs/promises'),path=require('node:path'),http=require('node:http');
const applyStateRequest=require('./state-api-fixture.cjs');
const root=path.resolve(__dirname,'..');
const feedback='### 总体评价\n\n观点清楚，补充一个例子。';
const originalSpeaking={id:'s1',part:'p2',prompt:'Describe a place where you enjoy reading.',transcript:'I enjoy reading at home.',review:feedback,reviewInput:{original:'I enjoy reading at home.',prompt:'Describe a place where you enjoy reading.',type:'Part 2'},updatedAt:'2026-10-01T01:00:00Z'};
let data={writings:[{id:'w1',type:'Task 2',minutes:40,prompt:'Should libraries remain free?',essay:'Libraries help people.',review:feedback,updatedAt:'2026-10-01T01:00:00Z'}],speaking:[structuredClone(originalSpeaking)],mistakes:[]};
let failWrites=false;
const server=http.createServer(async(req,res)=>{
 res.setHeader('Cache-Control','no-store');
 if(req.url.startsWith('/api/')){
  res.setHeader('Content-Type','application/json');
  if(req.url==='/api/data'){
   if(['PATCH','PUT'].includes(req.method)){
    if(failWrites){res.statusCode=503;return res.end('{"error":"Synthetic save failure"}');}
    let body='';for await(const part of req)body+=part;data=applyStateRequest(data,JSON.parse(body),req.method);
   }
   return res.end(JSON.stringify({data,storage:{bound:true,ready:true,directoryId:'ux-test'}}));
  }
  if(req.url==='/api/ai/status')return res.end('{"connected":false}');
  if(req.url==='/api/transcription/status')return res.end('{"ready":false}');
  if(req.url==='/api/library/packs')return res.end('{"packs":[]}');
  if(req.url==='/api/library/attempts')return res.end('{"attempts":[]}');
  if(req.url==='/api/library/summary')return res.end('{"counts":{"reading":0,"listening":0},"activityDates":[]}');
  res.statusCode=404;return res.end('{}');
 }
 const target=path.join(root,'app',req.url==='/'?'index.html':req.url.split('?')[0]);
 if(!target.startsWith(path.join(root,'app')+path.sep)){res.statusCode=403;return res.end();}
 try{res.setHeader('Content-Type',({'.html':'text/html; charset=utf-8','.js':'text/javascript; charset=utf-8','.css':'text/css; charset=utf-8'})[path.extname(target)]||'application/octet-stream');res.end(await fs.readFile(target));}catch{res.statusCode=404;res.end();}
});
(async()=>{
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 const browser=await chromium.launch({channel:'chrome',headless:true});
 try{
  const page=await browser.newPage({viewport:{width:1440,height:900}});page.setDefaultTimeout(8000);
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto(`http://127.0.0.1:${server.address().port}/#speaking`);
  await page.locator('[data-speaking-id="s1"]').click();await page.locator('#retryReviewSource').click();
  await page.waitForURL('**/#speaking');
  assert.equal(await page.locator('#speakingTranscript').inputValue(),'');
  assert.equal(await page.locator('#speakingPart').inputValue(),'p2');
  assert.equal(data.speaking.length,2);
  const child=data.speaking.find(item=>item.id!=='s1');
  assert.equal(child.parentSessionId,'s1');assert.equal(child.attemptNumber,2);
  const saved=page.waitForResponse(response=>response.url().endsWith('/api/data')&&response.request().method()==='PATCH');
  await page.locator('#speakingTranscript').fill('My second answer uses a new example.');await saved;
  assert.equal(data.speaking.find(item=>item.id==='s1').transcript,originalSpeaking.transcript);
  assert.equal(data.speaking.find(item=>item.id==='s1').review,feedback);
  assert.equal(data.speaking.find(item=>item.id===child.id).review,'');
  await page.reload();assert.equal(data.speaking.length,2);
  await page.locator('.nav-item[data-route="writing"]').click();await page.locator('[data-writing-id="w1"]').click();await page.locator('#retryReviewSource').click();
  await page.locator('#writingSessionView:not(.hidden)').waitFor();
  assert.equal(data.writings.length,2);assert.equal(await page.locator('#writingEssay').inputValue(),'');
  await page.locator('#writingEssay').fill('This is a new answer.');
  await page.locator('#toggleTimer').click();assert.equal(await page.locator('#writingEssay').getAttribute('readonly'),'');
  await page.locator('#toggleTimer').click();await page.locator('#finishWritingSession').click();
  await page.locator('#writingCompletion:not(.hidden)').waitFor();
  assert.equal(await page.locator('#toggleTimer').isVisible(),false);
  assert.equal(await page.locator('#finishWritingSession').isVisible(),false);
  assert.equal(await page.locator('#writingEssay').getAttribute('readonly'),'');
  await page.locator('#resumeWritingEdit').click();assert.equal(await page.locator('#writingEssay').getAttribute('readonly'),null);
  await page.locator('#returnWritingOverview').click();await page.locator('#newWriting').click();
  await page.locator('[data-writing-type="Task 1 General"]').click();
  assert.equal(await page.locator('#writingType').inputValue(),'Task 1 General');assert.equal(await page.locator('#writingMinutes').inputValue(),'20');
  assert.equal(await page.locator('#writingImagePanel').isVisible(),false);
  await page.locator('[data-writing-type="Task 1 Academic"]').click();assert.equal(await page.locator('#writingImagePanel').isVisible(),true);
  await page.locator('[data-writing-type="自由写作"]').click();assert.equal(await page.locator('#writingMinutes').inputValue(),'0');
  await page.locator('#writingMinutes').selectOption('60');assert.match(await page.locator('#startWritingSession').textContent(),/60/);
  await page.locator('.nav-item[data-route="speaking"]').click();
  await page.locator('[data-speaking-id="s1"]').click();
  const countBeforeFailure = data.speaking.length;
  failWrites = true;
  await page.locator('#retryReviewSource').click();
  await page.locator('#storageResult.is-error').waitFor({state:'attached'});
  assert.equal(data.speaking.length, countBeforeFailure, 'failed retry must not create a stored attempt');
  assert.equal(await page.locator('#reviewWorkspace').isVisible(), true, 'failed retry keeps the original report accessible');
  failWrites = false;
  await page.locator('#retryReviewSource').click();
  assert.equal(data.speaking.length, countBeforeFailure + 1);
  assert.equal(data.speaking.find(item => item.id === 's1').transcript, originalSpeaking.transcript);
  await page.waitForURL('**/#speaking');
  for (const skill of ['reading','listening']) {
    await page.evaluate(skill => { location.hash = `${skill}/new`; }, skill);
    await page.locator('.page.is-active[data-page="library"]').waitFor();
    assert.equal(await page.evaluate(() => window.ELPLibrary.returnSkill), skill);
    await page.locator('#libraryReturn').click();
    await page.locator(`.page.is-active[data-page="${skill}"]`).waitFor();
    await page.evaluate(skill => { location.hash = `${skill}/mock`; }, skill);
    await page.locator('.page.is-active[data-page="library"]').waitFor();
    assert.equal(await page.evaluate(() => window.ELPLibrary.returnSkill), skill);
  }
  assert.deepEqual(errors,[]);
  console.log('UX journeys passed: independent speaking/writing attempts, writing completion and visible task/time controls.');
 }finally{await browser.close();await new Promise(resolve=>server.close(resolve));}
})().catch(error=>{console.error(error);server.close();process.exitCode=1;});
