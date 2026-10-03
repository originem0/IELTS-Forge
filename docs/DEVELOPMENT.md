# 开发与验收

更新日期 2026-10-04。架构、开发和验收统一维护在本页；已完成的阶段计划、审查与维护日志不再分别维护。版本历史由 Git 保留，当前变更见 [更新说明](RELEASE_NOTES.md)。

## 项目结构

前端是无需构建的原生 JavaScript、HTML 和 CSS，后端使用 Go 标准库。支持 Windows 10/11 x64，完整包包含启动器、结束工具、whisper.cpp CPU 引擎及 small.en 模型。没有新增应用运行时依赖。

| 路径 | 职责 |
| --- | --- |
| `app/index.html`、`styles.css`、`desktop.css` | 页面结构、响应式样式与首页入门指南 |
| `app/app.js`、`practice-lifecycle.js` | 四科入口、状态协调、自动保存、计时和导航屏障 |
| `app/study-engine.js`、`study-coordinator.js`、`plan-budget.js` | 到期调度、实际材料队列、稳定每日任务、学习事件与时间预算 |
| `app/storage-client.js`、`state-schema.js`、`media-client.js` | 增量写入、旧数据校验、媒体引用 |
| `app/library.js`、`library-import.js`、`objective-practice.js` | 选题、导入、听读作答、模拟与复盘 |
| `app/assessment.js`、`ai-client.js`、`ai-settings.js` | 具体 Task/Part 评分要求、AI 调用与两套接口配置 |
| `app/review-workspace.js`、`review-annotations.js`、`markdown.js` | 固定报告结构、原文定位与安全 Markdown |
| `launcher/storage*.go`、`study_*.go` | 档案 revision、独立记录与媒体、恢复点和回收 |
| `launcher/library*.go`、`backup.go` | 不可变题包、来源、导入、索引、判分、删除保护和备份 |
| `launcher/library_explanations.go` | 所选错题的文字 AI 讲解、逐字引文校验与独立保存 |
| `launcher/ai*.go`、`credentials_windows.go`、`transcription*.go` | 三种接口协议、视觉验证、DPAPI 与本机转写 |
| `scripts/convert_question_banks.py`、`package_question_bank.py` | 本地素材转换及单个完整题库 ZIP |

## 数据约束

所有学习状态通过绑定目录的 API 写盘。主档案使用 revision 防止多标签页互相覆盖；Windows 文件锁阻止多个程序同时绑定同一目录。新的媒体与记录独立保存，旧内嵌录音、图片及未知字段兼容读取，迁移保留恢复点。

四科切页和档案切换等待真实保存完成。录音、转写和 AI 操作由共同屏障管理，保存失败保留原稿。计时依据时间锚点计算，后台节流不改变实际经过时间。

`learning` 元数据保存学习事件、按日期和计划版本隔离的每日任务快照、原作答复盘状态及听读错题调度；原 `planProgress` 不删除。错句、单词和表达沿用各自已保存的复习字段。首次读取听读记录只建立可重建的内存清单，用户保存或完成听读提交时统一写入，避免刷新造成后台迁移与档案 revision 冲突。听读处理按答案内容识别变化，AI 讲解更新不算一次新复习；正确重练后仍保留原来答错的来源，供下次合法创建局部重练。

每天最多一个主要新练习，余量由真实复习材料决定；Task 1 与 Task 2 使用不同预算，重写沿用原记录时长。近七天覆盖、失败次数与实际耗时参与分配，固定比例不再挤占整周新学习。完成与掌握分别记录；同一天重复成功不连续升级间隔，失败至少间隔五分钟，每项每天最多两次。

题库版本不可变，选题按来源与单元 ID 展示最新状态，旧记录保留原题。删除题包检查当前写说档案、听读记录、独立记录版本、滚动备份及每日备份。存在引用时拒绝删除，使用新版本的 `hidden` 标记退出选题。

