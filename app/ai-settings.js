(() => {
  "use strict";
  const presets = {
    custom: {baseUrl:"",protocol:"openai",help:"填写中转站提供的基础地址。一个站点可提供多个厂商的模型；按站点要求选择协议。"},
    openai: {baseUrl:"https://api.openai.com/v1",protocol:"openai",help:"OpenAI 官方 API。"},
    anthropic: {baseUrl:"https://api.anthropic.com/v1",protocol:"anthropic",help:"Claude 官方 Messages API。"},
    glm: {baseUrl:"https://open.bigmodel.cn/api/paas/v4",protocol:"openai",help:"智谱 GLM 官方接口。国际站可选择自定义地址填写 Z.AI 提供的地址。"},
    gemini: {baseUrl:"https://generativelanguage.googleapis.com/v1beta",protocol:"gemini",help:"使用 Google AI Studio 创建的 API Key；Gemini 网页会员与 API 分开。"},
    grok: {baseUrl:"https://api.x.ai/v1",protocol:"openai",help:"xAI Grok 官方 API。"},
    deepseek: {baseUrl:"https://api.deepseek.com",protocol:"openai",help:"DeepSeek 官方 API。是否支持图片取决于所选模型，不会自动替换模型名称。"}
  };
  window.ELPAISettings = {
    create({onConnection,notify}) {
      const forms = new Map();
      const request = async (url, body, signal) => {
        const response = await fetch(url,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(body),signal});
        const data = await response.json().catch(()=>({}));
        if (!response.ok) throw new Error(data.error || `接口请求失败（${response.status}），请确认已启动新版程序`);
        return data;
      };
      const node = (prefix,suffix) => document.getElementById(prefix+suffix);
      function modelOptions(prefix, models, selected="") {
        const select=node(prefix,"Model"); select.replaceChildren(new Option(models.length ? "请选择模型" : "先获取模型列表", ""));
        const seen=new Set();
        for(const model of models) {
          if (!model || typeof model.id!=="string" || !model.id.trim() || seen.has(model.id)) continue;
          seen.add(model.id); select.add(new Option(model.name && model.name!==model.id ? `${model.id} · ${model.name}` : model.id, model.id));
        }
        if(selected && !seen.has(selected)) select.add(new Option(`${selected}（已保存 / 手动指定）`,selected));
        select.value=selected || (seen.size===1 ? [...seen][0] : "");
      }
      const config = prefix => ({provider:node(prefix,"Provider").value,baseUrl:node(prefix,"BaseUrl").value.trim(),protocol:node(prefix,"Protocol").value,apiKey:node(prefix,"ApiKey").value,model:node(prefix,"Model").value});
      function message(prefix,text,error=false) {
        const target=node(prefix,"TestResult");target.className=`feedback-box${error?" is-error":""}`;target.textContent=text;
      }
      function invalidate(prefix) {
        const form=forms.get(prefix); form.version++; form.controller?.abort(); form.touched=true;
        modelOptions(prefix,[]);node(prefix,"ModelsStatus").textContent="地址或协议已更改，请重新获取模型";
        node(prefix,"ApiKey").placeholder="填写此接口的 Key；只会复用同一地址和协议下保存的 Key";
      }
      function populate(prefix,c) {
        const provider=Object.hasOwn(presets,c.provider) ? c.provider : Object.entries(presets).find(([id,preset])=>id!=="custom" && preset.baseUrl.replace(/\/$/,"")===c.baseUrl?.replace(/\/$/,"") && preset.protocol===(c.protocol||"openai"))?.[0] || "custom";
        node(prefix,"Provider").value=provider;node(prefix,"ProviderHelp").textContent=presets[provider].help;
        node(prefix,"BaseUrl").value=c.baseUrl || "";node(prefix,"Protocol").value=c.protocol || "openai";
        modelOptions(prefix,[],c.model || "");
        if(c.connected) node(prefix,"ApiKey").placeholder="已在本机加密保存；留空继续使用，填写可替换";
      }
      async function refresh() {
        try {
          const response=await fetch("/api/ai/status",{cache:"no-store"});if(!response.ok)throw new Error();const data=await response.json();
          onConnection(Boolean(data.connected),data.model || "");
          if(data.baseUrl) {forms.get("ai").hasSaved=true;if(!forms.get("ai").touched)populate("ai",data);}
          if(data.vision && !forms.get("vision").touched) populate("vision",data.vision);
          const badge=node("vision","AiBadge");badge.textContent=data.vision?.connected ? "已连接" : "使用文字接口";badge.className=`status-badge ${data.vision?.connected?"status-on":"status-off"}`;
          const route=document.getElementById("imageRouteStatus"), independent=Boolean(data.vision?.connected), active=independent ? data.vision : data;
          route.textContent=active.connected ? `图表提取使用${independent?"独立图片":"文字"}接口的 ${active.model}。${active.visionVerified?"识图测试已通过。":"首次提取会先测试识图；未通过则不会发送练习资料。"}` : "未配置独立图片接口时，图表提取使用文字接口所选的模型；该模型必须支持识图。";
          if(data.storageError)message("ai",data.storageError,true);
          else if(data.restored) message("ai","已恢复本机加密配置。可以直接使用，或重新测试连接。");
        } catch {onConnection(false);}
      }
      async function discover(prefix) {
        const form=forms.get(prefix), cfg=config(prefix), current=cfg.model;
        if(!cfg.baseUrl){node(prefix,"BaseUrl").focus();return message(prefix,"先填写接口地址",true);}
        form.controller?.abort();const controller=new AbortController();form.controller=controller;const version=++form.version;
        const button=node(prefix,"FetchModels"),status=node(prefix,"ModelsStatus");button.disabled=true;status.className="";status.textContent="正在获取模型……";
        try {
          const data=await request("/api/ai/models",{baseUrl:cfg.baseUrl,protocol:cfg.protocol,apiKey:cfg.apiKey,purpose:prefix==="vision"?"vision":"text"},controller.signal);
          if(version!==form.version)return;
          if(!Array.isArray(data.models) || !data.models.some(model=>typeof model?.id==="string" && model.id.trim()))throw new Error("没有获得可选择的模型");
          modelOptions(prefix,data.models,current);status.textContent=`已获取 ${data.models.length} 个模型，请从下方选择`;
        } catch(error) {if(version===form.version && error.name!=="AbortError"){status.className="is-error";status.textContent=error.message;}}
        finally {if(form.controller===controller && !form.saving)button.disabled=false;}
      }
      async function save(prefix,event) {
        event.preventDefault();const cfg=config(prefix),form=forms.get(prefix);
        if(!cfg.model)return message(prefix,"请先获取列表并选择模型",true);
        form.saving=true;form.version++;form.controller?.abort();node(prefix,"FetchModels").disabled=false;
        const controls=[...form.element.querySelectorAll("input,select,button")];const disabled=controls.map(control=>control.disabled);controls.forEach(control=>{control.disabled=true;});
        message(prefix,prefix==="vision"?"正在使用测试图片连接模型……":"正在测试文字连接……");
        try {
          await request(prefix==="vision"?"/api/ai/vision/config":"/api/ai/config",cfg);
          node(prefix,"ApiKey").value="";form.touched=false;await refresh();
          message(prefix,`${cfg.model} 已连接，配置已在本机加密保存。${prefix==="vision"?"识图测试已通过，图表提取将使用此接口，核对后由文字接口批改。":""}`);notify("接口已保存");
        } catch(error) {message(prefix,`${error.message}。之前保存的接口保持不变。`,true);}
        finally {form.saving=false;controls.forEach((control,i)=>{control.disabled=disabled[i];});}
      }
      async function disconnect(prefix) {
        if(!confirm(`删除本机保存的${prefix==="vision"?"独立图片":"文字"}接口配置？另一接口和学习记录会保留。`))return;
        try {
          await request(prefix==="vision"?"/api/ai/vision/disconnect":"/api/ai/disconnect",{});
          forms.get(prefix).touched=false;invalidate(prefix);populate(prefix,{});node(prefix,"ApiKey").value="";await refresh();
          message(prefix,prefix==="vision"?"已删除独立图片配置，图表提取将使用文字接口所选模型；该模型必须支持识图。":"已删除文字接口配置。");
        }catch(error){message(prefix,error.message,true);}
      }
      function bind() {
        for(const prefix of ["ai","vision"]) {
          const element=document.getElementById(prefix==="ai"?"aiSettings":"visionSettings");forms.set(prefix,{element,version:0,controller:null,touched:false});
          element.addEventListener("input",()=>{forms.get(prefix).touched=true;});
          node(prefix,"Provider").addEventListener("change",()=>{
            const preset=presets[node(prefix,"Provider").value];invalidate(prefix);node(prefix,"BaseUrl").value=preset.baseUrl;node(prefix,"Protocol").value=preset.protocol;node(prefix,"ProviderHelp").textContent=preset.help;node(prefix,"ApiKey").value="";
          });
          for(const suffix of ["BaseUrl","Protocol"])node(prefix,suffix).addEventListener("change",()=>invalidate(prefix));
          node(prefix,"ApiKey").addEventListener("input",()=>{const form=forms.get(prefix);form.version++;form.controller?.abort();node(prefix,"ModelsStatus").textContent="Key 已更改，请重新获取模型列表";});
          node(prefix,"FetchModels").addEventListener("click",()=>discover(prefix));
          node(prefix,"UseManualModel").addEventListener("click",()=>{const id=node(prefix,"ManualModel").value.trim();if(id)modelOptions(prefix,[],id);});
          element.addEventListener("submit",event=>save(prefix,event));
          document.getElementById(prefix==="ai"?"disconnectAi":"disconnectVision").addEventListener("click",()=>disconnect(prefix));
        }
        window.addEventListener("elp:route",event=>{if(event.detail==="settings")refresh();});
      }
      return {bind,refresh,populateLegacy(preferences){if(!forms.get("ai")?.touched && !forms.get("ai")?.hasSaved)populate("ai",{baseUrl:preferences.aiBaseUrl,model:preferences.aiModel,protocol:preferences.aiProtocol || "openai"});}};
    }
  };
})();
