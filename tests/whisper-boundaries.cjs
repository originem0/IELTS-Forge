const assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const source=fs.readFileSync('app/whisper.js','utf8');
async function run(duration,{silence=false,fail=false,cancel=false}={}) {
  const segments=[],requests=[],controller=new AbortController();
  class Audio { async decodeAudioData(){return {duration};} async close(){} }
  class Offline {
    constructor(channels,length,rate){this.segment={length,rate};segments.push(this.segment);}
    createBufferSource(){return {connect(){},start:(_,offset)=>{this.segment.offset=offset;}};}
    async startRendering(){return {getChannelData:()=>new Float32Array(this.segment.length)};}
  }
  const context={window:{AudioContext:Audio,OfflineAudioContext:Offline},OfflineAudioContext:Offline,Blob,DOMException,fetch:async(url,options)=>{
    const bytes=await options.body.arrayBuffer(),view=new DataView(bytes);
    assert.equal(url,'/api/transcription');assert.equal(view.getUint32(40,true),bytes.byteLength-44);
    assert.ok(bytes.byteLength<=480*32000+44,'request exceeded backend PCM limit');requests.push(bytes.byteLength);
    if(cancel)controller.abort();
    if(fail && requests.length===2)return {ok:false,json:async()=>({error:'engine failure'})};
    if(silence)return {ok:false,json:async()=>({error:'silent',code:'no_speech'})};
    return {ok:true,json:async()=>({text:`segment ${requests.length}`})};
  }};
  vm.runInNewContext(source,context);
  const work=context.window.localWhisper.transcribe(new Blob(['encoded audio']),controller.signal);
  if(fail)await assert.rejects(work,/engine failure/);
  else if(cancel)await assert.rejects(work,error=>error.name==='AbortError');
  else if(silence)await assert.rejects(work,/没有识别/);
  else {
    assert.equal(await work,requests.map((_,i)=>`segment ${i+1}`).join('\n'));
    assert.equal(segments.reduce((sum,s)=>sum+s.length,0),Math.ceil(duration*16000),'lost audio frames');
    for(let i=1;i<segments.length;i++)assert.equal(Math.round(segments[i].offset*16000),segments.slice(0,i).reduce((sum,s)=>sum+s.length,0),'segment gap or duplicate');
  }
  return requests.length;
}
(async()=>{
  assert.equal(await run(480),1);
  assert.equal(await run(480.01),2);
  assert.equal(await run(961),3,'background callback delay was not handled');
  await run(480.01,{fail:true});
  assert.equal(await run(480.01,{cancel:true}),1);
  await run(1,{silence:true});
  console.log('Whisper boundaries passed: exact limit, encoder tail, delayed stop, complete sample coverage, cancellation and no partial success.');
})().catch(error=>{console.error(error);process.exitCode=1;});
