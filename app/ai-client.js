(function(root,factory){const api=factory();if(typeof module==="object"&&module.exports)module.exports=api;if(root)root.ELPAI=api;})(typeof window==="object"?window:null,()=>{
 "use strict";
 function send(payload) {
   return fetch("/api/ai/chat", {method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(payload)});
 }
 // One budget includes provider-envelope, server-contract and client-semantic validation.
 async function validated(payload, validate = data => data) {
   let remaining = 2;
   while (remaining > 0) {
     const response = await send({...payload, format_attempts:remaining});
     const data = await response.json();
     if (!response.ok) throw new Error(data.error || "AI 请求失败");
     remaining -= Number.isInteger(data.attempts) ? Math.max(1, data.attempts) : 1;
     try { return await validate(data); }
     catch (error) { if (!remaining) throw error; }
   }
 }
 async function commitReview({record,content,report,snapshot,title,save,punctuation}) {
   const next = {review:content,reviewData:report,reviewInput:snapshot,topicTitle:title,reviewedAt:new Date().toISOString()};
   if (punctuation) Object.assign(next,{punctuatedTranscript:punctuation,punctuationSource:snapshot.original});
   const previous = Object.fromEntries(Object.keys(next).map(key=>[key,record[key]]));
   Object.assign(record,next);
   try { await save(); }
   catch (error) { for (const key of Object.keys(next)) { if (previous[key]===undefined) delete record[key]; else record[key]=previous[key]; } throw error; }
 }
 function create({getConnected,routeTo,media,reviewFormatContract}) {
  async function askAi(messages, output, onSuccess, isCurrent = () => true, images = [], reportKind = "", reviewType = "") {
    if (!getConnected()) return routeTo("settings");
    messages = [{role:"system", content:messages.filter(message => message.role === "system").map(message => message.content).join("\n\n")}, ...messages.filter(message => message.role !== "system")];
    let safeImages;
    try { safeImages = await media.images(images); }
    catch (error) { output.textContent = error.message; output.classList.remove("hidden"); output.classList.add("is-error"); return; }
    if (safeImages.length) {
      const userIndex = messages.findLastIndex(message => message.role === "user");
      if (userIndex >= 0) messages[userIndex] = {
        ...messages[userIndex],
        content: [
          { type: "text", text: messages[userIndex].content },
          ...safeImages.map(src => ({ type: "image_url", image_url: { url: src, detail: "auto" } }))
        ]
      };
    }
    output.classList.remove("hidden");
    output.classList.remove("is-error", "markdown-body");
    output.textContent = "正在生成反馈……";
    try {
      const result = await validated({
          messages: messages.map(message => message.role === "system" && reportKind ? { ...message, content: message.content + "\n\n" + reviewFormatContract(reportKind) } : message),
          temperature: 0,
          max_tokens: 6000,
          output_contract: reportKind ? (reportKind === "speaking" ? "review-json-v1-speaking" : `review-json-v1-writing-${reviewType.startsWith("Task 1") ? "task1" : "task2"}`) : "text"
      }, data => reportKind ? window.ELPAssessment.reviewPresentation(data.content, reportKind) : {markdown:data.content});
      if (isCurrent()) {
        output.textContent = "";
        output.classList.add("hidden");
      }
      if (result.markdown && typeof onSuccess === "function") await onSuccess(result.markdown, result.report);
    } catch (error) {
      if (!isCurrent()) return;
      output.classList.remove("markdown-body");
      output.textContent = `AI 反馈失败：${error.message}\n\n原稿与录音不会因此丢失。若提示长度上限或思考内容，请使用最新启动器或检查模型模式；只有鉴权失败才需要检查 Key。`;
      output.classList.add("is-error");
    }
  }

 return {askAi};
 }
 return Object.freeze({send,validated,commitReview,create});
});
