// Synthetic upstreams only; never reads or sends a user's API credentials.
const {chromium}=require(process.env.ELP_PLAYWRIGHT_MODULE || 'playwright');
const assert=require('node:assert/strict');
const http=require('node:http'),fs=require('node:fs/promises'),path=require('node:path');
const applyStateRequest=require('./state-api-fixture.cjs');
const root=path.resolve(__dirname,'..');
let data={writings:[],speaking:[],mistakes:[]},saved={connected:false},failDiscovery=false,failSave=false;
const requests=[];
const server=http.createServer(async(req,res)=>{
 res.setHeader('Cache-Control','no-store');
 if(req.url.startsWith('/api/')) {
  res.setHeader('Content-Type','application/json');let body={};
  if(req.method==='POST'||req.method==='PATCH'||req.method==='PUT'){let raw='';for await(const part of req)raw+=part;body=JSON.parse(raw||'{}');}
  if(req.url==='/api/data'){if(req.method==='PATCH'||req.method==='PUT')data=applyStateRequest(data,body,req.method);return res.end(JSON.stringify({data,storage:{bound:true,ready:true,directoryId:'fixture'}}));}
  if(req.url==='/api/ai/status')return res.end(JSON.stringify(saved));
  if(req.url==='/api/ai/models') {
   requests.push(body);
   if(failDiscovery){res.statusCode=401;return res.end('{"error":"fixture key denied"}');}
   return res.end(JSON.stringify({models:[{id:'gpt-relay'},{id:'claude-relay'},{id:'glm-relay'},{id:'gemini-relay'},{id:'grok-relay'},{id:'deepseek-relay'}]}));
  }
  if(req.url==='/api/ai/config'||req.url==='/api/ai/vision/config') {
   requests.push(body);
   if(failSave){res.statusCode=502;return res.end('{"error":"fixture upstream unavailable"}');}
   const value={baseUrl:body.baseUrl,model:body.model,protocol:body.protocol,provider:body.provider,connected:true,visionVerified:req.url.includes('/vision/')};
   if(req.url.includes('/vision/'))saved.vision=value;else saved={...saved,...value};saved.saved=true;
   return res.end(JSON.stringify({saved:true,connected:true,model:body.model}));
  }
  if(req.url==='/api/ai/vision/disconnect'){delete saved.vision;return res.end('{"ok":true}');}
  if(req.url==='/api/ai/disconnect'){saved={connected:false,vision:saved.vision};return res.end('{"ok":true}');}
  if(req.url==='/api/library/packs')return res.end('{"packs":[]}');
  if(req.url==='/api/library/attempts')return res.end('{"attempts":[]}');
  if(req.url==='/api/library/summary')return res.end('{"counts":{"reading":0,"listening":0},"activityDates":[]}');
  if(req.url==='/api/transcription/status')return res.end('{"ready":false}');
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
  const page=await browser.newPage({viewport:{width:1440,height:1000}});page.setDefaultTimeout(8000);
  const errors=[];page.on('pageerror',error=>errors.push(error.message));page.on('dialog',dialog=>dialog.accept());
  const base=`http://127.0.0.1:${server.address().port}`;await page.goto(`${base}/#settings`);
  await page.locator('.page.is-active[data-page="settings"]').waitFor();
  for(const prefix of ['ai','vision']) {
   assert.deepEqual(await page.locator(`#${prefix}Provider option`).evaluateAll(nodes=>nodes.map(node=>node.value)),['custom','openai','anthropic','glm','gemini','grok','deepseek']);
   for(const [provider,protocol] of [['anthropic','anthropic'],['gemini','gemini'],['openai','openai'],['glm','openai'],['grok','openai'],['deepseek','openai']]) {
    await page.locator(`#${prefix}Provider`).selectOption(provider);assert.equal(await page.locator(`#${prefix}Protocol`).inputValue(),protocol);
   }
   await page.locator(`#${prefix}Provider`).selectOption('custom');
   await page.locator(`#${prefix}BaseUrl`).fill(`https://${prefix}.example/v1`);
   await page.locator(`#${prefix}ApiKey`).fill(`${prefix}-synthetic-secret`);
   await page.locator(`#${prefix}FetchModels`).click();
   await page.locator(`#${prefix}ModelsStatus`).filter({hasText:'已获取 6'}).waitFor().catch(async error=>{console.error({prefix,errors,status:await page.locator(`#${prefix}ModelsStatus`).textContent(),result:await page.locator(`#${prefix}TestResult`).textContent(),requestCount:requests.length});throw error;});
   await page.locator(`#${prefix}Model`).selectOption(prefix==='ai'?'claude-relay':'gemini-relay');
   assert.equal(requests.at(-1).purpose,prefix==='ai'?'text':'vision');
   await page.locator(prefix==='ai'?'#aiSettings button[type="submit"]':'#visionSettings button[type="submit"]').click();
   await page.locator(`#${prefix}TestResult`).filter({hasText:'配置已在本机加密保存'}).waitFor();
   assert.equal(await page.locator(`#${prefix}ApiKey`).inputValue(),'');
   assert.equal(requests.at(-1).provider,'custom');
   assert.match(await page.locator('#imageRouteStatus').textContent(),prefix==='ai'?/文字接口的 claude-relay.*首次带图批改会先测试识图/:/独立图片接口的 gemini-relay.*识图测试已通过/);
  }
  assert.equal(saved.baseUrl,'https://ai.example/v1');assert.equal(saved.vision.baseUrl,'https://vision.example/v1');
  assert.equal(saved.protocol,'openai');assert.equal(saved.vision.protocol,'openai','model vendor changed relay protocol');
  await page.reload();await page.locator('#visionModel').filter({has:page.locator('option[value="gemini-relay"]')}).waitFor();
  assert.equal(await page.locator('#aiModel').inputValue(),'claude-relay');assert.equal(await page.locator('#visionModel').inputValue(),'gemini-relay');
  assert.equal(await page.locator('#aiApiKey').inputValue(),'');assert.equal(await page.locator('#visionApiKey').inputValue(),'');
  await page.locator('#visionFetchModels').click();await page.locator('#visionModelsStatus').filter({hasText:'已获取 6'}).waitFor();
  assert.equal(requests.at(-1).apiKey,'','browser must not read saved credentials');
  failDiscovery=true;await page.locator('#aiFetchModels').click();await page.locator('#aiModelsStatus.is-error').waitFor();assert.equal(await page.locator('#aiModel').inputValue(),'claude-relay');failDiscovery=false;
  failSave=true;await page.locator('#visionModel').selectOption('grok-relay');await page.locator('#visionSettings button[type="submit"]').click();await page.locator('#visionTestResult.is-error').waitFor();assert.equal(saved.vision.model,'gemini-relay');failSave=false;
  await page.locator('#visionModel').selectOption('gemini-relay');
  let release;const held=new Promise(resolve=>{release=resolve;});let entered;const started=new Promise(resolve=>{entered=resolve;});
  await page.route('**/api/ai/models',async route=>{entered();await held;try{await route.fulfill({contentType:'application/json',body:'{"models":[{"id":"stale-model"}]}'});}catch{}},{times:1});
  await page.locator('#aiFetchModels').click();await started;await page.locator('#aiBaseUrl').fill('https://changed.example/v1');await page.locator('#aiBaseUrl').blur();release();
  await page.waitForFunction(()=>!document.querySelector('#aiFetchModels').disabled);
  assert.equal(await page.locator('#aiModel option[value="stale-model"]').count(),0,'late model response replaced the new station');
  await page.reload();await page.locator('#aiModel option[value="claude-relay"]').waitFor({state:'attached'});
  const artifacts=path.join(root,'dist-test/ai-settings');await fs.mkdir(artifacts,{recursive:true});
  for(const width of [1440,900,390]){await page.setViewportSize({width,height:1000});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true,`AI settings overflow at ${width}px`);if(width===1440){await page.locator('#aiSettings').scrollIntoViewIfNeeded();await page.screenshot({path:path.join(artifacts,'settings.png')});}}
  await page.setViewportSize({width:1440,height:1000});await page.locator('#disconnectVision').click();await page.locator('#visionTestResult').filter({hasText:'已删除独立图片配置'}).waitFor();assert.equal(saved.connected,true);assert.equal(saved.vision,undefined);
  assert.match(await page.locator('#imageRouteStatus').textContent(),/文字接口的 claude-relay.*首次带图批改会先测试识图/);
  await page.reload();await page.locator('#aiModel option[value="claude-relay"]').waitFor({state:'attached'});
  assert.equal(await page.locator('#visionModel').inputValue(),'');
  assert.match(await page.locator('#imageRouteStatus').textContent(),/文字接口的 claude-relay/);
  await page.locator('#aiProvider').selectOption('glm');
  await page.locator('#aiBaseUrl').fill('https://glm-custom-region.example/v4');
  await page.locator('#aiFetchModels').click();await page.locator('#aiModelsStatus').filter({hasText:'已获取 6'}).waitFor();
  await page.locator('#aiModel').selectOption('glm-relay');
  await page.locator('#aiSettings button[type="submit"]').click();await page.locator('#aiTestResult').filter({hasText:'配置已在本机加密保存'}).waitFor();
  await page.reload();await page.locator('#aiModel option[value="glm-relay"]').waitFor({state:'attached'});
  assert.equal(await page.locator('#aiProvider').inputValue(),'glm','customized official endpoint lost its chosen source');
  assert.equal(await page.locator('#aiBaseUrl').inputValue(),'https://glm-custom-region.example/v4');
  assert.equal(await page.locator('#visionBaseUrl').inputValue(),'');
  saved.visionVerified=true;
  await page.locator('.nav-item[data-route="home"]').click();await page.locator('.nav-item[data-route="settings"]').click();
  await page.locator('#imageRouteStatus').filter({hasText:'识图测试已通过'}).waitFor();
  assert.deepEqual(errors,[]);console.log('AI settings browser checks passed: seven choices per form, mixed relay models, isolated credentials, reload, errors, stale discovery, independent deletion and responsive layout.');
 }finally{await browser.close();await new Promise(resolve=>server.close(resolve));}
})().catch(error=>{console.error(error);server.close();process.exitCode=1;});
