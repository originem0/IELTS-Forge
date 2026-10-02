# 开发与验收

更新日期 2026-10-02。架构、开发和验收统一维护在本页；已完成的阶段计划、审查与维护日志不再分别维护。版本历史由 Git 保留，当前变更见 [更新说明](RELEASE_NOTES.md)。

## 项目结构

前端是无需构建的原生 JavaScript、HTML 和 CSS，后端使用 Go 标准库。支持 Windows 10/11 x64，完整包包含启动器、结束工具、whisper.cpp CPU 引擎及 small.en 模型。没有新增应用运行时依赖。

| 路径 | 职责 |
| --- | --- |
| `app/index.html`、`styles.css`、`desktop.css` | 页面结构、响应式样式与首页入门指南 |
| `app/app.js`、`practice-lifecycle.js` | 四科入口、状态协调、自动保存、计时和导航屏障 |
| `app/storage-client.js`、`state-schema.js`、`media-client.js` | 增量写入、旧数据校验、媒体引用 |
| `app/library.js`、`library-import.js`、`objective-practice.js` | 选题、导入、听读作答、模拟与复盘 |
| `app/assessment.js`、`ai-client.js`、`ai-settings.js` | 具体 Task/Part 评分要求、AI 调用与两套接口配置 |
| `app/review-workspace.js`、`review-annotations.js`、`markdown.js` | 固定报告结构、原文定位与安全 Markdown |
| `launcher/storage*.go`、`study_*.go` | 档案 revision、独立记录与媒体、恢复点和回收 |
| `launcher/library*.go`、`backup.go` | 不可变题包、来源、导入、索引、判分、删除保护和备份 |
| `launcher/ai*.go`、`credentials_windows.go`、`transcription*.go` | 三种接口协议、视觉验证、DPAPI 与本机转写 |
| `scripts/convert_question_banks.py`、`package_question_bank.py` | 本地素材转换及单个完整题库 ZIP |

## 数据约束

所有学习状态通过绑定目录的 API 写盘。主档案使用 revision 防止多标签页互相覆盖；Windows 文件锁阻止多个程序同时绑定同一目录。新的媒体与记录独立保存，旧内嵌录音、图片及未知字段兼容读取，迁移保留恢复点。

四科切页和档案切换等待真实保存完成。录音、转写和 AI 操作由共同屏障管理，保存失败保留原稿。计时依据时间锚点计算，后台节流不改变实际经过时间。

题库版本不可变，选题按来源与单元 ID 展示最新状态，旧记录保留原题。删除题包检查当前写说档案、听读记录、独立记录版本、滚动备份及每日备份。存在引用时拒绝删除，使用新版本的 `hidden` 标记退出选题。

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
```

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
