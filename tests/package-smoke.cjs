// Exercise the built Windows launcher/resources with isolated runtime data.
const assert=require('node:assert/strict');
const fs=require('node:fs/promises');
const path=require('node:path');
const {spawn,execFile}=require('node:child_process');
const {promisify}=require('node:util');
const {createHash}=require('node:crypto');
const run=promisify(execFile);
const root=path.resolve(__dirname,'..');
const packageRoot=process.env.ELP_PACKAGE_ROOT;
if(!packageRoot)throw new Error('ELP_PACKAGE_ROOT must point to the built package');
const wait=ms=>new Promise(resolve=>setTimeout(resolve,ms));
let child;
(async()=>{
  const scratch=await fs.mkdtemp(path.join(root,'dist-test/package-smoke-'));
  const config=path.join(scratch,'config');const dataDir=path.join(scratch,'data');
  await fs.mkdir(config);await fs.mkdir(dataDir);
  await fs.writeFile(path.join(config,'config.json'),JSON.stringify({dataDirectory:dataDir}));
  async function start(){
    child=spawn(path.join(packageRoot,'启动学习中心.exe'),[],{cwd:packageRoot,windowsHide:true,stdio:'ignore',env:{...process.env,ENGLISH_LEARN_PATH_CONFIG_DIR:config,ENGLISH_LEARN_PATH_NO_BROWSER:'1'}});
    let launchError;child.on('error',error=>{launchError=error;});
    for(let i=0;i<80;i++){
      if(launchError)throw launchError;if(child.exitCode!==null)throw new Error(`launcher exited: ${child.exitCode}`);
      const {stdout}=await run('netstat.exe',['-ano','-p','tcp'],{windowsHide:true});
      const match=stdout.split(/\r?\n/).map(line=>line.trim().split(/\s+/)).find(cols=>cols.at(-1)===String(child.pid)&&/^127\.0\.0\.1:\d+$/.test(cols[1]||''));
      if(match){const base=`http://${match[1]}`;try{if((await fetch(`${base}/api/health`)).ok)return base;}catch{}}
      await wait(200);
    }
    throw new Error('packaged launcher did not bind a loopback port');
  }
  async function stop(base){
    await fetch(`${base}/api/app/shutdown`,{method:'POST'});
    for(let i=0;i<40&&child.exitCode===null;i++)await wait(100);
    assert.notEqual(child.exitCode,null,'launcher failed to exit through its own API');
  }
  let base=await start();
  const info=await(await fetch(`${base}/api/app/info`)).json();assert.equal(info.version,process.env.ELP_PACKAGE_VERSION||'four-skills-preview');
  assert.equal((await(await fetch(`${base}/api/transcription/status`)).json()).ready,true,'packaged Whisper missing');
  console.log('Packaged launcher started with isolated data and bundled Whisper.');
  await new Promise((resolve,reject)=>{
    const test=spawn(process.execPath,[path.join(__dirname,'library-ui.cjs')],{cwd:root,windowsHide:true,stdio:'inherit',env:{...process.env,ELP_TEST_URL:base}});
    test.once('error',reject);test.once('exit',code=>code===0?resolve():reject(new Error(`packaged browser suite failed: ${code}`)));
  });
  const current=await(await fetch(`${base}/api/data`)).json();
  const legacyWav=process.env.ELP_WHISPER_TEST_WAV?await fs.readFile(process.env.ELP_WHISPER_TEST_WAV):Buffer.alloc(0);
  const legacyAudio=legacyWav.length?`data:audio/wav;base64,${legacyWav.toString('base64')}`:'';
  const legacyImage='data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aD1sAAAAASUVORK5CYII=';
  const legacy={...current.data,writings:[{id:'migration-writing',type:'Task 2',minutes:40,prompt:'A saved question.',promptImages:[legacyImage],essay:'A saved answer remains intact.',updatedAt:'2026-09-01T10:00:00Z'}],speaking:[{id:'migration-speaking',part:'p1',prompt:'A saved speaking topic.',transcript:'My original words.',audio:legacyAudio,duration:Math.max(0,(legacyWav.length-44)/32000),updatedAt:'2026-09-01T10:00:00Z'}],legacyOpaque:{preserved:true}};
  const saved=await fetch(`${base}/api/data`,{method:'PUT',headers:{'Content-Type':'application/json','X-ELP-Directory':current.storage.directoryId,'If-Match':current.revision},body:JSON.stringify({data:legacy})});assert.equal(saved.ok,true);
  await stop(base);base=await start();
  const loaded=await(await fetch(`${base}/api/data`)).json();
  const digest=value=>createHash('sha256').update(JSON.stringify(value)).digest('hex');
  assert.equal(digest(loaded.data),digest(legacy),'actual executable restart changed existing data or embedded media');
  console.log('Actual executable restart restored writing, speaking, plans, library binding and opaque legacy data.');
  if(process.env.ELP_WHISPER_TEST_WAV){
    const audio=await fs.readFile(process.env.ELP_WHISPER_TEST_WAV);
    const response=await fetch(`${base}/api/transcription`,{method:'POST',headers:{'Content-Type':'audio/wav'},body:audio});
    const result=await response.json();assert.equal(response.ok,true,JSON.stringify(result));assert.match(result.text.toLowerCase(),/country/);
    console.log('Packaged offline transcription passed on the bundled public speech sample.');
  }
  const {chromium}=require(process.env.ELP_PLAYWRIGHT_MODULE||'playwright');const browser=await chromium.launch({...(process.env.ELP_BROWSER_CHANNEL === 'bundled' ? {} : {channel:'chrome'}),headless:true});
  try{
    const page=await browser.newPage({viewport:{width:1440,height:1000}});await page.goto(`${base}/#home`);
    await page.waitForFunction(()=>document.getElementById('metricLibraryStatus').textContent==='含重练与模拟');
    assert.equal(await page.locator('.nav-item[data-route="reading"],.nav-item[data-route="listening"]').count(),2);
    if(legacyAudio){
      await page.locator('.nav-item[data-route="speaking"]').click();await page.locator('[data-speaking-id="migration-speaking"]').click();
      await page.waitForFunction(()=>document.getElementById('speakingPlayback').readyState>=1);
      assert.equal(await page.locator('#speakingTranscript').inputValue(),'My original words.');
    }
    await page.locator('.nav-item[data-route="writing"]').click();await page.locator('[data-writing-id="migration-writing"]').click();
    assert.equal(await page.locator('#writingPromptImagePreview img').count(),1,'legacy prompt image missing');
    assert.equal(await page.locator('#writingEssay').inputValue(),'A saved answer remains intact.');
    await page.locator('.nav-item[data-route="home"]').click();await page.locator('[data-page="home"] [data-route="plan"]').click();await page.locator('#planEditor > summary').click();await page.locator('#planDailyMinutes').fill('240');
    const planSaved=page.waitForResponse(response=>response.url().endsWith('/api/data')&&response.request().method()==='PATCH'&&response.ok());
    await page.locator('#saveManualPlan').click();await planSaved;
    await page.locator('.nav-item[data-route="home"]').click();
    await page.waitForFunction(()=>document.getElementById('metricLibraryStatus').textContent==='含重练与模拟');
    await page.screenshot({path:path.join(scratch,'packaged-home.png'),fullPage:true});
  }finally{await browser.close();}
  await stop(base);
  await fs.writeFile(path.join(scratch,'result.json'),JSON.stringify({packageRoot,version:info.version,startup:true,fullBrowserSuite:true,restartMigration:true,legacyMediaReload:true,offlineTranscription:!!process.env.ELP_WHISPER_TEST_WAV},null,2));
  console.log(`PACKAGE_SMOKE_RESULT=${path.join(scratch,'result.json')}`);
})().catch(error=>{console.error(error);process.exitCode=1;}).finally(()=>{if(child&&child.exitCode===null)child.kill();});
