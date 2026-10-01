import assert from 'node:assert/strict';
import { readFile, access } from 'node:fs/promises';
const read = name => readFile(new URL(`../${name}`, import.meta.url), 'utf8');
const workflow = await read('.github/workflows/release.yml');
assert.match(workflow, /windows-2022/);
assert.doesNotMatch(workflow, /macos|build-macos/);
for (const name of ['build-macos.sh', 'build-whisper-macos.sh', 'launcher/credentials_darwin.go', 'launcher/directory_picker_darwin.go']) {
  await assert.rejects(access(new URL(`../${name}`, import.meta.url)), { code: 'ENOENT' });
}
assert.match(await read('launcher/credentials_windows.go'), /CryptProtectData/);
assert.match(await read('build-portable.ps1'), /结束学习中心/);
console.log('Windows platform checks passed.');
