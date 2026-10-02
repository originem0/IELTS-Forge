const assert = require('node:assert/strict');

module.exports = async (page, base) => {
  for (const skill of ['reading', 'listening']) {
    await page.evaluate(skill => { location.hash = skill; }, skill);
    const rows = page.locator('.is-active .objective-history-row');
    await rows.first().waitFor();
    const before = (await (await page.request.get(`${base}/api/library/attempts`)).json()).attempts;
    const count = await rows.count();
    page.once('dialog', dialog => dialog.dismiss());
    await rows.first().getByRole('button', {name: /^删除练习记录/}).click();
    assert.equal(await rows.count(), count, 'cancel must preserve history');
    await page.route('**/api/library/attempts/*', route => route.request().method() === 'DELETE'
      ? route.fulfill({status:503, contentType:'application/json', body:JSON.stringify({error:'Synthetic delete failure'})}) : route.continue());
    page.once('dialog', dialog => dialog.accept());
    await rows.first().getByRole('button', {name: /^删除练习记录/}).click();
    await page.locator('#objectiveError').filter({hasText:'Synthetic delete failure'}).waitFor();
    assert.equal(await rows.count(), count, 'failed deletion must preserve history');
    await page.unroute('**/api/library/attempts/*');
    page.once('dialog', dialog => dialog.accept());
    const deleted = page.waitForResponse(response => response.request().method() === 'DELETE');
    await rows.first().getByRole('button', {name: /^删除练习记录/}).click();
    assert.equal((await deleted).status(), 200);
    await page.waitForFunction(expected => document.querySelectorAll('.is-active .objective-history-row').length === expected, count - 1);
    const after = (await (await page.request.get(`${base}/api/library/attempts`)).json()).attempts;
    assert.equal(after.length, before.length - 1);
    const removed = before.filter(item => !after.some(other => other.id === item.id));
    assert.equal(removed.length, 1);
    assert.equal((await page.request.get(`${base}/api/library/attempts/${removed[0].id}`)).status(), 404);
    await page.reload();
    await page.locator('.is-active .objective-summary').waitFor();
    assert.equal(await rows.count(), count - 1, 'deleted history returned after reload');
  }
};
