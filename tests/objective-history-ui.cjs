// Many-history layout and pagination use synthetic records, never a learner's archive.
const {chromium}=require(process.env.ELP_PLAYWRIGHT_MODULE||'playwright');
const assert=require('node:assert/strict'),http=require('node:http'),fs=require('node:fs/promises'),path=require('node:path');
const app=path.resolve(__dirname,'../app'),artifacts=path.resolve(__dirname,'../dist-test/history-layout-20261003');
const pack={id:'history-pack',version:1,title:'History fixture',source:{name:'Synthetic history',status:'generated'},units:[]};
let attempts=[];
for(const skill of ['reading','listening']) for(let i=0;i<101;i++) {
  const id=`${skill}-${i}`,title=`${skill} topic ${String(i).padStart(3,'0')}${i===0?' '+('A long question title with useful context. ').repeat(12):''}`;
  pack.units.push({id,skill,part:skill==='reading'?'academic':'1',title,prompt:'Fixture question',minutes:20,passages:[],groups:[]});
  attempts.push({id,unitId:id,packId:pack.id,revision:1,status:i%2?'submitted':'draft',mode:i%5?'practice':'simulation',answers:{},elapsedSeconds:150,createdAt:'2026-10-02T10:00:00Z',updatedAt:new Date(Date.UTC(2026,9,2,10,0,0)-i*1000).toISOString()});
}
let failDelete=false;
const server=http.createServer(async(req,res)=>{
  try {
    const url=new URL(req.url,'http://localhost');res.setHeader('Cache-Control','no-store');
    if(url.pathname.startsWith('/api/')){
      res.setHeader('Content-Type','application/json');
      if(url.pathname==='/api/data')return res.end(JSON.stringify({data:{writings:[],speaking:[],mistakes:[]},storage:{bound:true,ready:true,directoryId:'history-fixture'}}));
      if(url.pathname==='/api/ai/status')return res.end('{"connected":false}');
      if(url.pathname==='/api/library/summary')return res.end(JSON.stringify({counts:{reading:101,listening:101},activityDates:[]}));
      if(url.pathname==='/api/library/packs')return res.end(JSON.stringify({packs:[{...pack,count:202,skills:{reading:101,listening:101}}]}));
      if(url.pathname===`/api/library/packs/${pack.id}`)return res.end(JSON.stringify(pack));
      if(url.pathname==='/api/library/attempts')return res.end(JSON.stringify({attempts}));
      const match=url.pathname.match(/^\/api\/library\/attempts\/([^/]+)(\/result)?$/);
      if(match){
        if(req.method==='DELETE'){
          if(failDelete){res.statusCode=503;return res.end('{"error":"Synthetic delete failure"}');}
          attempts=attempts.filter(item=>item.id!==match[1]);return res.end('{}');
        }
        return res.end(JSON.stringify(match[2]?{items:[],points:0,total:0}:attempts.find(item=>item.id===match[1])));
      }
      res.statusCode=404;return res.end('{}');
    }
    const file=path.resolve(app,'.'+(url.pathname==='/'?'/index.html':url.pathname));
    if(!file.startsWith(app+path.sep)){res.statusCode=403;return res.end();}
    res.setHeader('Content-Type',({'.html':'text/html; charset=utf-8','.js':'text/javascript; charset=utf-8','.css':'text/css; charset=utf-8'})[path.extname(file)]||'application/json');
    res.end(await fs.readFile(file));
  }catch(error){res.statusCode=500;res.end(JSON.stringify({error:error.message}));}
});
(async()=>{
  await fs.mkdir(artifacts,{recursive:true});await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
  const browser=await chromium.launch({...(process.env.ELP_BROWSER_CHANNEL==='bundled'?{}:{channel:'chrome'}),headless:true});
  try {
    const page=await browser.newPage({viewport:{width:1440,height:900}});page.setDefaultTimeout(10000);
    const errors=[];page.on('pageerror',error=>errors.push(error.message));
    const base=`http://127.0.0.1:${server.address().port}`;
    for(const skill of ['reading','listening']){
      await page.goto(`${base}/#${skill}`);
      const history=page.locator('.is-active .objective-history'),rows=history.locator('.objective-history-row');
      await rows.first().waitFor();assert.equal(await rows.count(),10);
      assert.match(await history.locator('.objective-history-status').textContent(),/共 101 条/);
      for(const width of [1440,900,390,320]){
        await page.setViewportSize({width,height:900});
        const layout=await rows.evaluateAll(nodes=>nodes.map(node=>{
          const rect=node.getBoundingClientRect(),text=node.querySelector('h4').getBoundingClientRect();
          const [primary,remove]=[...node.querySelectorAll('.objective-history-actions button')].map(el=>el.getBoundingClientRect());
          return {height:rect.height,gap:remove.left-primary.right,inside:remove.right<=rect.right+1&&primary.left>=rect.left-1,separate:text.right<=primary.left||text.bottom<=primary.top};
        }));
        assert.ok(layout.every(item=>item.gap>=10&&item.inside&&item.separate),`${skill} controls collide at ${width}px`);
        assert.ok((await history.locator('.objective-history-toolbar .button-row').boundingBox()).height<=66,`${skill} filters waste vertical space at ${width}px`);
        if(width>=900)assert.ok(layout.every(item=>item.height<=82),`${skill} records are too tall`);
        assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1),false);
        if([1440,390].includes(width))await page.screenshot({path:path.join(artifacts,`${skill}-${width}.png`)});
      }
      await page.setViewportSize({width:1440,height:900});
      const next=history.getByRole('button',{name:'下一页',exact:true});
      const previous=history.getByRole('button',{name:'上一页',exact:true});
      assert.equal(await previous.isDisabled(),true);
      await next.click();assert.equal(await rows.first().getAttribute('data-attempt-id'),`${skill}-10`);
      await page.evaluate(()=>{location.hash='home';});await page.locator('[data-page="home"].is-active').waitFor();
      await page.evaluate(skill=>{location.hash=skill;},skill);await rows.first().waitFor();
      assert.equal(await rows.first().getAttribute('data-attempt-id'),`${skill}-10`,'returning to a module lost the page');
      const search=history.getByRole('searchbox');await search.fill('topic 099');assert.equal(await rows.count(),1);
      assert.equal(await rows.first().getAttribute('data-attempt-id'),`${skill}-99`);
      await history.locator('[data-filter="draft"]').click();assert.equal(await rows.count(),0);
      await search.fill('');assert.equal(await rows.count(),10);assert.match(await history.locator('.objective-history-status').textContent(),/共 51 条/);
      await history.locator('[data-filter="all"]').click();
      for(let i=0;i<10;i++)await next.click();
      assert.equal(await rows.count(),1);assert.equal(await next.isDisabled(),true);
      const remove=rows.first().getByRole('button',{name:/^删除练习记录/});
      page.once('dialog',dialog=>dialog.dismiss());await remove.click();assert.equal(await rows.count(),1);
      failDelete=true;page.once('dialog',dialog=>dialog.accept());await remove.click();
      await page.locator('#objectiveError').filter({hasText:'Synthetic delete failure'}).waitFor();assert.equal(await rows.count(),1);
      failDelete=false;page.once('dialog',dialog=>dialog.accept());await remove.click();
      await page.waitForFunction(()=>document.querySelectorAll('.is-active .objective-history-row').length===10);
      assert.equal(await rows.first().getAttribute('data-attempt-id'),`${skill}-90`,'deleting the last item should return to the preceding page');
      assert.match(await history.locator('.objective-history-status').textContent(),/共 100 条/);
      assert.equal(attempts.some(item=>item.id===`${skill}-100`),false);
      await page.reload();await rows.first().waitFor();assert.match(await history.locator('.objective-history-status').textContent(),/共 100 条/);
    }
    assert.deepEqual(errors,[]);
    console.log('Objective history passed: 202 records, ten-row pages, title search, filters, return state, last-page deletion, failure/cancel preservation and separated controls at four widths.');
  }finally{await browser.close();await new Promise(resolve=>server.close(resolve));}
})().catch(error=>{console.error(error);server.close();process.exitCode=1;});
