const {create}=require('../app/storage-client.js');
const {performance}=require('node:perf_hooks');
const {mkdirSync,writeFileSync}=require('node:fs');
const {spawnSync}=require('node:child_process');
const path=require('node:path');

async function worker(mode){
  const raw={writings:Array.from({length:1000},(_,i)=>({id:String(i),essay:'A saved essay. '.repeat(150)})),speaking:Array.from({length:100},(_,i)=>({id:`voice-${i}`,audio:`data:audio/webm;base64,${'A'.repeat(65536)}`})),mistakes:[],preferences:{}};
  let bytes=0;
  const client=create({request:async(_,options)=>{bytes=Buffer.byteLength(options.body);return {ok:true,json:async()=>({revision:'fixture'})}}});
  const state=client.adopt(raw,{revision:'fixture'});
  const times=[];let peak=0;
  for(let i=0;i<25;i++){
    const started=performance.now();
    if(mode==='before'){raw.writings[0].essay=`Edited answer ${i}`;bytes=Buffer.byteLength(JSON.stringify({data:structuredClone(raw)}));}
    else{state.writings[0].essay=`Edited answer ${i}`;await client.save();}
    times.push(performance.now()-started);peak=Math.max(peak,process.memoryUsage().heapUsed);
  }
  times.sort((a,b)=>a-b);
  return {mode,requestBytes:bytes,p95PreparationMs:Number(times[Math.ceil(times.length*.95)-1].toFixed(3)),peakObservedHeapBytes:peak};
}
if(process.argv[2])worker(process.argv[2]).then(result=>console.log(JSON.stringify(result))).catch(error=>{console.error(error);process.exitCode=1;});
else{
  const samples=['before','after'].map(mode=>{const result=spawnSync(process.execPath,[__filename,mode],{encoding:'utf8'});if(result.status!==0)throw new Error(result.stderr);return JSON.parse(result.stdout);});
  const result={scope:'Client request preparation only; excludes network, disk and browser rendering. Separate Node processes, 25 samples each.',fixture:{writingRecords:1000,speakingRecords:100,embeddedMediaCharsPerSpeaking:65536},samples};
  const directory=path.resolve(__dirname,'../dist-test/performance');mkdirSync(directory,{recursive:true});writeFileSync(path.join(directory,'storage.json'),JSON.stringify(result,null,2));console.log(JSON.stringify(result,null,2));
}
