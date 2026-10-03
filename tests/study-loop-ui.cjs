// Isolated synthetic learner; no personal archive or provider credentials.
const {chromium}=require(process.env.ELP_PLAYWRIGHT_MODULE || 'playwright');
const assert=require('node:assert/strict'),http=require('node:http'),fs=require('node:fs/promises'),path=require('node:path');
const apply=require('./state-api-fixture.cjs');
const app=path.resolve(__dirname,'../app'),artifacts=path.resolve(__dirname,'../dist-test/learning-loop');
const E=require('../app/study-engine.js');
const date=new Date(),day=`${date.getFullYear()}-${String(date.getMonth()+1).padStart(2,'0')}-${String(date.getDate()).padStart(2,'0')}`,yesterday=E.addDays(day,-1);
const key=JSON.stringify(['writing','expression 89']);
let data={writings:[{id:'w-old',type:'Task 2',minutes:40,prompt:'Explain public libraries.',essay:'Libraries help local people.',status:'completed',review:'### 总体评价\nExplain one reason.',updatedAt:`${yesterday}T12:00:00Z`},{id:'question-only',prompt:'EXCLUDE_QUESTION_ONLY',essay:'',status:'completed'},{id:'unfinished',prompt:'EXCLUDE_UNFINISHED',essay:'An unfinished draft.',status:'draft'}],speaking:[],mistakes:[{id:'c1',kind:'correction',module:'writing',title:'He go home.',correction:{original:'He go home.',corrected:'He goes home.',explanation:'主谓一致'},createdAt:yesterday},{id:'v1',module:'vocabulary',title:'sustainable',text:'可持续的',vocabularyMode:'productive',cue:'可持续的',createdAt:yesterday},{id:'n1',module:'writing',title:'段落依据',text:'每个观点要解释原因。',createdAt:yesterday}],planProgress:{[yesterday]:{'writing-0':true}},languageBank:{speaking:[{title:"公园散步",personalCore:"I enjoy walking in the park.",reusableTopics:["户外活动"],expressions:["get some fresh air｜呼吸新鲜空气"],answerFrames:[],sourceKeys:[]}],writing:[{domain:'教育',collocations:Array.from({length:90},(_,i)=>`expression ${i}｜表达 ${i}`),sentencePatterns:[],sourceKeys:[]}],sourceKeys:[]},languagePractice:{[key]:{attempt:'Old answer must stay hidden.',lastReviewedDate:yesterday,dueDate:day,rating:'again',failures:1}}};
let fail=false,ai=false,requests=[],attempts=[],pack;
async function until(predicate) { const end=Date.now()+10000; while(!predicate()) { if(Date.now()>end)throw new Error('Timed out waiting for the saved fixture');await new Promise(resolve=>setTimeout(resolve,25)); } }
const server=http.createServer(async(req,res)=>{
 try {
  const url=new URL(req.url,'http://localhost');res.setHeader('Cache-Control','no-store');
  if(url.pathname.startsWith('/api/')) {
   res.setHeader('Content-Type','application/json');
   if(url.pathname==='/api/data') {
    if(['PATCH','PUT'].includes(req.method)) {let body='';for await(const chunk of req)body+=chunk;if(fail){res.statusCode=503;return res.end('{"error":"Synthetic save failure"}');}const payload=JSON.parse(body);if(process.env.ELP_STUDY_TRACE&&payload.metadata?.learning)console.log('learning write',Object.keys(payload.metadata.learning.objectiveItems||{}),Object.keys(payload.metadata.learning.observed||{}));data=apply(data,payload,req.method);}
    return res.end(JSON.stringify({data,storage:{bound:true,ready:true,directoryId:'study-test',directory:'study-test'}}));
   }
   if(url.pathname==='/api/ai/status')return res.end(JSON.stringify({connected:ai}));
   if(url.pathname==='/api/transcription/status')return res.end('{"ready":false}');
   if(url.pathname==='/api/library/summary')return res.end(JSON.stringify({counts:{reading:attempts.length,listening:0},activityDates:[]}));
   if(url.pathname==='/api/library/attempts')return res.end(JSON.stringify({attempts}));
   if(url.pathname==='/api/library/packs')return res.end(JSON.stringify({packs:pack?[{id:'test-pack',title:pack.title,source:pack.source,skills:{reading:1}}]:[]}));
   if(url.pathname==='/api/library/packs/test-pack')return res.end(JSON.stringify(pack));
   const match=url.pathname.match(/^\/api\/library\/attempts\/([^/]+)(\/result)?$/);
   if(match) {
    if(req.method==='PUT'){let body='';for await(const chunk of req)body+=chunk;const record={...JSON.parse(body),revision:1,createdAt:new Date().toISOString(),updatedAt:new Date().toISOString()};attempts.push(record);return res.end(JSON.stringify(record));}
    const record=attempts.find(item=>item.id===match[1]);if(!record){res.statusCode=404;return res.end('{}');}
    return res.end(JSON.stringify(match[2]?{points:0,total:1,items:[{id:pack.units[0].groups[0].questions[0].id,correct:false}]}:record));
   }
   if(url.pathname==='/api/ai/chat'){let body='';for await(const chunk of req)body+=chunk;const request=JSON.parse(body);requests.push(request);const source=JSON.parse(request.messages.at(-1).content.slice(request.messages.at(-1).content.indexOf('\n{')+1));const sourceKeys=(source.writing || []).map(item=>item.sourceKey);return res.end(JSON.stringify({content:JSON.stringify({summary:'',speaking:[],writing:[{domain:'Task 1 学术图表',collocations:['a steady increase｜图表中的持续上升'],sentencePatterns:[],sourceKeys}]})}));}
   res.statusCode=404;return res.end('{}');
  }
  const target=path.resolve(app,'.'+(url.pathname==='/'?'/index.html':url.pathname));
  if(!target.startsWith(app+path.sep)){res.statusCode=403;return res.end();}
  res.setHeader('Content-Type',({'.html':'text/html; charset=utf-8','.js':'text/javascript; charset=utf-8','.css':'text/css; charset=utf-8'})[path.extname(target)]||'application/json');res.end(await fs.readFile(target));
 }catch(error){res.statusCode=500;res.end(JSON.stringify({error:error.message}));}
});
(async()=>{
 await fs.mkdir(artifacts,{recursive:true});await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 const browser=await chromium.launch({...(process.env.ELP_BROWSER_CHANNEL==='bundled'?{}:{channel:'chrome'}),headless:true});
 try {
  const page=await browser.newPage({viewport:{width:1440,height:1000}});page.setDefaultTimeout(10000);
  const errors=[];page.on('pageerror',error=>errors.push(error.message));const base=`http://127.0.0.1:${server.address().port}`;
  await page.goto(base+'/#writing');const card=page.locator('#dailyWritingLanguage .language-recall-card').first();await card.waitFor();
  assert.match(await card.textContent(),/表达 89/);assert.equal(await card.locator('textarea').inputValue(),'');
  await card.locator('textarea').fill('I can use expression 89.');await page.locator('.nav-item[data-route="home"]').click();
  await page.reload();await page.locator('.nav-item[data-route="writing"]').click();
  assert.equal(await card.locator('textarea').inputValue(),'I can use expression 89.','unfinished draft lost');
  await card.locator('[data-reveal]').click();fail=true;await card.locator('[data-rating="good"]').click();
  await card.locator('.recall-status').filter({hasText:'保存失败'}).waitFor();assert.equal(data.languagePractice[key].rating,'again');
  fail=false;await card.locator('[data-rating="good"]').click();await card.locator('.recall-status').filter({hasText:'已记录'}).waitFor();
  assert.equal(data.languagePractice[key].dueDate,E.addDays(day,1));assert.equal(data.languagePractice[key].draftPending,false);
  await page.reload();assert.doesNotMatch(await page.locator('#dailyWritingLanguage').textContent(),/表达 89/);
  await page.locator('.nav-item[data-route="language"]').click();await page.locator('#languageTabWriting').click();await page.locator('#languageBankDetail .language-reference > summary').click();
  const archive=page.locator('.study-archive-row').filter({hasText:'expression 0'});await archive.getByRole('button',{name:'暂停学习'}).click();
  assert.equal(data.languagePractice[JSON.stringify(['writing','expression 0'])].paused,true);assert.equal(data.languageBank.writing[0].collocations.length,90);
  await page.locator('.nav-item[data-route="mistakes"]').click();
  const note=page.locator('.mistake-entry').filter({hasText:'段落依据'});await note.getByRole('button',{name:'仅保存 · 加入复习'}).click();
  assert.equal(data.mistakes.find(item=>item.id==='n1').reviewEnabled,true);
  await page.locator('[data-mistake-filter="vocabulary"]').click();await page.locator('#startVocabularyStudy').click();
  assert.equal(await page.locator('#vocabularyStudyWord').textContent(),'可持续的');assert.equal(await page.locator('#vocabularyStudyAnswer').isVisible(),false);
  await page.locator('#vocabularyStudyAttempt').fill('sustainable');await page.locator('#revealVocabularyAnswer').click();await page.locator('#vocabularyKnown').click();
  await page.locator('#vocabularyStudyCard').waitFor({state:'hidden'});assert.equal(data.mistakes.find(item=>item.id==='v1').vocabularyReview.level,1);
  await page.locator('.nav-item[data-route="home"]').click();await page.locator('[data-page="home"] [data-route="plan"]').click();
  await page.locator('#planExamDate').fill(E.addDays(day,30));await page.locator('#planDailyMinutes').fill('60');
  await page.locator('.manual-plan-options').evaluate(node=>node.open=true);
  for(const id of ['manualSpeaking','manualReading','manualListening'])await page.locator('#'+id).fill('0');
  await page.locator('#saveManualPlan').click();await page.locator('#currentPlan:not(.hidden)').waitFor();
  let plan=await page.evaluate(()=>window.ELPStudyPlan.today());assert.ok(plan.tasks.some(task=>task.kind==='writing'));assert.ok(plan.day.usedMinutes<=60);
  await page.evaluate(()=>window.ELPStudyPlan.start('writing-0'));await page.locator('#writingPrompt').fill('Describe changes in library visits.');
  await page.locator('#startWritingSession').click();await page.locator('#writingEssay').fill('Library visits increased steadily.');await page.locator('#finishWritingSession').click();
  await page.locator('#writingCompletion:not(.hidden)').waitFor();await page.waitForFunction(()=>window.ELPStudyPlan.today().progress['writing-0']);
  const historicalDays=JSON.stringify(data.learning.days),legacy=JSON.stringify(data.planProgress);
  await page.locator('.nav-item[data-route="home"]').click();await page.locator('[data-page="home"] [data-route="plan"]').click();await page.locator('#planEditor').evaluate(node=>node.open=true);
  await page.locator('#planDailyMinutes').fill('90');await page.locator('#saveManualPlan').click();
  await page.waitForFunction(()=>window.ELPStudyPlan.today().day.budgetMinutes===90);assert.equal(JSON.stringify(data.learning.days),historicalDays);assert.equal(JSON.stringify(data.planProgress),legacy);
  ai=true;await page.goto(base+'/#language');await page.reload();await page.locator('#generateLanguageBank').click();
  await until(()=>JSON.stringify(data.languageBank).includes('a steady increase'));
  assert.ok(requests.length);assert.ok(requests.every(request=>!JSON.stringify(request).includes('EXCLUDE_')),'draft or question-only source reached AI');
  assert.match(requests[0].messages[0].content,/Task 1 General/);assert.equal(data.languageBank.writing.find(item=>item.domain==='教育').collocations.length,90);
  await page.locator('.nav-item[data-route="speaking"]').click();
  const spoken=page.locator('#dailySpeakingLanguage .language-recall-card').first();
  await spoken.locator('[data-reveal]').click();await spoken.locator('[data-use]').click();
  await page.locator('#speakingPrompt').fill('What do you do in the park?');
  await page.locator('#speakingTranscript').fill('I walk in the park to get some fresh air.');
  await page.locator('#finishSpeakingPractice').click();
  await until(()=>data.speaking.some(item=>item.status==='completed'));
  assert.ok(data.learning.events.some(item=>item.type==='complete'&&item.skill==='speaking'));
  await page.locator('#speakingLanguageUse').getByRole('button',{name:'自然用到了'}).click();
  await until(()=>data.learning.events.some(item=>item.type==='language-use'&&item.skill==='speaking'&&item.rating==='good'));
  assert.ok(data.speaking.some(item=>item.languageUse?.rating==='good'));
  pack=JSON.parse(await fs.readFile(path.join(app,'starter-pack.json'),'utf8'));
  attempts=[{id:'old-reading',packId:'test-pack',unitId:pack.units[0].id,status:'submitted',revision:1,answers:{},elapsedSeconds:60,updatedAt:`${yesterday}T12:00:00Z`}];
  await page.goto(base+'/#home');await page.reload();
  await page.locator('.study-due-panel').evaluate(node=>node.open=true);
  await page.locator('#dailyReviewQueue button').filter({hasText:pack.units[0].title}).waitFor();
  assert.equal(Object.keys(data.learning.objectiveItems).length,0,'read-only hydration unexpectedly wrote the archive');
  await page.evaluate(()=>window.ELPStudyPlan.syncObjective());
  await until(()=>Object.keys(data.learning.objectiveItems).length===1);
  const objectiveState=await page.evaluate(async()=>(await(await fetch('/api/data')).json()).data.learning);
  const wrong=Object.values(objectiveState.objectiveItems)[0];assert.ok(wrong,JSON.stringify(objectiveState));assert.equal(wrong.recordId,'old-reading');
  await page.locator('.study-due-panel').evaluate(node=>node.open=true);
  const wrongButton=page.locator('#dailyReviewQueue button').filter({hasText:wrong.title});await wrongButton.click();
  await page.locator('.objective-answer').waitFor();assert.equal(await page.locator('.objective-question').count(),1);assert.equal(attempts.at(-1).reviewOf,'old-reading');
  for(const width of [1440,900,390]) {
   await page.setViewportSize({width,height:1000});
   for(const route of ['home','writing','speaking','language','mistakes','plan']) {
    await page.goto(base+'/#'+route);await page.locator(`.page.is-active[data-page="${route}"]`).waitFor();
    assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true,`${route} overflow ${width}`);
    if(width===1440||width===390)await page.screenshot({path:path.join(artifacts,`${route}-${width}.png`),fullPage:true});
   }
  }
  assert.deepEqual(errors,[]);console.log('Study loop UI passed: due language, hidden answers, draft/failure persistence, pause/archive, manual review, productive words, automatic completion, retained plan history, source boundaries, objective queue and responsive pages.');
 }finally{await browser.close();await new Promise(resolve=>server.close(resolve));}
})().catch(error=>{console.error(error);server.close();process.exitCode=1;});
