import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import vm from 'node:vm';
import { createRequire } from 'node:module';
const storageClient = createRequire(import.meta.url)('../app/storage-client.js');
const mediaClient = createRequire(import.meta.url)('../app/media-client.js');
const applyStateRequest = createRequire(import.meta.url)('./state-api-fixture.cjs');
const practiceLifecycle = createRequire(import.meta.url)('../app/practice-lifecycle.js');
const planBudget = createRequire(import.meta.url)('../app/plan-budget.js');
const studyEngine = createRequire(import.meta.url)('../app/study-engine.js');
const studyCoordinator = await readFile(new URL('../app/study-coordinator.js',import.meta.url),'utf8');

const aiSource = await readFile(new URL('../app/ai-client.js',import.meta.url),'utf8');
const reviewSource = await readFile(new URL('../app/review-workspace.js',import.meta.url),'utf8');
const assessmentSource = await readFile(new URL('../app/assessment.js',import.meta.url),'utf8');
const source = await readFile(new URL('../app/app.js', import.meta.url), 'utf8');
const timers = new Set();
function harness(initialState, localWhisper) {
  localWhisper ||= { status: async () => ({ready:true}), transcribe: async () => 'Local Whisper result.' };
  const elements = new Map();
  let persisted = structuredClone(initialState || {writings:[],speaking:[],mistakes:[]});
  let failWrites = false;
  let languageReply;
  let writeGate;
  const chatRequests = [];
  const element = selector => {
    if (!elements.has(selector)) elements.set(selector, {
      value: '', textContent: '', innerHTML: '', src: '', disabled: false,
      dataset: {}, events: {}, classList: { add() {}, remove() {}, toggle() {} },
      get children() { return new Proxy([], {get: (_, key) => /^\d+$/.test(String(key)) ? element(`${selector} child:${key}`) : 0}); },
      addEventListener(name, fn) { this.events[name] = fn; },
      querySelectorAll() { return []; }, querySelector(child) { return child === '[data-jump-mistake]' ? null : element(`${selector} ${child}`); }, pause() {}, load() {}, focus() {}, scrollIntoView() {},
      removeAttribute(name) { this[name] = ''; },
      replaceChildren() {}, append() {}, prepend() {}, remove() {}, setAttribute() {},
    });
    return elements.get(selector);
  };
  class Reader {
    async readAsDataURL(blob) {
      this.result = `data:${blob.type};base64,${Buffer.from(await blob.arrayBuffer()).toString('base64')}`;
      this.onload?.();
    }
  }
  class Recorder {
    constructor() { this.state = 'inactive'; this.mimeType = 'audio/webm;codecs=opus'; }
    start() { this.state = 'recording'; }
    stop() {
      this.state = 'inactive';
      this.ondataavailable?.({ data: new Blob(['test-audio-payload'], { type: this.mimeType }) });
      this.onstop?.();
    }
  }
  let recognitionStarts = 0;
  class Recognition {
    start() {
      recognitionStarts++;
      const result = [{ transcript: 'last sentence pending' }];
      result.isFinal = false;
      this.onresult?.({ resultIndex: 0, results: [result] });
    }
    stop() {
      queueMicrotask(() => {
        const result = [{ transcript: 'This is the final sentence.' }];
        result.isFinal = true;
        this.onresult?.({ resultIndex: 0, results: [result] });
        this.onend?.();
      });
    }
    abort() {}
  }
  const context = vm.createContext({
    fixtureLoaded: value => { persisted = JSON.parse(JSON.stringify(value)); },
    Blob, Uint8Array, atob, structuredClone, AbortController, CustomEvent, FileReader: Reader,
    MediaRecorder: Recorder, URL: { createObjectURL: () => 'blob:test-audio', revokeObjectURL() {} },
    window: { ELPStudy: studyEngine, ELPStorage: {...storageClient,create: options => storageClient.create({...options,request:(...args)=>context.fetch(...args)})}, ELPMedia: mediaClient, ELPPractice: practiceLifecycle, localWhisper, ELPPlanBudget: planBudget, SpeechRecognition: Recognition, addEventListener() {}, dispatchEvent() {} },
    navigator: { mediaDevices: { getUserMedia: async () => ({ getTracks: () => [{ stop() {} }] }) } },
    document: { querySelector: element, querySelectorAll: () => [], createElement: name => element(`created:${name}:${Math.random()}`) },
    location: { hash: '#unit-test' }, confirm: () => true,
    setTimeout(fn, ms) { const timer = setTimeout(fn, ms); timers.add(timer); return timer; }, clearTimeout,
    setInterval(fn, ms) { const timer = setInterval(fn, ms); timers.add(timer); return timer; }, clearInterval,
    fetch: async (url, options) => {
      if (url === '/api/ai/chat') {
        const request = JSON.parse(options.body);
        chatRequests.push(request);
        const languagePayload = request.output_contract === 'personal-language-bank-json-v1' ? JSON.parse(request.messages.at(-1).content.slice(request.messages.at(-1).content.indexOf('\n{') + 1)) : null;
        const languageItems = kind => (languagePayload?.[kind] || languagePayload?.batches?.flatMap(batch => batch[kind] || []) || []);
        const languageKeys = kind => languageItems(kind).flatMap(item => item.sourceKeys || [item.sourceKey]).filter(Boolean);
        const content = request.output_contract === 'personal-language-bank-json-v1'
          ? JSON.stringify(languageReply ? languageReply(languagePayload) : {summary:'My bank',speaking:languageItems('speaking').length ? [{title:'Cycling',personalCore:'I enjoy cycling with friends.',reusableTopics:['爱好'],expressions:['clear my mind｜理清思绪'],answerFrames:['One reason is...｜一个原因是……'],sourceKeys:languageKeys('speaking')}] : [],writing:languageItems('writing').length ? [{domain:'education',collocations:['equal access｜平等的机会'],sentencePatterns:['It is important to...｜……十分重要'],sourceKeys:languageKeys('writing')}] : []})
          : request.messages[0].content.startsWith('只提取 IELTS') ? JSON.stringify({description:'2000 年为 10，2010 年为 20，单位为百分比。',uncertainties:'无'}) : request.output_contract?.startsWith('review-json-v1') ? JSON.stringify({topic:'练习主题',overall:6,range:[5.5,6.5],criteria:(request.output_contract.endsWith('speaking')?['FC','LR','GRA','P']:[request.output_contract.endsWith('task1')?'TA':'TR','CC','LR','GRA']).map(code=>({code,score:code==='P'?null:6,evidence:'具体文本证据'})),overview:'表达基本清楚。',transcript:'A spoken answer.',corrections:[],improvements:[],modelAnswer:'A model answer.',modelExplanation:'保留原意。',language:{collocations:[],sentencePatterns:[]}}) : 'Synthetic feedback';
        return { ok: true, json: async () => ({ content }) };
      }
      if (url === '/api/data' && ['PUT','PATCH'].includes(options?.method)) {
        if (writeGate) await writeGate;
        if (failWrites) return { ok: false, json: async () => ({ error: 'test disk failure' }) };
        persisted = applyStateRequest(persisted,JSON.parse(options.body),options.method);
      }
      return { ok: true, json: async () => ({ storage: { bound: true, directory: 'test-data' } }) };
    }
  });
  vm.runInContext(aiSource,context);
  vm.runInContext(reviewSource,context);
  vm.runInContext(assessmentSource,context);
  vm.runInContext(studyCoordinator,context);
  vm.runInContext(source.replace(/  initialize\(\);\s*\}\)\(\);\s*$/, `
    globalThis.api = { calculateStreak, refreshTranscriptionStatus, transcribeLocalRecording, bindEvents, toggleRecording, saveWriting, saveSpeaking, loadSpeaking, newSpeaking, loadWriting, todayPlanTasks, normalizePlanDay, normalizeAiPlanDay, reviewWriting, reviewSpeaking, reviewLearnerContext, reviewTopicTitle, practiceTitle, generateLanguageBank, normalizeLanguageBank, languageBankSource,
      startCorrectionStudy, startVocabularyStudy, rateCorrection, rateVocabulary, recallDrafts,
      get correctionSession() { return correctionSession; }, get vocabularySession() { return vocabularySession; },
      deleteWritingRecord, deleteSpeakingRecord, deleteSavedRecord, renderLanguageBank, enableAi() { aiConnected = true; },
      get busy() { return recordingBusy; }, get blob() { return recordingBlob; }, get speakingPhase() { return speakingPhase; },
      get state() { return state; }, set state(value) { fixtureLoaded(value); state = persistence.adopt(normalizeState(value)); }
    };
  })();`), context);
  if (initialState) context.api.state = initialState;
  element('#speakingPart').value = 'p1';
  element('#speechLanguage').value = 'en-GB';
  context.api.bindEvents();
  return { api: context.api, element, context, chatRequests, persisted: () => persisted,
    holdWrites() { let release; writeGate = new Promise(resolve => { release = resolve; }); return () => { writeGate = null; release(); }; },
    recognitionStarts: () => recognitionStarts, failWrites: (value = true) => { failWrites = value; }, languageReply: callback => { languageReply = callback; } };
}

