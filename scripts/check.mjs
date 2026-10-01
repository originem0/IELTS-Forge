import {readdirSync} from 'node:fs';
import {spawnSync} from 'node:child_process';
const commands = [
  ...readdirSync('app').filter(name => name.endsWith('.js')).map(name => [process.execPath, ['--check', `app/${name}`]]),
  ...['static-smoke.mjs','windows-platform-smoke.mjs','speaking-writing-regression.mjs','plan-budget.cjs','practice-lifecycle.cjs','storage-client.cjs'].map(name => [process.execPath,[`tests/${name}`]]),
  ['python',['tests/question-bank-converter.py']]
];
for (const [command,args] of commands) {
  const result=spawnSync(command,args,{stdio:'inherit'});
  if (result.error) {console.error(result.error.message);process.exit(1);}
  if (result.status!==0) process.exit(result.status || 1);
}
