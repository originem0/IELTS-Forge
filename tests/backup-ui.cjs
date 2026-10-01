const assert=require('node:assert/strict');

module.exports=async function backupUI(page,base) {
  await page.setViewportSize({width:1440,height:1000});
  await page.locator('.nav-item[data-route="settings"]').click();
  const oldTab=await page.context().newPage();
  await oldTab.goto(`${base}/#settings`);
  await oldTab.locator('#storageOnboarding.hidden').waitFor({state:'attached'});
  const originalDirectory=await page.locator('#dataDirectoryPath').textContent();
  const snapshot=await page.evaluate(async()=>({
    data:(await(await fetch('/api/data')).json()).data,
    packs:(await(await fetch('/api/library/packs')).json()).packs,
    attempts:(await(await fetch('/api/library/attempts')).json()).attempts
  }));
  const downloading=page.waitForEvent('download');
  await page.locator('#exportFullBackup').click();
  const download=await downloading; const file=await download.path();
  assert.match(download.suggestedFilename(),/full-backup.*\.zip$/);
  await page.locator('.nav-item[data-route="writing"]').click();
  await page.locator('#newWriting').click();await page.locator('#writingPrompt').fill('Created after the full backup.');
  await page.waitForFunction(async()=> (await(await fetch('/api/data')).json()).data.writings.some(item=>item.prompt==='Created after the full backup.'));
  await page.locator('.nav-item[data-route="settings"]').click();
  await page.locator('#restoreFullBackup').setInputFiles(file);
  await page.locator('#backupStatus').filter({hasText:'完整备份已恢复'}).waitFor();
  const restoredDirectory=await page.locator('#dataDirectoryPath').textContent();
  assert.notEqual(restoredDirectory,originalDirectory);
  assert.match(restoredDirectory,/restored-/);
  const restored=await page.evaluate(async()=>({
    data:(await(await fetch('/api/data')).json()).data,
    packs:(await(await fetch('/api/library/packs')).json()).packs,
    attempts:(await(await fetch('/api/library/attempts')).json()).attempts
  }));
  assert.deepEqual(restored.data.writings,snapshot.data.writings,'restore lost or mixed writing records');
  assert.deepEqual(restored.data.speaking,snapshot.data.speaking,'restore lost speaking records');
  assert.deepEqual(restored.attempts,snapshot.attempts,'restore lost listening/reading history');
  assert.deepEqual(restored.packs,snapshot.packs,'restore changed question versions/order');
  await oldTab.locator('.nav-item[data-route="writing"]').click();
  await oldTab.locator('#newWriting').click();
  await oldTab.locator('#writingPrompt').fill('Stale tab must not overwrite the restored archive.');
  await oldTab.locator('#storageResult.is-error').filter({hasText:'其他页面更换'}).waitFor({state:'attached'});
  const intact=await page.evaluate(async()=> (await(await fetch('/api/data')).json()).data);
  assert.deepEqual(intact.writings,snapshot.data.writings,'old tab overwrote the restored archive');
  await oldTab.close();
  assert.equal(await page.locator('#writeDataNow').isVisible(),false,'healthy autosave must not suggest manual saves');
  await page.locator('#restoreFullBackup').setInputFiles({name:'broken.zip',mimeType:'application/zip',buffer:Buffer.from('not a zip')});
  await page.locator('#backupStatus').filter({hasText:'恢复失败'}).waitFor();
  assert.equal(await page.locator('#dataDirectoryPath').textContent(),restoredDirectory,'failed restore changed the active folder');
  await page.reload();
  await page.locator('#storageOnboarding.hidden').waitFor({state:'attached'});
  assert.equal(await page.locator('#dataDirectoryPath').textContent(),restoredDirectory,'restored binding did not survive reload');
  console.log('Full backup UI passed: download, restore into separate archive, original history, pack ordering, stale-tab rejection, invalid archive and reload.');
};
