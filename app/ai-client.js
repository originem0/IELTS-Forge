(function(root,factory){const api=factory();if(typeof module==="object"&&module.exports)module.exports=api;if(root)root.ELPAI=api;})(typeof window==="object"?window:null,()=>{
 "use strict";
 function send(payload) {
   return fetch("/api/ai/chat", {method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(payload)});
 }
 function create({getConnected,routeTo,media,reviewFormatContract}) {
  async function askAi(messages, output, onSuccess, isCurrent = () => true, images = [], reportKind = "") {
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
      const response = await send({
          messages: messages.map(message => message.role === "system" && reportKind ? { ...message, content: message.content + "\n\n" + reviewFormatContract(reportKind) } : message),
          temperature: 0,
          max_tokens: 6000,
          output_contract: reportKind ? `review-markdown-v1-${reportKind}` : "text"
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(data.error || "请求失败");
      if (isCurrent()) {
        output.textContent = "";
        output.classList.add("hidden");
      }
      if (data.content && typeof onSuccess === "function") await onSuccess(data.content);
    } catch (error) {
      if (!isCurrent()) return;
      output.classList.remove("markdown-body");
      output.textContent = `AI 反馈失败：${error.message}\n\n原稿与录音不会因此丢失。若提示长度上限或思考内容，请使用最新启动器或检查模型模式；只有鉴权失败才需要检查 Key。`;
      output.classList.add("is-error");
    }
  }

 return {askAi};
 }
 return Object.freeze({send,create});
});
