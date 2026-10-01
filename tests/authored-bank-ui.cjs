// Authored "我的题目" bank: saving a typed writing/speaking prompt — and a Task 1 figure — as a
// reusable bank unit. Invoked at the end of the library browser suite with the same page/base.
const assert = require('node:assert/strict');

// A 1x1 PNG; compressImage re-encodes it to WebP in the browser before upload.
const PNG = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aD1sAAAAASUVORK5CYII=';
const authoredUnits = (page, skill) => page.evaluate(async skill => {
  const units = await window.ELPLibrary.listUnits(skill);
  return units.filter(item => item.source.name === '我的题目').map(item => item.unit);
}, skill);

module.exports = async function testAuthoredBank(page) {
  // Writing Task 2 (text only): the default type after 新建练习.
  await page.locator('.nav-item[data-route="writing"]').click();
  await page.locator('#newWriting').click();
  await page.locator('#writingPrompt').fill('Some people think children should learn history at school. Discuss.');
  await page.locator('#saveWritingToBank').click();
  await page.waitForFunction(async () => (await window.ELPLibrary.listUnits('writing')).some(item => item.source.name === '我的题目' && item.unit.part === 'Task-2'));
  const task2 = (await authoredUnits(page, 'writing')).find(unit => unit.part === 'Task-2');
  assert.match(task2.prompt, /children should learn history/, 'authored Task 2 prompt not saved');

  // Writing Task 1 Academic requires a figure; saving copies it into the library media store.
  await page.locator('#newWriting').click();
  await page.locator('[data-writing-type="Task 1 Academic"]').click();
  await page.locator('#writingPrompt').fill('The chart shows library visits by year.');
  await page.locator('#writingPromptImageInput').setInputFiles({ name: 'chart.png', mimeType: 'image/png', buffer: Buffer.from(PNG, 'base64') });
  await page.locator('#writingPromptImagePreview img').waitFor();
  await page.locator('#saveWritingToBank').click();
  await page.waitForFunction(async () => (await window.ELPLibrary.listUnits('writing')).some(item => item.source.name === '我的题目' && item.unit.part === 'Task-1-Academic' && (item.unit.images || []).length));
  const task1 = (await authoredUnits(page, 'writing')).find(unit => unit.part === 'Task-1-Academic');
  assert.ok(/^[a-f0-9]{64}\.webp$/.test(task1.images[0]), 'authored Task 1 figure was not stored as library media');
  assert.equal(await page.evaluate(id => fetch(`/api/library/media/${id}`).then(response => response.ok), task1.images[0]), true, 'authored Task 1 figure is not retrievable');

  // Speaking Part 1 (text): default part after 新建练习.
  await page.locator('.nav-item[data-route="speaking"]').click();
  await page.locator('#newSpeaking').click();
  await page.locator('#speakingPrompt').fill('Describe your hometown and explain why you like it.');
  await page.locator('#saveSpeakingToBank').click();
  await page.waitForFunction(async () => (await window.ELPLibrary.listUnits('speaking')).some(item => item.source.name === '我的题目' && item.unit.part === 'p1'));
  const speaking = (await authoredUnits(page, 'speaking')).find(unit => unit.part === 'p1');
  assert.match(speaking.prompt, /hometown/, 'authored speaking prompt not saved');
  assert.equal(speaking.images, undefined, 'speaking units must not carry images');

  // A hidden unit (how the library manager retires an authored question) is accepted by the strict
  // importer but excluded from selection, while its visible sibling still appears.
  await page.evaluate(() => window.ELPLibrary.request('packs', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({
    version: 1, title: 'Hidden fixture', source: { name: 'Hidden fixture', status: 'unverified' }, units: [
      { id: 'hidden-1', skill: 'writing', part: 'Task-2', title: 'Hidden', prompt: 'hidden prompt', minutes: 40, hidden: true },
      { id: 'shown-1', skill: 'writing', part: 'Task-2', title: 'Shown', prompt: 'shown prompt', minutes: 40 }
    ] }) }));
  const visible = await page.evaluate(() => window.ELPLibrary.listUnits('writing'));
  assert.ok(visible.some(item => item.unit.id === 'shown-1'), 'visible authored unit missing from selection');
  assert.ok(!visible.some(item => item.unit.id === 'hidden-1'), 'hidden unit leaked into selection');

  console.log('Authored-bank acceptance passed: writing text, Task 1 figure uploaded to library media, speaking, and hidden-unit filtering.');
};