听读记录删除通过 `DELETE /api/library/attempts/{id}`，使用 `X-ELP-Directory` 和 `If-Match` 校验目录与记录版本，同时移除该记录的滚动副本。已有重练保留创建时核验的题目范围，删除来源记录不妨碍继续保存；旧页面不能通过后续保存恢复被删除的记录。

带图写作的 `chartExtraction` 保存提取来源、可编辑文字及确认时间，来源由题型、题目和图片引用共同确定；改变来源会使提取结果失效。批改的 `reviewInput` 独立保留已确认的图表文字，原始题图继续用于报告展示。图片接口提取与文字接口批改之间必须有用户确认，不静默降级。

`POST /api/library/attempts/{id}/explanations` 仅接受已提交记录的 1–5 道错题，后端从不可变题包读取标准答案和原文。模型只使用文字接口，结果按固定结构校验；引文需逐字连续存在于指定来源，失败自动重试一次。调用结束后重新核验目录和记录版本，单独写入 `explanations`，不经过答案修改接口，不影响判分。

ZIP 导入总量及解压内容各限 1 GiB，单媒体限 128 MiB，最多 5000 个归档条目。媒体流式哈希和落盘，锁内只提交结果；导入路由单独延长上传期限，普通请求仍保留短期限。裸 MPEG Layer III 使用相邻帧头识别，不添加标签或重编码。文件名、内容类型和引用仍需匹配，不能仅凭 `.mp3` 扩展名放行。

题库摘要可重建，未修改条目不重复读取正文；前端共享请求和题包缓存，选题分页并延迟创建题干。导入回执持久化，响应丢失可确认原结果，不产生重复题包。

## 入门指南与文档边界

首页的“雅思入门指南”使用 `#guide` 路由，正文在 `app/index.html`。四科说明使用原生 `details` 按需展开，三题阅读、写作和口语热身沿用现有练习入口。原创示例须与正式试题区分，考试规则附官方出处与核对日期，不将应用练习时长或 AI 反馈冒充正式考务与成绩。

README 负责安装、功能与入口；[AI 配置](AI_CONFIGURATION.md)、[题库格式](QUESTION_BANK_FORMAT.md) 和 [材料范围](CONTENT_GUIDE.md) 各自保留一个说明来源。历史计划、临时交付记录、个人档案清理日志与题库统计不放进公共文档。

## 可重复验证

```powershell
node scripts/check.mjs
go -C launcher test ./...
go -C launcher vet ./...
```

真实浏览器验证使用已有 Playwright，通过 `ELP_PLAYWRIGHT_MODULE` 指定模块。默认调用 Chrome；CI 设置 `ELP_BROWSER_CHANNEL=bundled` 使用安装的 Chromium。

```powershell
$env:ELP_BROWSER_TEST='1'
go -C launcher test ./... -count=1
node tests/ui-smoke.cjs
node tests/markdown-review.cjs
node tests/correction-notebook.cjs
node tests/study-loop-ui.cjs
```

图表核对的分流与持久化由 `tests/speaking-writing-regression.mjs` 和 `tests/markdown-review.cjs` 覆盖。听读讲解可单独运行 `go -C launcher test -run 'TestObjectiveExplanation' -count=1 -v`；设置上述浏览器环境后，同时运行真实磁盘与浏览器验收。模型响应使用测试夹具，不调用个人 API。听读历史删除随 `TestLibraryBrowser` 验证，删除来源后的重练保存由 `TestAttemptDeletePreservesReviewAndRejectsStaleWrites` 验证。

可选题库验收直接读取本地整理 ZIP，无需保留抓取目录。归档应由 `scripts/package_question_bank.py` 生成并包含清单。检查题包、附件哈希、全部标准答案及允许变体、整套试卷数量；开启浏览器后还要求题包包含 General 书信及 Part 2 口语，并验证筛选与默认设置、录音元数据、重复导入及窄屏布局。

```powershell
$env:ELP_QUESTION_BANK_ZIP=(Resolve-Path 'question-banks/完整题库.zip').Path
$env:ELP_BROWSER_TEST='1'
go -C launcher test -run '^TestLocalQuestionBankArchive$' -count=1 -v
```

