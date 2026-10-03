const assert = require('node:assert/strict');
const {validated} = require('../app/ai-client.js');
(async () => {
  const original = global.fetch;
  try {
    for (const [attempts, expected] of [[1,2],[2,1]]) {
      const budgets=[];
      global.fetch=async (_,options)=>{budgets.push(JSON.parse(options.body).format_attempts);return {ok:true,json:async()=>({attempts,content:'bad semantic sources'})};};
      await assert.rejects(validated({},()=>{throw Error('wrong source');}),/wrong source/);
      assert.equal(budgets.length,expected);
      assert.deepEqual(budgets,expected===2?[2,1]:[2]);
    }
    let calls=0;
    global.fetch=async()=>{calls++;return {ok:false,json:async()=>({error:'format exhausted'})};};
    await assert.rejects(validated({}),/format exhausted/);assert.equal(calls,1);
    calls=0;global.fetch=async()=>{calls++;throw Error('network');};
    await assert.rejects(validated({}),/network/);assert.equal(calls,1);
  } finally {global.fetch=original;}
  console.log('AI retry budget includes backend and client validation; transport failures do not retry.');
})().catch(error=>{console.error(error);process.exitCode=1;});