try {
  for (const kind of ['correction','vocabulary']) {
    const note = kind==='correction'?{id:'recall',kind:'correction',module:'writing',correction:{original:'He go.',corrected:'He goes.'}}:{id:'recall',module:'vocabulary',title:'access',text:'机会',vocabularyMode:'productive'};
    const recall = harness({mistakes:[note]});
    const start = kind==='correction'?'startCorrectionStudy':'startVocabularyStudy';
    const session = `${kind}Session`, input = `#${kind}StudyAttempt`;
    recall.api[start]('recall');
    recall.element(input).events.input({target:{value:'Unfinished memory answer'}});
    assert.equal(await recall.api.recallDrafts.flush(),true);
    const restarted = harness(recall.persisted());
    restarted.api[start]('recall');
    assert.equal(restarted.api[session].draft,'Unfinished memory answer','restart lost unfinished recall');
    assert.equal(restarted.api[session].revealed,false,'restart exposed reference answer');
    restarted.api[session].revealed=true;
    restarted.failWrites();
    const rate = () => kind==='correction'?restarted.api.rateCorrection('good'):restarted.api.rateVocabulary(true);
    await rate();
    assert.equal(restarted.api.state.mistakes[0].recallDraft.text,'Unfinished memory answer','failed rating discarded draft');
    restarted.failWrites(false);
    await rate();
    assert.equal(restarted.persisted().mistakes[0].recallDraft,undefined,'completed recall retained visible draft');
  }
  for (const mode of ['legacy','paused','running']) {
    const timer = harness({writings:[{id:'timer',type:'Task 2',minutes:40,prompt:'Question',essay:'Saved answer',status:'running',elapsedSeconds:600,...(mode==='legacy'?{}:{timerState:{elapsedMilliseconds:600000,anchor:mode==='running'?Date.now()-60000:null,paused:mode==='paused'}})}]});
    await timer.api.loadWriting('timer');
    assert.match(timer.element('#writingTimer').textContent,mode==='running'?/^28:5\d$|^29:00$/:/^30:00$/,'restored writing timer lost elapsed time');
    assert.equal(timer.element('#writingEssay').readOnly,mode!=='running');
    if (mode==='running') await timer.element('#toggleTimer').events.click();
    else await timer.api.saveWriting({silent:true});
    assert.ok(timer.persisted().writings[0].elapsedSeconds>=600);
    assert.equal(timer.persisted().writings[0].timerState.anchor,null);
    assert.equal(timer.persisted().writings[0].timerState.paused,true);
  }
  for (const kind of ['writing','speaking']) {
    const collection = kind === 'writing' ? 'writings' : 'speaking';
    const record = {id:'delete-me',type:'Task 2',minutes:40,part:'p1',prompt:'Question',essay:'Keep my answer',transcript:'Keep my answer',updatedAt:new Date().toISOString()};
    const deletion = harness({[collection]:[record]});
    const load = kind === 'writing' ? deletion.api.loadWriting : deletion.api.loadSpeaking;
    const remove = kind === 'writing' ? deletion.api.deleteWritingRecord : deletion.api.deleteSpeakingRecord;
    const save = kind === 'writing' ? deletion.api.saveWriting : deletion.api.saveSpeaking;
    const answer = deletion.element(kind === 'writing' ? '#writingEssay' : '#speakingTranscript');
    await load(record.id);
    deletion.failWrites();
    assert.equal(await remove(record.id),false);
    assert.equal(deletion.api.state[collection][0].id,record.id,'failed deletion lost record');
    assert.equal(answer.value,'Keep my answer','failed deletion cleared editor');
    assert.equal(deletion.persisted()[collection][0].id,record.id);
    deletion.failWrites(false);
    const release = deletion.holdWrites();
    const pending = remove(record.id);
    await new Promise(setImmediate);
    assert.equal(answer.value,'Keep my answer','editor cleared before commit acknowledgement');
    assert.equal(await save({silent:true}),false,'autosave ran during deletion');
    release();
    assert.equal(await pending,true);
    assert.equal(deletion.persisted()[collection].length,0);
    assert.equal(answer.value,'');
    await save({silent:true});
    assert.equal(deletion.persisted()[collection].length,0,'late autosave resurrected record');
  }
  const noteDelete = harness({mistakes:[{id:'note',text:'keep'}],studyPlan:{summary:'keep'}});
  noteDelete.failWrites();
  assert.equal(await noteDelete.api.deleteSavedRecord('mistakes','note',null,()=>{}),false);
  assert.equal(noteDelete.api.state.mistakes[0].id,'note');
  await noteDelete.element('#deletePlan').events.click();
  assert.equal(noteDelete.api.state.studyPlan.summary,'keep');
  assert.equal(noteDelete.persisted().studyPlan.summary,'keep');

  const languageFixture = {writings:[{id:'new-answer',type:'Task 2',prompt:'Education',essay:'Education helps people learn.',review:'Feedback'}],speaking:[],languageBank:{summary:'existing',speaking:[],writing:[{domain:'教育',collocations:['equal access｜平等的机会'],sentencePatterns:[],sourceKeys:['old']}],sourceKeys:['old']}};
  for (const mode of ['empty','foreign','unattributed','disk-failure','no-content','valid']) {
    const extraction = harness(languageFixture);
    extraction.api.enableAi();
    extraction.languageReply(payload => {
      const keys = payload.writing.map(item => item.sourceKey);
      if (mode === 'empty') return {summary:'',speaking:[],writing:[]};
      if (mode === 'no-content') return {summary:'',speaking:[],writing:[],noContentSources:keys.map(sourceKey=>({sourceKey,reason:'内容只有题目复述，没有可复用表达。'}))};
      return {summary:'',speaking:[],writing:[{domain:'教育',collocations:['access to learning｜学习机会'],sentencePatterns:[],sourceKeys:mode==='foreign'?['not-in-batch']:mode==='unattributed'?[]:keys}]};
    });
    if (mode === 'disk-failure') extraction.failWrites();
    await extraction.api.generateLanguageBank();
    if (['empty','foreign','unattributed','disk-failure'].includes(mode)) {
      assert.equal(JSON.stringify(extraction.api.state.languageBank),JSON.stringify(languageFixture.languageBank),`${mode} changed the live bank`);
      assert.equal(JSON.stringify(extraction.persisted().languageBank),JSON.stringify(languageFixture.languageBank),`${mode} consumed source eligibility`);
    } else {
      assert.equal(extraction.persisted().languageBank.sourceKeys.length,2,`${mode} did not persist verified source`);
      if (mode==='no-content') assert.equal(extraction.persisted().languageBank.noContentSources.length,1);
    }
  }
  const legacyBank = harness({languageBank:{summary:'legacy',speaking:[],writing:[{domain:'education',collocations:['equal access'],sentencePatterns:[]}],sourceKeys:[]}});
  const bankBefore = JSON.stringify(legacyBank.api.state.languageBank);
  legacyBank.api.renderLanguageBank();
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(JSON.stringify(legacyBank.api.state.languageBank), bankBefore, 'render migrated the live archive');
  assert.equal(JSON.stringify(legacyBank.persisted().languageBank), bankBefore, 'render wrote the archive');

  let unsafeTranscriptionCalls = 0;
  const unsavedRecording = harness(undefined, {status:async()=>({ready:true}),transcribe:async()=>{unsafeTranscriptionCalls++;return 'unexpected';}});
  await unsavedRecording.api.refreshTranscriptionStatus();
  await unsavedRecording.api.toggleRecording();
  unsavedRecording.failWrites();
  await unsavedRecording.api.toggleRecording();
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(unsafeTranscriptionCalls, 0, 'Whisper started before audio was durable');
  assert.equal(await unsavedRecording.api.blob.text(), 'test-audio-payload');
  assert.match(unsavedRecording.element('#recordHint').textContent, /尚未写入磁盘/);

  let completeTranscription;
  const offline = harness(undefined, {status: async () => ({ready:true}), transcribe: () => new Promise(resolve => {completeTranscription = resolve;})});
  await offline.api.refreshTranscriptionStatus();
  assert.match(offline.element('#transcriptionStatus').textContent, /录音与转写已就绪/);
  await offline.api.toggleRecording();
  assert.equal(offline.recognitionStarts(), 0, 'offline capture must not invoke browser speech services');
  await offline.api.toggleRecording();
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(offline.api.busy, true);
  assert.equal(offline.element('#speakingTranscript').disabled, true);
  assert.match(offline.persisted().speaking[0].audio, /^data:audio/, 'audio must reach storage before Whisper resolves');
  assert.equal(offline.persisted().speaking[0].transcriptionStatus, 'pending');
  const interruptedRecording = harness(offline.persisted());
  await interruptedRecording.api.loadSpeaking(offline.persisted().speaking[0].id);
  assert.equal(await interruptedRecording.api.blob.text(), 'test-audio-payload', 'restart during transcription lost audio');
  completeTranscription('Offline result.');
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(offline.api.busy, false);
  assert.equal(offline.element('#speakingTranscript').value, 'Offline result.');
  assert.equal(await offline.api.saveSpeaking(), true);
  assert.equal(offline.persisted().speaking[0].transcript, 'Offline result.');
  assert.equal(offline.persisted().speaking[0].transcriptionStatus, 'completed');
  assert.match(offline.persisted().speaking[0].audio, /^data:audio/);
  const pending = offline.api.transcribeLocalRecording();
  await offline.api.newSpeaking();
  completeTranscription('Late obsolete result');
  await pending;
  assert.equal(offline.element('#speakingTranscript').value, '', 'late transcription must not overwrite a new practice');

  const part2 = harness();
  await part2.api.refreshTranscriptionStatus();
  part2.element('#speakingPart').value = 'p2';
  part2.element('#speakingPart').events.change();
  await part2.api.toggleRecording();
  assert.equal(part2.api.speakingPhase, 'preparing', 'Part 2 must begin with an unrecorded preparation phase');
  assert.equal(part2.element('#recordButton').textContent, '立即开始 2 分钟回答');
  await part2.api.toggleRecording();
  assert.equal(part2.api.speakingPhase, 'recording', 'Part 2 must enter the answer recording after preparation');
  await part2.api.toggleRecording();
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(part2.element('#speakingTranscript').value, 'Local Whisper result.');

  const h = harness();
  const localDate = date => `${date.getFullYear()}-${String(date.getMonth()+1).padStart(2,'0')}-${String(date.getDate()).padStart(2,'0')}`;
  const now = new Date(); const yesterday = new Date(now); yesterday.setDate(yesterday.getDate()-1);
  h.api.state.activityDates = [localDate(now), localDate(yesterday)];
  assert.equal(h.api.calculateStreak(), 2, 'streak must compare local calendar dates rather than UTC midnights');
  assert.equal(h.api.reviewTopicTitle('- **主题**: 儿童成长环境的选择，属于社会与教育类话题。'), '儿童成长环境的选择');
  assert.equal(h.api.reviewTopicTitle('主题：城市与乡村的儿童成长'), '城市与乡村的儿童成长');
  assert.equal(h.api.practiceTitle({type:'Task 2', prompt:'A specific question'}), 'A specific question');
  assert.equal(h.api.practiceTitle({type:'Task 2', review:'- **主题**: 儿童成长环境的选择，属于社会与教育类话题。'}), '儿童成长环境的选择');
  assert.equal('listening' in h.api.state, false, 'fresh state must not create retired libraries');
  assert.equal('reading' in h.api.state, false);
  assert.deepEqual(Object.keys(h.api.normalizePlanDay({ listening: 9, reading: 9, writing: 5, speaking: 10 })), ['writing', 'speaking', 'reading', 'listening', 'writingReview', 'writingRewrite', 'speakingReview', 'readingReview', 'listeningReview', 'languageMinutes', 'reviewMinutes', 'note']);
  assert.equal(h.api.normalizePlanDay({writing:5}).writing, 1, 'new writing volume must be capped');
  assert.equal(h.api.normalizePlanDay({speaking:10}).speaking, 2, 'new speaking volume must be capped');
  assert.equal(JSON.stringify(h.api.normalizeAiPlanDay({writing:1,speaking:1})), JSON.stringify({writing:1,speaking:1,reading:0,listening:0,writingReview:1,writingRewrite:1,speakingReview:1,readingReview:0,listeningReview:0,languageMinutes:10,reviewMinutes:0,note:''}), 'AI output must preserve old plans while adding only requested skills');
  h.api.state = { listening: [{ id: 'keep-legacy' }], reading: [{ id: 'keep-old' }], writings: [], speaking: [] };
  await h.api.refreshTranscriptionStatus();
  await h.api.toggleRecording();
  assert.equal(h.recognitionStarts(), 0, 'browser speech recognition must never start');
  assert.match(h.element('#speakingSaveStatus').textContent, /结束转写后自动保存/);
  await h.api.toggleRecording();
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(h.api.busy, false);
  assert.equal(h.element('#speakingTranscript').value, 'Local Whisper result.');
  assert.equal(await h.api.saveSpeaking(), true);
  const saved = h.persisted().speaking[0];
  assert.equal(h.persisted().listening[0].id, 'keep-legacy', 'normal saves must preserve opaque old data');
  assert.equal(h.persisted().reading[0].id, 'keep-old');
  assert.match(saved.audio, /^data:audio\/webm;codecs=opus;base64,/);
  assert.equal(saved.transcript, 'Local Whisper result.');

  const reloaded = harness(h.persisted());
  await reloaded.api.loadSpeaking(saved.id);
  assert.equal(await reloaded.api.blob.text(), 'test-audio-payload', 'history must restore the audio bytes');
  assert.equal(reloaded.element('#speakingTranscript').value, saved.transcript);
  assert.equal(await reloaded.api.saveSpeaking(), true, 'resaving history must retain its audio');
  assert.equal(reloaded.persisted().speaking[0].audio, saved.audio);
  reloaded.failWrites();
  assert.equal(await reloaded.api.saveSpeaking(), false, 'disk failure must not report success');

  const legacy = harness({ speaking: [{ id: 'legacy', transcript: 'Old text', prompt: '', duration: 2 }] });
  await legacy.api.loadSpeaking('legacy');
  assert.equal(legacy.api.blob, null);
  assert.equal(legacy.element('#speakingTranscript').value, 'Old text');

  for (const [type, minutes] of [['Task 1 Academic', '20'], ['Task 1 General', '20'], ['Task 2', '40']]) {
    h.element('#writingType').value = type;
    h.element('#writingType').events.change();
    assert.equal(h.element('#writingMinutes').value, minutes);
    assert.equal(h.element('#writingTimer').textContent, `${minutes}:00`);
  }
  h.element('#writingType').value = '自由写作';
  h.element('#writingType').events.change();
  assert.equal(h.element('#writingMinutes').value, '0');
  assert.equal(h.element('#writingTimer').textContent, '00:00');
  assert.equal(h.element('#toggleTimer').textContent, '开始正计时');
  h.api.state.writings.push({ id: 'custom', type: 'Task 1 Academic', minutes: 60, prompt: 'Saved', essay: '', updatedAt: new Date().toISOString() });
  await h.api.loadWriting('custom');
  assert.equal(h.element('#writingMinutes').value, '60', 'history must preserve the saved duration');
  h.api.state.studyPlan = { profile: { currentLevel: '写作 5.5，口语 6.0', targetLevel: '写作 7.0，口语 7.5', focus: '论证与自然表达' } };
  h.api.enableAi();
  h.element('#writingEssay').value = 'A short practice essay.';
  await h.api.reviewWriting();
  await h.api.reviewSpeaking();
  assert.equal(h.chatRequests.length, 2);
  for (const request of h.chatRequests) {
    assert.match(request.messages[1].content, /现有水平（用户自述）：写作 5.5，口语 6.0/);
    assert.match(request.messages[1].content, /目标水平（用户设定）：写作 7.0，口语 7.5/);
    assert.match(request.messages[1].content, /论证与自然表达/);
    assert.doesNotMatch(request.messages[0].content, /6.0–6.5/);
    assert.doesNotMatch(request.messages[0].content, /批改原则补充|写作修订规则|口语修订规则/);
  }
  assert.match(h.chatRequests[0].messages[0].content, /只有客观、明确/);
  assert.match(h.chatRequests[0].messages[0].content, /可选优化建议/);
  assert.match(h.chatRequests[1].messages[0].content, /本地 Whisper/);
  assert.match(h.chatRequests[1].messages[1].content, /完整题目 \/ 题卡/);
  assert.match(h.chatRequests[1].messages[1].content, /Part 1/);
  assert.equal(h.chatRequests[1].output_contract, 'review-json-v1-speaking');
  const vision = harness({ writings: [{ id: 'vision', type: 'Task 1 Academic', minutes: 20, prompt: 'Describe the chart.', promptImages: ['data:image/webp;base64,AAAA'], essay: 'The chart changes.', updatedAt: new Date().toISOString() }], speaking: [] });
  await vision.api.loadWriting('vision');
  vision.api.enableAi();
  await vision.api.reviewWriting();
  assert.equal(vision.chatRequests.length, 1);
  assert.equal(Array.isArray(vision.chatRequests[0].messages[1].content), true, 'image review must use multimodal content blocks');
  assert.equal(vision.chatRequests[0].messages[1].content[0].type, 'text');
  assert.equal(vision.chatRequests[0].messages[1].content[1].type, 'image_url');
  assert.equal(vision.chatRequests[0].messages[1].content[1].image_url.url, 'data:image/webp;base64,AAAA');
  assert.equal(JSON.stringify(vision.chatRequests[0]).includes('The chart changes.'), false, 'extraction must not send essay');
  assert.equal(vision.api.state.writings[0].review, '');
  assert.ok(vision.persisted().writings[0].chartExtraction.text.includes('百分比'));
  await vision.element('#confirmWritingChart').events.click();
  assert.equal(vision.chatRequests.length, 2);
  assert.equal(vision.chatRequests[1].output_contract, 'review-json-v1-writing-task1');
  assert.equal(vision.chatRequests[1].messages.every(message => typeof message.content === 'string'), true, 'confirmed review must use text connection');
  assert.match(vision.chatRequests[1].messages.at(-1).content, /2000 年为 10/);
  assert.ok(vision.persisted().writings[0].reviewInput.chartConfirmedAt);
  await h.api.generateLanguageBank();
  assert.equal(h.chatRequests.at(-1).output_contract, 'personal-language-bank-json-v1');
  assert.equal(h.api.state.languageBank.speaking[0].title, 'Cycling');
  assert.equal(h.api.state.languageBank.writing[0].domain, '教育');
  assert.match(h.api.state.languageBank.writing[0].collocations[0], /｜平等的机会/);
  assert.match(h.chatRequests.at(-1).messages[0].content, /中文翻译/);
  const translationUpgrade = harness({ writings: [], speaking: [], languageBank: { summary:'Legacy', speaking:[], writing:[{domain:'education',collocations:['equal access'],sentencePatterns:[]}], sourceKeys:[] } });
  translationUpgrade.api.enableAi();
  await translationUpgrade.api.generateLanguageBank();
  assert.equal(translationUpgrade.chatRequests.length, 1, 'legacy translation enrichment runs only after the user explicitly updates the bank');
  assert.equal(translationUpgrade.api.state.languageBank.writing[0].collocations[0], 'equal access｜平等的机会', 'translation enrichment supplements the existing English item instead of duplicating it');
  const manyRecords = harness({
    speaking: Array.from({length: 40}, (_, index) => ({id:`s${index}`, part:'p1', prompt:`Question ${index} ${'q'.repeat(1200)}`, transcript:`Answer ${index} ${'a'.repeat(3000)}`, review:'f'.repeat(1800), updatedAt:new Date(2026, 0, index + 1).toISOString()})),
    writings: Array.from({length: 30}, (_, index) => ({id:`w${index}`, type:'Task 2', prompt:`Writing ${index} ${'q'.repeat(1200)}`, essay:`Essay ${index} ${'e'.repeat(3500)}`, review:'f'.repeat(1800), updatedAt:new Date(2026, 1, index + 1).toISOString()}))
  });
  const completeSource = manyRecords.api.languageBankSource();
  assert.equal(completeSource.speaking.length, 40, 'language-bank source must retain every speaking record');
  assert.equal(completeSource.writing.length, 30, 'language-bank source must retain every writing record');
  assert.match(completeSource.speaking[0]?.question || '', /Question 39/, 'the newest speaking records must be retained first');
  assert.match(completeSource.writing[0]?.question || '', /Writing 29/, 'the newest writing records must be retained first');
  assert.equal(completeSource.writing[0].answer.length, 'Essay 29 '.length + 3500, 'a complete writing answer must not be truncated before staged extraction');
  h.api.state.studyPlan = null;
  assert.match(h.api.reviewLearnerContext('写作'), /现有水平（用户自述）：未提供/);
  assert.match(h.api.reviewLearnerContext('写作'), /目标水平（用户设定）：未提供/);
  const autosave = harness();
  autosave.element('#writingEssay').value = 'Autosaved writing draft.';
  autosave.element('#writingEssay').events.input();
  await new Promise(resolve => setTimeout(resolve, 750));
  assert.equal(autosave.persisted().writings[0].essay, 'Autosaved writing draft.');
  autosave.element('#speakingTranscript').value = 'Autosaved speaking draft.';
  autosave.element('#speakingTranscript').events.input();
  await new Promise(resolve => setTimeout(resolve, 750));
  assert.equal(autosave.persisted().speaking[0].transcript, 'Autosaved speaking draft.');
  console.log('Speaking capture/transcript/disk persistence/reload/failure and writing timer regression tests passed.');
} finally {
  for (const timer of timers) { clearTimeout(timer); clearInterval(timer); }
}
