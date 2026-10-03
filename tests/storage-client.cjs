const assert = require('node:assert/strict');
const {create} = require('../app/storage-client.js');

(async () => {
  const requests = [];
  let fail = false, release;
  const storage = create({request:async (path, options) => {
    requests.push(JSON.parse(options.body));
    if(release) await new Promise(resolve => {release(resolve);});
    return {ok:!fail,json:async()=>fail?{error:'fixture'}:{saved:true,revision:'next',records:{}}};
  }});
  const state = storage.adopt({writings:Array.from({length:1000},(_,i)=>({id:String(i),essay:'essay'.repeat(100)})),speaking:[{id:'voice',audio:'huge-existing-recording'}],mistakes:[],preferences:{}},{revision:'initial'});
  state.writings[20].essay = 'one changed answer';
  await storage.save();
  assert.deepEqual({metadata:requests[0].metadata,collections:requests[0].collections},{metadata:{},collections:{writings:{upsert:[{id:'20',essay:'one changed answer'}]}}});
  assert.ok(JSON.stringify(requests[0]).length<3500,'only the edited record and its concurrency base may be transmitted');
  state.writings.push({id:'new',essay:'new'});
  await storage.save();
  assert.equal(requests[1].collections.writings.upsert.length,1);
  assert.equal(requests[1].collections.writings.order.length,1001);
  state.preferences.model = 'local';
  fail=true;await assert.rejects(storage.save());assert.equal(storage.dirty,true);
  fail=false;await storage.save();assert.equal(storage.dirty,false);
  let resume;
  release=resolve=>{resume=resolve};state.writings[20].essay='older in flight';
  const pending=storage.save();await new Promise(setImmediate);
  state.writings[20].essay='newer while saving';
  release=null;resume();await pending;
  assert.equal(requests.at(-1).collections.writings.upsert[0].essay,'newer while saving');
  console.log('Incremental storage passed: 1,000-record history, single-record payload, append order, failed-write retry and concurrent edits.');
})().catch(error=>{console.error(error);process.exitCode=1});