测试使用隔离档案。个人素材与 `dist-test/` 测试输出不提交；README 的产品截图仅使用演示数据。自动验证不能代替逐字核对出版原件或逐段人工听审；音频能加载也不等于所有录音内容均准确。

指南修改需检查首页入口、折叠内容的键盘操作、桌面和窄屏布局，以及三种热身入口。页面文件修改后还需核验实际运行副本；保留用户未保存的练习，不用测试开关让用户的启动器在后台静默驻留。

## 构建与交付

```powershell
./build-portable.ps1 -Version dev
```

可用 `-WhisperBundleDirectory` 复用已校验的完整 Whisper 目录。构建在新的 `dist/build-*` 下进行，不清空 `dist`，不覆盖已有 ZIP。两个 Go 构建任一失败立即停止；打包验证语音模型哈希、组件和许可完整性，并排除档案、凭据与开发机路径。

`tests/package-smoke.cjs` 通过 `ELP_PACKAGE_ROOT`、`ELP_PACKAGE_VERSION` 指向新包，运行实际可执行程序、整套浏览器流程、退出重启及旧媒体恢复；设置 `ELP_WHISPER_TEST_WAV` 可验证真实离线转写。它不会使用个人 API Key。

本地交付与公开 Release 分开。推送 `main`、提交 Pull Request 或手动触发工作流会运行 Windows 浏览器验证和完整打包；`v*` 标签才触发最后的发布任务。两个 Windows 任务固定 `windows-2022`，浏览器任务成功后才构建，完整构建及 artifact 上传成功后才允许发布。每次提交的实际结果以 [GitHub Actions](https://github.com/originem0/IELTS-Forge/actions) 为准。


## 审查后的可靠性边界

`storage-client.js` 负责增量事务、提交回执和删除回滚，`practice-lifecycle.js` 负责自动保存、计时与任务屏障，`study-engine.js` / `study-coordinator.js` 负责复习调度和显式提交，`ai-client.js` 负责统一格式重试预算和报告提交回滚。页面代码仍负责渲染与事件绑定，不增加新的框架或运行依赖。

保存操作使用持久提交编号。丢失响应后的重试复用编号和原请求，服务端不能把后续写入的 revision 当成本次回执。增量补丁携带实际修改字段的原值；不相关记录可以合并，同记录修改和删除前发生的变化返回冲突。整个档案的 PUT 替换继续要求严格 revision 匹配。

读取主索引时校验所有引用，再依次尝试滚动与每日备份。高版本文件禁止回退。两份主快照损坏时，可在设置中检查并抢救健康记录；先展示清单，确认时核对快照 token，保留原件和抢救报告，旧索引不得复活已删除记录。听读从备份恢复时有可见提示。

写说新报告使用 `review-json-v1-*` 契约，核对任务对应维度、0–9 的半分档、非空证据与正文。口语仅有转写时不能生成发音分。结构化结果保存在 `reviewData`，固定适配器生成兼容现有批注组件的展示文本，历史 Markdown 保留。每次 AI 请求的服务端与客户端格式校验共享两次上游调用预算；认证、网络与明确拒绝不作为格式失败重试。

完整备份先在锁内固定文件快照，再在锁外校验和压缩。批量读取只验证一次目录路径，仍逐文件拒绝符号链接并校验内容。性能验收 `ELP_STORAGE_BENCH=1 go test -run TestAuditStorageLatency -v` 使用 1000 条模拟写作记录和 500 KB 音频，结果写入 dist-test；不包含 UI 的自动保存延迟，不代表慢盘或真实大档案的性能保证。

Windows 构建工具链使用受支持的 Go 1.27.x，go.mod 保留最低兼容版本。最终 ZIP 必须解压后使用其中的 exe 和 Whisper 验收，未通过不可上传或发布。结束工具默认只关闭本目录记录并核对的实例，等待已接受请求完成；明确应急确认后才按进程编号强制结束，且再次核对程序路径。`--quiet` 支持自动验收，不会自动同意强制终止。
