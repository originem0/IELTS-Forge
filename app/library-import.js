(() => {
  "use strict";
  const $ = id => document.getElementById(id);
  const api = window.ELPLibrary;
  const names = {reading:"阅读",listening:"听力",writing:"写作",speaking:"口语"};
  const units = {reading:"篇",listening:"段",writing:"题",speaking:"题"};
  let selection = null;
  let version = 0;
  let savedDisabled = [];
  const node = (tag,text,className) => { const el=document.createElement(tag);if(text!==undefined)el.textContent=text;if(className)el.className=className;return el; };
  const report = (text,error=false) => { $("libraryStatus").textContent=text;$("libraryStatus").classList.toggle("is-error",error); };
  function busy(value) {
    const controls=["libraryFiles","libraryFolder","importLibrary","cancelLibraryImport","refreshLibrary","selectDataDirectory","onboardingSelectDirectory"].map($).filter(Boolean);
    if(value){savedDisabled=controls.map(control=>control.disabled);controls.forEach(control=>{control.disabled=true;});}
    else {controls.forEach((control,i)=>{control.disabled=savedDisabled[i]||false;});$("importLibrary").disabled=!selection;}
    api.setBusy(value);$("libraryPage").setAttribute("aria-busy",String(value));
  }
  function step(number) {
    document.querySelectorAll("[data-import-step]").forEach(item=>{const current=Number(item.dataset.importStep)===number;item.classList.toggle("is-current",current);if(current)item.setAttribute("aria-current","step");else item.removeAttribute("aria-current");});
    $("libraryChoose").classList.toggle("hidden",number!==1);
    $("libraryPreview").classList.toggle("hidden",number!==2);
    $("libraryConfirm").classList.toggle("hidden",number!==2);
    $("librarySuccess").classList.toggle("hidden",number!==3);
  }
  function discard() { if(selection?.token)api.request(`import/${selection.token}`,{method:"DELETE"}).catch(()=>{});selection=null; }
  function reset() { version++;discard();$("libraryFiles").value="";$("libraryFolder").value="";$("importLibrary").disabled=true;step(1);report(""); }
  function countCards(counts) {
    const cards=node("div",undefined,"library-counts");
    for(const [skill,name] of Object.entries(names))if(counts[skill]){const card=node("div");card.append(node("span",name),node("strong",String(counts[skill])),node("small",units[skill]));cards.append(card);}
    return cards;
  }
  async function choose(files) {
    if(api.busy||!files.length)return;
    discard();const requestVersion=++version;step(1);report("正在识别题目和配套附件……");busy(true);
    try {
      if(files.reduce((sum,file)=>sum+file.size,0)>256*1024*1024)throw new Error("资料超过 256 MB，请选择整理好的题库包或题库文件夹。");
      const form=new FormData();for(const file of files)form.append("files",file,file.name);
      form.append("modified",JSON.stringify(files.map(file=>file.lastModified||0)));
      const result=await api.request("import/preview",{method:"POST",body:form});
      if(requestVersion!==version){api.request(`import/${result.token}`,{method:"DELETE"}).catch(()=>{});return;}
      selection=result;
      const preview=$("libraryPreview");preview.replaceChildren(node("span","已自动识别","kicker"),node("h3","这些题目已准备好导入"),countCards(result.counts));
      preview.append(node("p",result.attachments?`已配对 ${result.attachments} 份音频或图片，无需再选择附件。`:"题目内容已检查，可以直接导入。"));
      const details=node("details");details.append(node("summary",`查看 ${result.titles.length} 个题库`));const list=node("ul");for(const title of result.titles)list.append(node("li",title));details.append(list);preview.append(details);
      if(result.ignored)preview.append(node("p",`已自动跳过 ${result.ignored} 个说明、核验或重复文件。`,"muted"));
      $("importLibrary").textContent="确认导入";step(2);report("");
    } catch(error) {if(requestVersion===version){report(error.message,true);$("libraryFiles").value="";$("libraryFolder").value="";}}
    finally {if(requestVersion===version)busy(false);}
  }
  for(const id of ["libraryFiles","libraryFolder"]){$(id).addEventListener("change",event=>choose([...event.target.files]));}
  $("cancelLibraryImport").addEventListener("click",reset);
  $("importLibrary").addEventListener("click",async()=>{
    if(!selection||api.busy)return;
    const selected=selection;const requestVersion=version;busy(true);report("正在保存题库……");
    try {
      const result=await api.request(`import/${selected.token}`,{method:"POST"});if(requestVersion!==version)return;
      selection=null;$("libraryFiles").value="";$("libraryFolder").value="";
      const success=$("librarySuccess");success.replaceChildren(node("span","已保存到本机","kicker"),node("h3",result.added?"导入成功":"题库已存在，无需重复导入"),node("p",`新增 ${result.added} 个题库，跳过 ${result.existing} 个已有题库。`),countCards(selected.counts));
      const actions=node("div",undefined,"button-row");let primary=true;
      const skills=Object.entries(names).sort(([a],[b])=>Number(b===api.returnSkill)-Number(a===api.returnSkill));
      for(const [skill,name] of skills)if(selected.counts[skill]){const action=node("button",`开始${name}`,`button ${primary?"button-primary":"button-secondary"}`);action.type="button";primary=false;action.addEventListener("click",()=>{if(skill==="reading"||skill==="listening")location.hash=`${skill}/new`;else window.dispatchEvent(new CustomEvent("elp:bank-practice",{detail:skill}));});actions.append(action);}
      const again=node("button","继续导入其他题库","button button-quiet");again.type="button";again.addEventListener("click",reset);actions.append(again);success.append(actions);step(3);report("题库已保存，重复内容会自动跳过。");
      await api.refresh();window.dispatchEvent(new CustomEvent("elp:library-updated"));
    } catch(error){if(requestVersion===version){
      if(error.status===409){discard();step(1);report(`${error.message}。请重新选择资料。`,true);}
      else {step(2);$("importLibrary").textContent="重试并确认导入结果";report(`尚未确认导入结果：${error.message}。点击重试，无需重新上传，也不会重复添加。`,true);}
    }}
    finally{if(requestVersion===version)busy(false);}
  });
  const drop=$("libraryDropZone");
  $("libraryReturn").addEventListener("click",()=>{const skill=api.returnSkill;if(skill==="reading"||skill==="listening")location.hash=skill;else if(skill)window.dispatchEvent(new CustomEvent("elp:bank-practice",{detail:skill}));});
  drop.addEventListener("dragover",event=>{event.preventDefault();if(!api.busy)drop.classList.add("is-dragover");});
  drop.addEventListener("dragleave",()=>drop.classList.remove("is-dragover"));
  drop.addEventListener("drop",event=>{event.preventDefault();drop.classList.remove("is-dragover");if([...event.dataTransfer.items].some(item=>item.webkitGetAsEntry?.()?.isDirectory)){report("文件夹请点击“选择题库文件夹”，ZIP 可以直接拖入。",true);return;}choose([...event.dataTransfer.files]);});
  window.addEventListener("elp:storage-changed",()=>{reset();if(api.busy)busy(false);});
})();
