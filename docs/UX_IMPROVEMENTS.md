# 体验优化与新手引导验收

目标：落实 2026-09-30 与 2026-10-01 逐页体验审稿中的全部问题，并给雅思初学者提供简单、可执行的引导。保留当前 Windows 离线能力、已有数据与未提交改动。此文件记录实现和验收，未勾选项目不视为完成。

- [x] 首页：第一次练习优先、返回用户可继续原练习、零统计收起、装饰轨道换成可理解的学习步骤。
- [x] 新手：解释 Academic / General 与四科、Task / Part；提供无需题库和 AI 的原创热身；再引导复盘与计划；有可返回的导航入口。
- [x] 题库：明确支持的资料格式和无题包路径；已导入题库能查看并选题；导入后回到原练习科目。
- [x] 写作设置：所有类型与时间可见；默认输入形式随类型变化；已有题图和明确时间不丢失；紧凑且开始动作可达。
- [x] 写作状态：暂停保持只读；完成后不再显示作答中的计时动作，提供继续修改、反馈和返回。
- [x] 口语：同题重练保留独立 ID、原稿、音频、批改和尝试序号；取消无效英美口音选项；按准备、录音、转写和回听状态展示操作。
- [x] 写说概览：空态有可执行下一步；计划任务带到具体记录；每日语料可回忆、造句并用于练习。
- [x] 听读入口：空题库与模拟入口直接到题库，保留返回科目；选题展示题数、时长和类别。
- [x] 阅读作答：压缩顶部、增加内容高度和答题进度 / 题号定位。
- [x] 听力作答：首屏含实际题目，滚动中播放与回退仍可达；模拟保持原有一次原速播放规则。
- [x] 听读复盘：优先错题对照，原文按需访问并可返回，保留错题重练和真实依据；不虚构时间戳。
- [x] 写说报告：统一紧凑标题与卡片，既定章节顺序、普通滚动、标注双向定位与收藏保留；末尾接同题重练和前次对照。
- [x] 语料库：无数据有行动入口；已有表达可主动回忆、造句、保存进度并进入对应练习；减少套卡。
- [x] 计划：常用目标先填、配额折叠、有计划时先显示安排；支持未知水平，复盘任务连接已有记录。
- [x] 错题与单词：复习时弱化新增表单和重复分类，保持字段可达、自主纠错、揭晓与自评。
- [x] 设置与首次目录：状态和结果优先、技术细节折叠，正常自动保存时不出现手动保存暗示；错误有恢复入口。
- [x] 指南：四科、题库、首次练习、批改 / 无 AI 自查、重练形成可点击路径。
- [x] 视觉：统一标题层级、按钮权重、提示和间距；桌面首屏关键动作可见，窄窗口 / 移动布局无横向溢出。
- [x] 验证：新增针对记录隔离、关键状态、保存失败和学习衔接的验证，完成相关既有检查及真实 Go API 浏览器验收。
- [x] 交付：构建新的 Windows 本地候选副本并验证实际页面，不覆盖使用者数据；核对上述每项的证据。

实施顺序：练习数据和状态 → 入口和新手流程 → 学习与复盘衔接 → 视觉和完整验收。

## 2026-10-01 实现与证据

| 验收范围 | 实现位置 | 验证 |
| --- | --- | --- |
| 首页、新手、四科介绍与原创热身 | `app/index.html`、`app/app.js`、`app/starter-pack.json` | `tests/ux-acceptance.cjs` 从指南创建真实阅读记录，完成后查看依据、返回题目并进入计划 |
| 题库格式、查看选题、返回科目 | `app/library.js`、`app/library-import.js` | `tests/library-ui.cjs` 验证导入、已导入选题；`tests/ux-flow.cjs` 验证空题库与模拟入口返回，不再循环进入导入页 |
| 写作设置、暂停、完成、重写 | `app/app.js`、`app/practice-lifecycle.js` | `tests/ux-flow.cjs`、`tests/speaking-writing-regression.mjs` 验证类型与时间、只读、恢复编辑和独立尝试 |
| 口语准备、录音、转写、重练隔离 | `app/app.js` | `tests/speaking-writing-regression.mjs`、`tests/ux-flow.cjs` 验证同步保存、失败恢复、原稿与批改保持不变 |
| 听读进度、回放、复盘依据 | `app/objective-practice.js`、`app/library.css` | `tests/library-ui.cjs`、`tests/listening-ui.cjs`、`tests/mock-ui.cjs` 验证四类题、错题优先、双向定位、暂停恢复与模拟一次原速播放 |
| 写说报告、前次对照、收藏 | `app/review-workspace.js`、`app/index.html` | `tests/markdown-review.cjs` 保留安全渲染、快照和批注验证；`tests/ux-acceptance.cjs` 验证前次与本次原文对照 |
| 语料回忆、造句、自评、进入练习 | `app/app.js`、`app/styles.css` | `tests/ux-acceptance.cjs` 验证真实磁盘持久化、刷新恢复、保存失败不丢例句，以及带入对应科目 |
| 计划与错题、单词复习 | `app/app.js`、`app/index.html`、`app/styles.css` | `tests/four-skill-plan-ui.cjs`、`tests/plan-budget.cjs`、`tests/correction-notebook.cjs`、`tests/ui-smoke.cjs` 验证配额、计划折叠、复习时收起表单与自评保存 |
| 设置、首次目录与失败恢复 | `app/app.js`、`app/index.html` | `tests/storage-ui.cjs`、`tests/backup-ui.cjs` 验证自动保存、拒绝过期页面覆盖、备份恢复及错误后的重试 |
| 响应式布局 | `app/styles.css`、`app/library.css` | `tests/ux-acceptance.cjs` 验证 1440 / 900 / 390 像素页面，另检查 1280 像素写作作答；截图在 `dist-test/library-v1/ux-*.png` |

测试均使用合成记录和独立临时档案；未调用真实 AI 服务。已有模型响应、Markdown 安全、批改快照与录音转写检查继续保留。验收代码不会修改使用者档案。

## 最终验收与本地交付

- `ELP_BROWSER_TEST=1 go test -count=1 ./...` 全部通过，包括真实 Go API 浏览器验收。使用 `-count=1` 避免 Go 缓存跳过已更新的 JavaScript 场景。
- `go vet ./...` 通过。
- 静态结构、Windows 平台、写说状态、Markdown 报告、错题复习、计划配额、练习生命周期、存储客户端、UX 流程、完整 UI 回归均通过。
- 最终候选包版本为 `ux-complete-20261001-final`。ZIP 位于 `dist/EnglishLearnPath-Windows-x64-Full-ux-complete-20261001-final.zip`；可直接运行的目录位于 `dist/build-28c7e3a4b28e4d45be19e0ca81abed4b/EnglishLearnPath-Windows-x64`。
- `tests/package-smoke.cjs` 已实际运行该版本的 Windows 启动器，完成全部浏览器验收、退出与重启、旧图与旧录音恢复，以及 Whisper 对真实语音样本的离线转写。结果在 `dist-test/package-smoke-SSBxEo/result.json`，实际首页截图在同目录 `packaged-home.png`。
- 包含启动与结束程序、离线 Whisper、模型和许可证。此次为本地候选交付，未发布 GitHub Release。检查时没有正在运行的旧启动器；未覆盖已有解压目录或使用者数据。
