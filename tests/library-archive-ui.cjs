// Validate the actual private ZIP through the public import UI on isolated data.
const {chromium} = require(process.env.ELP_PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const base = process.env.ELP_TEST_URL;
const archive = process.env.ELP_QUESTION_BANK_ZIP;
if (!base || !archive) throw Error('ELP_TEST_URL and ELP_QUESTION_BANK_ZIP are required');
(async () => {
  const browser = await chromium.launch({...(process.env.ELP_BROWSER_CHANNEL === 'bundled' ? {} : {channel:'chrome'}), headless:true});
  const output = path.resolve(__dirname, '../dist-test/final-archive');
  await fs.mkdir(output, {recursive:true});
  try {
    const page = await browser.newPage({viewport:{width:1440,height:1000}});
    page.setDefaultTimeout(120000);
    const errors = []; page.on('pageerror', error => errors.push(error.message));
    await page.goto(base+'/#library');
    await page.locator('#libraryFiles').setInputFiles(archive);
    await page.locator('#libraryPreview:not(.hidden)').waitFor();
    await page.locator('#importLibrary').click();
    await page.locator('#librarySuccess:not(.hidden)').waitFor();
    const first = await page.evaluate(() => window.ELPLibrary.request('packs'));
    const counts = await page.evaluate(async () => {
      const result={}; for(const skill of ['reading','listening','writing','speaking']) result[skill]=(await window.ELPLibrary.listUnits(skill)).length;
      return result;
    });
    const general = await page.evaluate(async () => (await window.ELPLibrary.listUnits('writing')).filter(item=>item.unit.part==='Task-1-General'));
    assert(general.length > 0, 'General letters missing');
    await page.locator('.nav-item[data-route="writing"]').click();
    await page.locator('#newWriting').click();
    await page.locator('#pickWritingQuestion').click();
    await page.locator('#writingQuestionPicker [data-part="Task-1-General"]').click();
    assert.equal(await page.locator('#writingQuestionPicker .question-picker-row').count(), Math.min(30,general.length));
    const row=page.locator('#writingQuestionPicker .question-picker-row').first();
    await row.locator('summary').click();
    await row.getByRole('button',{name:'使用本题'}).click();
    assert.equal(await page.locator('#writingType').inputValue(),'Task 1 General');
    assert.equal(await page.locator('#writingMinutes').inputValue(),'20');
    assert.match(await page.locator('#writingPrompt').inputValue(),/Write a letter/);
    assert.match(await page.locator('#writingPrompt').inputValue(),/150 words/);
    assert.equal(await page.locator('#writingPromptImagePreview img').count(),0);
    await page.screenshot({path:path.join(output,'general-writing.png')});
    // The parallel speaking picker must retain its Part controls and full cards.
    await page.locator('.nav-item[data-route="speaking"]').click();
    await page.locator('#newSpeaking').click();
    await page.locator('#pickSpeakingQuestion').click();
    await page.locator('#speakingQuestionPicker [data-part="p2"]').click();
    assert(await page.locator('#speakingQuestionPicker .question-picker-row').count()>0);
    const audio = await page.evaluate(async () => {
      const units=await window.ELPLibrary.listUnits('listening'), results=[];
      for(const {unit} of units) {
        const element=new Audio('/api/library/media/'+unit.audio);
        element.preload='metadata';
        try {
          await new Promise((resolve,reject)=>{const timeout=setTimeout(()=>reject(Error('audio timeout '+unit.id)),15000);element.onloadedmetadata=()=>{clearTimeout(timeout);resolve();};element.onerror=()=>{clearTimeout(timeout);reject(Error('audio unreadable '+unit.id));};});
          if(!Number.isFinite(element.duration)||element.duration<=0)throw Error('invalid duration '+unit.id);
          results.push({id:unit.id,duration:element.duration});
        } finally {element.pause();element.removeAttribute('src');element.load();}
      }
      return results;
    });
    assert.equal(audio.length,counts.listening);
    await page.goto(base+'/#library');
    await page.locator('#libraryFiles').setInputFiles(archive);
    await page.locator('#libraryPreview:not(.hidden)').waitFor();
    await page.locator('#importLibrary').click();
    await page.locator('#librarySuccess').filter({hasText:'无需重复导入'}).waitFor();
    assert.equal((await page.evaluate(()=>window.ELPLibrary.request('packs'))).packs.length,first.packs.length);
    await page.reload(); await page.locator('#libraryFiles').waitFor();
    await page.setViewportSize({width:390,height:844});
    assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1));
    await page.screenshot({path:path.join(output,'library-mobile.png')});
    assert.deepEqual(errors,[]);
    const result={packs:first.packs.length,counts,generalLetters:general.length,audioLoaded:audio.length,duplicateImportSkipped:true,errors};
    await fs.writeFile(path.join(output,'result.json'),JSON.stringify(result,null,2));
    console.log(JSON.stringify(result));
  } finally {await browser.close();}
})().catch(error=>{console.error(error);process.exitCode=1;});
