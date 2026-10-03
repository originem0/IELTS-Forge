(function (root, factory) {
 const api=factory();
 if(typeof module === "object" && module.exports) module.exports=api;
 if(root) root.ELPReview=api;
})(typeof window === "object" ? window : null, () => {
 "use strict";
 function create({ getContext, $, practiceTitle, showToast, saveReviewCorrection, correctionKey, renderLanguageUse }) {
  let reviewWorkspaceAudioUrl = null;
  function renderAiFeedback(output, content) {
    if (window.renderReviewMarkdown) window.renderReviewMarkdown(output, content);
    else { output.classList.remove("is-error"); output.textContent = content; }
  }

  function recoverEmbeddedWritingPrompt(text) {
    const source = String(text || "");
    const answerMarker = source.search(/\n\s*(?:sample\s+writing\s+answer|my\s+answer|writing\s+answer|answer)\s*:?[ \t]*\n/i);
    if (answerMarker < 0) return "";
    const candidate = source.slice(0, answerMarker).trim();
    return /(?:TASK\s*[12]|write\s+(?:about|at\s+least)|summari[sz]e\s+the\s+information)/i.test(candidate) ? candidate : "";
  }

  function closeReviewImageLightbox() {
    const lightbox = $("#reviewImageLightbox");
    lightbox.classList.add("hidden");
    $("#reviewImageLightboxImage").removeAttribute("src");
    document.body.classList.remove("has-image-lightbox");
  }

  function openReviewImageLightbox(src, alt) {
    const image = $("#reviewImageLightboxImage");
    image.src = src;
    image.alt = alt;
    $("#reviewImageLightbox").classList.remove("hidden");
    document.body.classList.add("has-image-lightbox");
    $("#closeReviewImageLightbox").focus();
  }

  function populateReviewWorkspace() {
    const {state, reviewWorkspaceSelection, aiConnected, pendingWritingPromptImages} = getContext();
    if (!reviewWorkspaceSelection) return;
    const { module, id } = reviewWorkspaceSelection;
    const writing = module === "writing";
    const item = (writing ? state.writings : state.speaking).find(entry => entry.id === id);
    const snapshot = item?.reviewInput;
    // The review is tied to its submitted text, not a later edit in the editor.
    const original = snapshot?.original || (item?.review ? (writing ? item.essay : item.transcript) : $(writing ? "#writingEssay" : "#speakingTranscript").value);
    const storedPrompt = String(snapshot?.prompt || item?.prompt || "").trim();
    const recoveredPrompt = writing && !storedPrompt ? recoverEmbeddedWritingPrompt(original) : "";
    const prompt = storedPrompt || recoveredPrompt || (!item?.review ? $(writing ? "#writingPrompt" : "#speakingPrompt").value : "");
    const punctuated = !writing && item?.punctuationSource === original && window.isPunctuationOnlyRevision?.(original, item.punctuatedTranscript) ? item.punctuatedTranscript : "";
    $("#reviewRawTranscriptPanel").classList.toggle("hidden", writing);
    $("#reviewRawTranscript").textContent = writing ? "" : original || "";
    $("#reviewAnnotatedTitle").textContent = writing ? "原文与修改标注" : "标点与大小写整理稿 · 修改标注";
    $("#reviewPunctuationTools").classList.toggle("hidden", writing);
    $("#generatePunctuation").classList.toggle("hidden", Boolean(punctuated));
    $("#generatePunctuation").disabled = !aiConnected || !item || !original;
    $("#punctuationStatus").textContent = punctuated ? "仅整理标点、大小写和分段，保留原词句；以下标注对应 AI 已给出的修改。" : "当前报告尚无通过校验的整理稿。生成会调用已配置的 AI，仅补标点与大小写，不重新批改。";
    $("#reviewWorkspaceTitle").textContent = practiceTitle(item || { prompt });
    const type = snapshot?.type || item?.type || ({p1:"Part 1",p2:"Part 2",p3:"Part 3",free:"自由表达"}[item?.part]) || (writing ? "写作" : "口语");
    $("#reviewWorkspaceMeta").textContent = `${writing ? "写作" : "口语"}批改报告 · ${type}${item?.reviewedAt ? ` · ${new Date(item.reviewedAt).toLocaleString()}` : ""}`;
    $("#retryReviewSource").textContent = writing ? "根据本次批改再练此题" : "再次练习本题";
    const previous = (writing ? state.writings : state.speaking).find(entry => entry.id === item?.parentSessionId);
    const comparison = $("#reviewAttemptComparison"); comparison.replaceChildren();
    const disclosure = $("#reviewPreviousAttempt"); disclosure.open = false;
    disclosure.classList.toggle("hidden", !previous);
    if (previous) {
      for (const [record, label] of [[previous, "前次回答"], [item, "本次回答"]]) {
        const section = document.createElement("section"), title = document.createElement("h4"), answer = document.createElement("p");
        title.textContent = `${label} · 第 ${record.attemptNumber || 1} 次`;
        answer.className = "review-original";
        answer.textContent = record.reviewInput?.original || (writing ? record.essay : record.transcript) || "暂无文字回答";
        section.append(title, answer); comparison.append(section);
      }
      const link = document.createElement("a"); link.className = "button button-secondary";
      link.href = `#review/${module}/${encodeURIComponent(previous.id)}`; link.textContent = "打开前次报告与录音";
      comparison.append(link);
    }
    $("#reviewWorkspacePrompt").textContent = prompt || "尚未填写题目 / 话题";
    document.getElementById("reviewChartEvidence")?.remove();
    if (writing && typeof snapshot?.chartText === "string" && snapshot.chartText) {
      const details = document.createElement("details"); details.id = "reviewChartEvidence";
      const summary = document.createElement("summary"); summary.textContent = "本次批改采用的已核对图表信息";
      const content = document.createElement("p"); content.className = "review-original"; content.textContent = snapshot.chartText;
      details.append(summary, content); $("#reviewWorkspacePrompt").after(details);
    }
    $("#reviewWorkspaceOriginal").textContent = original || "尚无原始回答";
    $("#reviewWorkspaceNotice").textContent = recoveredPrompt
      ? "这条旧记录没有单独保存原题，已从提交原稿中识别并补充到原题板块；提交原稿仍完整保留，不做删改。"
      : snapshot ? "展示批改时提交的原稿。标注来自 AI 已返回的修改，不改变你的原文。" : item?.review ? "历史报告：原文取自该记录保存的答案，旧记录没有独立的提交快照。" : "本题预览，不会自动请求 AI 或覆盖编辑区。";
    $("#editReviewSource").disabled = !item;
    const snapshotImages = Array.isArray(snapshot?.promptImages) && snapshot.promptImages.length ? snapshot.promptImages : null;
    const images = writing ? snapshotImages || (item?.review ? item.promptImages : pendingWritingPromptImages) || [] : [];
    const imageRoot = $("#reviewWorkspaceImages");
    imageRoot.replaceChildren();
    images.forEach((src, index) => {
      if (!window.ELPMedia.isStored(src) && (typeof src !== "string" || !/^data:image\/(png|jpe?g|gif|webp);base64,/i.test(src))) return;
      const preview = document.createElement("button");
      preview.type = "button";
      preview.className = "review-image-preview";
      preview.setAttribute("aria-label", `全屏查看题目图片 ${index + 1}`);
      const image = document.createElement("img");
      image.src = src;
      image.alt = `题目图片 ${index + 1}`;
      const zoom = document.createElement("span");
      zoom.className = "review-image-zoom-icon";
      zoom.setAttribute("aria-hidden", "true");
      zoom.innerHTML = '<svg viewBox="0 0 24 24"><circle cx="10.5" cy="10.5" r="6.5"></circle><path d="m15.5 15.5 5 5M10.5 7.5v6M7.5 10.5h6"></path></svg>';
      preview.append(image, zoom);
      preview.addEventListener("click", () => openReviewImageLightbox(src, image.alt));
      imageRoot.append(preview);
    });
    $("#reviewQuestionPanel").classList.toggle("has-images", imageRoot.childElementCount > 0);
    const audio = $("#reviewWorkspaceAudio");
    audio.pause();
    audio.removeAttribute("src");
    if (reviewWorkspaceAudioUrl) URL.revokeObjectURL(reviewWorkspaceAudioUrl);
    reviewWorkspaceAudioUrl = null;
    audio.classList.add("hidden");
    if (!writing && item?.transcript === original && window.ELPMedia.isStored(item.audio)) {
      audio.src = item.audio; audio.classList.remove("hidden");
    } else if (!writing && item?.transcript === original && /^data:audio\/[\w.+-]+(?:;codecs=[\w.-]+)?;base64,/.test(item?.audio || "")) {
      try {
        const [header, encoded] = item.audio.split(",");
        const blob = new Blob([Uint8Array.from(atob(encoded), char => char.charCodeAt(0))], { type: header.slice(5).replace(/;base64$/, "") });
        reviewWorkspaceAudioUrl = URL.createObjectURL(blob);
        audio.src = reviewWorkspaceAudioUrl;
        audio.classList.remove("hidden");
      } catch { showToast("录音无法读取，原始文字和 AI 反馈仍可查看"); }
    }
    const feedback = item?.review || "尚未生成 AI 反馈。原稿和录音无需 AI 即可保存与复习。";
    const annotations = window.renderReviewAnnotations?.({
      original: writing ? original || "" : punctuated, markdown: item?.review || "", punctuationOnly: !writing,
      definiteOnly: writing,
      originalElement: $("#reviewWorkspaceOriginal"), correctionsElement: $("#reviewCorrections"),
      countElement: $("#reviewAnnotationCount"), noticeElement: $("#reviewAnnotationNotice")
      ,notebookOriginal: original || ""
      ,onSaveCorrection: item?.review ? correction => saveReviewCorrection(module, item, correction) : undefined
      ,isCorrectionSaved: correction => state.mistakes.some(note => note.correctionKey === correctionKey(module, id, correction))
    });
    if (!writing && !punctuated) $("#reviewAnnotationNotice").textContent = "生成整理稿后，修改标注将显示在这里。原始转写和下方修改建议保持不变。";
    if (window.renderReviewReport) window.renderReviewReport($("#reviewWorkspaceFeedback"), feedback, null, {
      dedupeCorrections: true,
      hideTranscript: !writing,
      strictSections: true,
      scoreElement: $("#reviewScoreSummary"),
      overviewElement: $("#reviewOverviewSummary")
    });
    else renderAiFeedback($("#reviewWorkspaceFeedback"), feedback);
    let usage = document.getElementById("reviewLanguageUse");
    if (!usage) { usage = document.createElement("div"); usage.id = "reviewLanguageUse"; usage.className = "study-language-use"; $("#reviewWorkspaceFeedback").append(usage); }
    renderLanguageUse?.(usage, module, item);
  }

  function refreshReviewWorkspace(module, id) {
    const {reviewWorkspaceSelection} = getContext();
    if (reviewWorkspaceSelection?.module === module && reviewWorkspaceSelection.id === id) populateReviewWorkspace();
  }

  function releaseReviewAudio() {
    $("#reviewWorkspaceAudio").pause();
    $("#reviewWorkspaceAudio").removeAttribute("src");
    if (reviewWorkspaceAudioUrl) URL.revokeObjectURL(reviewWorkspaceAudioUrl);
    reviewWorkspaceAudioUrl = null;
  }

  return {renderAiFeedback,populateReviewWorkspace,refreshReviewWorkspace,releaseReviewAudio,closeReviewImageLightbox,openReviewImageLightbox};
 }
 return Object.freeze({create});
});
