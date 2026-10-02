const {chromium}=require(process.env.ELP_PLAYWRIGHT_MODULE||'playwright');
const assert=require('node:assert/strict');
const {silentWave}=require('./listening-ui.cjs');
const path=require('node:path');
const fs=require('node:fs/promises');
(async()=>{
 const base=process.env.ELP_TEST_URL;
 const browser=await chromium.launch({...(process.env.ELP_BROWSER_CHANNEL==='bundled'?{}:{channel:'chrome'}),headless:true});
 try {
  const page=await browser.newPage({viewport:{width:1280,height:900}});
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  const audio=await (await page.request.post(`${base}/api/library/media`,{headers:{'Content-Type':'audio/wav'},data:silentWave()})).json();
  for(const skill of ['reading','listening']) {
   const pack={version:1,title:`Explanation ${skill}`,source:{name:'Synthetic explanation fixture',status:'generated'},units:[{id:skill,skill,part:skill==='reading'?'academic':'1',title:'Library opening day',minutes:0,prompt:'Complete the sentence.',...(skill==='reading'?{passages:[{id:'A',text:'The library opens on Monday.'}]}:{audio:audio.id,transcript:'The library opens on Monday.'}),groups:[{id:'g1',kind:'text',instruction:'ONE WORD ONLY',maxWords:1,questions:[{id:'q1',label:'1',text:'Opening day: ____',answers:['Monday']}]}]}]};
   const imported=await page.request.post(`${base}/api/library/packs`,{data:pack});assert.equal(imported.status(),201);
   await page.goto(`${base}/#${skill}/new`);
   await page.getByRole('button',{name:'开始练习',exact:true}).click();
   if(skill==='listening')await page.waitForFunction(()=>document.querySelector('.objective-audio audio')?.readyState>=1);
   await page.locator('[data-question="q1"]').fill('Tuesday');
   await page.getByRole('button',{name:'完成作答',exact:true}).click();
   await page.locator('.objective-score').waitFor();
   const score=await page.locator('.objective-score').textContent();
   await page.locator('[data-explain-question="q1"]').check();
   await page.getByRole('button',{name:'讲解所选 1 道错题',exact:true}).click();
   await page.locator('.objective-ai-explanation blockquote').waitFor();
   assert.equal(await page.locator('.objective-ai-explanation blockquote').textContent(),'The library opens on Monday.');
   assert.equal(await page.locator('.objective-score').textContent(),score);
   await page.getByRole('button',{name:'定位这段原文'}).click();
   assert.equal(await page.locator('.objective-source-disclosure').evaluate(node=>node.open),true);
   await page.getByRole('button',{name:'返回这道错题'}).click();
   await page.reload();await page.locator('.objective-ai-explanation blockquote').waitFor();
   assert.equal(await page.locator('.objective-score').textContent(),score);
   await page.setViewportSize({width:390,height:844});
   assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
   await fs.mkdir(path.resolve(__dirname,'../dist-test/ai-learning'),{recursive:true});
   await page.screenshot({path:path.resolve(__dirname,`../dist-test/ai-learning/${skill}.png`),fullPage:true});
   await page.setViewportSize({width:1280,height:900});
  }
  assert.deepEqual(errors,[]);
  console.log('Reading/listening AI explanations: selection, real disk persistence, unchanged score, evidence navigation, reload and narrow layout passed.');
 }finally{await browser.close()}
})().catch(error=>{console.error(error);process.exitCode=1});
