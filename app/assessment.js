(function(root,factory){const api=factory();if(typeof module==="object"&&module.exports)module.exports=api;if(root)root.ELPAssessment=api;})(typeof window==="object"?window:null,()=>{
 "use strict";
  const IELTS_WRITING_SCORING_GUIDE = `评分必须依据 IELTS 官方公开 Writing Band Descriptors（Task 1 使用 Task Achievement；Task 2 使用 Task Response；两者都使用 Coherence & Cohesion、Lexical Resource、Grammatical Range & Accuracy），不得凭“感觉不错”笼统给分。
四项分别独立判断，分数只用 0.5 递进。只有原稿充分符合某一整数档的正向特征时才给到该档；介于相邻档时给半分。单篇暂定总分取四项平均并按 0.5 档呈现，不把用户目标分当作评分依据。
分档校准：5 分通常是任务覆盖或观点发展不充分、组织不完全顺畅、词汇和结构范围有限且错误可能影响阅读；6 分能回应主要要求并有总体连贯性，词汇基本够用，能混用简单和复杂结构，错误存在但通常不妨碍理解；7 分覆盖任务要求，Task 1 有清楚 Overview 和恰当分组或 Task 2 有清晰且有发展的立场，逻辑推进明确，词汇有一定灵活与准确度，复杂结构有变化且无错句较常见；8 分回应充分且发展良好，信息易于跟随，词汇宽广而精确，多数句子无错；9 分只用于完全、深入、自然且几乎无失误的作答。不得仅因使用生僻词或长句抬高 LR/GRA。
字数不足没有独立的固定扣分值：只在它实际导致任务覆盖、展开或语言证据不足时反映到对应项目，并在证据中明确指出。`;

  const IELTS_SPEAKING_SCORING_GUIDE = `评分必须依据 IELTS 官方公开 Speaking Band Descriptors：Fluency & Coherence、Lexical Resource、Grammatical Range & Accuracy、Pronunciation。分数只用 0.5 递进；只有回答充分符合某整数档的正向特征时才给到该档，介于相邻档时给半分。
分档校准：5 分通常能继续表达但依赖重复、自我修正或慢速，词汇灵活度有限，复杂结构范围有限且错误多；6 分愿意并能够作较长表达，偶有停顿、重复或自我修正导致连贯受损，词汇足以展开并通常能改述，简单和复杂结构混用且错误通常不妨碍交流；7 分能较自然地持续表达，少量犹豫不破坏连贯，能灵活讨论多种话题并有效改述，复杂结构较灵活且无错句较常见；8 分表达流畅，重复或自我修正很少，话题展开连贯，词汇宽广灵活且含义精确，结构广泛并且多数句子无错；9 分只用于全程自然、精确、灵活且几乎无失误的表现。
官方口语分数依据三个 Part 的整体表现。当前只有一条文字转写，因此只能给非官方、基于转写的暂定估分；没有音频绝不能臆测 Pronunciation，也不能把 ASR 标点和大小写当作错误。`;

  function writingTaskAssessment(type) {
    if (type === "Task 1 Academic") return "Task 1 Academic：建议 20 分钟，至少 150 个英文词。重点核对是否准确选择并突出关键特征，是否有清楚 Overview，是否合理分组并用题目中的数据、单位和比较支持描述；不得要求个人观点或结论。";
    if (type === "Task 1 General") return "Task 1 General Training：建议 20 分钟，至少 150 个英文词。重点核对写信目的是否清楚、所有 bullet points 是否覆盖并展开、格式与语气是否符合收件人和情境。";
    if (type === "Task 2") return "Task 2：建议 40 分钟，至少 250 个英文词；正式考试中权重为 Task 1 的两倍。重点核对是否回答题目所有部分、立场是否清晰且贯穿全文、主要观点是否充分解释并用相关例子或结果支持。";
    return "自由写作：不套用 Task 1 或 Task 2 的字数和任务完成要求；如需给分，只能按语言、组织和用户明确目的谨慎评价。";
  }

  function speakingPartAssessment(part, duration) {
    const durationText = Number(duration) > 0 ? `本次录音 ${Number(duration)} 秒。` : "本次录音时长未记录，不能判断是否达到时长要求。";
    if (part === "p1") return `Part 1：正式环节约 4–5 分钟并包含多个熟悉话题；当前按单题练习评价。检查是否直接回答日常或个人问题并自然补充理由或细节，不因答案不够长篇而扣分。${durationText}`;
    if (part === "p2") return `Part 2：1 分钟准备后进行 1–2 分钟个人长陈述。逐项核对题卡提示，检查能否持续展开、按逻辑组织并在接近 2 分钟内完成；少于 60 秒必须明确提示展开不足，超过 120 秒提示正式考试会被考官叫停。${durationText}`;
    if (part === "p3") return `Part 3：正式环节约 4–5 分钟，围绕 Part 2 相关的更一般、抽象问题深入讨论。当前按单题练习评价，检查是否解释和论证观点，并能分析、比较、推测或讨论影响。${durationText}`;
    return `自由表达：不套用 Part 1、2、3 的任务时长，只按表达目的和可观察到的语言证据谨慎反馈。${durationText}`;
  }

  function reviewFormatContract(kind) {
    return `只输出 JSON 对象，不要 Markdown、代码块或其他文字。此格式要求取代前文的标题和表格要求，评分与纠错原则仍然适用。结构为 {topic:string,overall:number,range:number[],criteria:[{code:string,score:number|null,evidence:string}],overview:string,transcript:string,corrections:[{original:string,replacement:string,type:string,reason:string}],improvements:[{original:string,suggestion:string,reason:string}],modelAnswer:string,modelExplanation:string,language:{collocations:[{english:string,chinese:string}],sentencePatterns:[{english:string,chinese:string}]}}。全部字段必需。分数范围 0 到 9、步长 0.5。主题、总体评价、范文和各项证据不能空白。没有纠错或优化时返回空数组，不能制造错误。original 逐字引用原文，corrections 只包含确定错误，improvements 只含可选优化。语料含中文释义，搭配最多 5 条、句式最多 3 条。${kind === "speaking" ? "criteria 恰含 FC、LR、GRA、P；P.score 必须为 null，证据说明无法仅凭转写判断发音。range 为包含 overall 的合理区间 [下限,上限]。transcript 仅补标点、大小写和分段，不得增删替换词。" : "Task 1 的 criteria 恰含 TA、CC、LR、GRA；Task 2 恰含 TR、CC、LR、GRA。range 为空数组，transcript 为空字符串。"}`;
  }

  // JSON is the stored result; this deterministic adapter keeps legacy report and
  // annotation components working without allowing a model to choose their layout.
  function reviewPresentation(content, kind) {
    const parsed = JSON.parse(content);
    const pick = (value, keys) => Object.fromEntries(keys.map(key => [key, value[key]]));
    const report = pick(parsed, ["topic","overall","range","overview","transcript","modelAnswer","modelExplanation"]);
    report.criteria = parsed.criteria.map(item => pick(item,["code","score","evidence"]));
    report.corrections = parsed.corrections.map(item => pick(item,["original","replacement","type","reason"]));
    report.improvements = parsed.improvements.map(item => pick(item,["original","suggestion","reason"]));
    report.language = Object.fromEntries(["collocations","sentencePatterns"].map(key => [key,parsed.language[key].map(item => pick(item,["english","chinese"]))]));
    const plain = value => String(value ?? "").replace(/&/g,"&amp;").replace(/</g,"&lt;").replace(/>/g,"&gt;").replace(/([\\`*_{}\[\]()#+.!|~-])/g,"\\$1");
    const cell = value => plain(value).replace(/\r?\n/g," ");
    const row = values => "| " + values.map(cell).join(" | ") + " |";
    const lines = ["主题："+cell(report.topic),"", "### 评分与小分", kind === "speaking" ? `基于转写的暂定总分：${report.overall}（合理区间：${report.range.join("–")}；真实总分会受发音影响）` : `非官方总分：${report.overall}`,"", "| 项目 | 小分 | 证据 |","| --- | --- | --- |", ...report.criteria.map(c => row([c.code,c.score ?? "不可仅凭转写判断",c.evidence])), "", "### 总体评价", plain(report.overview), ""];
    if (kind === "speaking") lines.push("### 转写整理稿",plain(report.transcript), "");
    lines.push("### 确定语法错误");
    if (report.corrections.length) lines.push("| 原文 | 修改 | 类型 | 原因 |","| --- | --- | --- | --- |",...report.corrections.map(c=>row([c.original,c.replacement,c.type,c.reason])));
    else lines.push("没有确定语法错误。");
    lines.push("", "### 原文优化建议", ...report.improvements.map(c=>`- ${cell(c.original)} → ${cell(c.suggestion)}。${cell(c.reason)}`));
    if (!report.improvements.length) lines.push("没有额外优化建议。");
    lines.push("", "### 目标水平范文",plain(report.modelAnswer),"",plain(report.modelExplanation),"", "### 最终值得记忆的语料");
    for (const [key,title] of [["collocations","核心搭配"],["sentencePatterns","实用句式"]]) lines.push("", "#### "+title,...report.language[key].map(e=>`- ${cell(e.english)}｜${cell(e.chinese)}`));
    return {report,markdown:lines.join("\n")};
  }

 return Object.freeze({IELTS_WRITING_SCORING_GUIDE,IELTS_SPEAKING_SCORING_GUIDE,writingTaskAssessment,speakingPartAssessment,reviewFormatContract,reviewPresentation});
});
