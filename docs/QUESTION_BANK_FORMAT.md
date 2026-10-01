# 本地题包与开发验收

题包 JSON 与配套媒体在设置页的“本地题库”导入。它们与学习档案备份不是同一种格式。附件先保存，只有完整题包通过结构及附件引用验证后才进入题库；失败可以重新选择并重试，不覆盖已有练习。

## 转换本机下载素材

开发脚本仅使用 Python 标准库，不运行来源程序。以仓库根目录为工作目录：

```powershell
python scripts/convert_question_banks.py --reading question-banks/2026-09-30/02-reading-217-passages/original-files/src/generated/reading-native --output question-banks/converted --limit 12
python scripts/convert_question_banks.py --speaking question-banks/2026-09-30/01-speaking-2026-09-12/original-files/index.html --writing question-banks/2026-09-30/05-ai-generated-practice/original-files/datasets --output question-banks/converted
python scripts/convert_question_banks.py --listening question-banks/2026-09-30/05-ai-generated-practice/original-files/datasets --output question-banks/converted/listening
```

生成的 `*-starter.json` 才是题包；`reading-conversion-audit.json` 是审查清单，不要作为题包导入。素材和转换结果都留在被 Git 忽略的 `question-banks/` 中，不随程序发布。

当前阅读转换器保留可明确提取的文章、选择/判断、匹配、填空和多选题，并能识别以表格承载的段落匹配选项，以及被来源错标为 yes_no 但选项与答案实为 TRUE/FALSE/NOT GIVEN 的题组。仍依赖表格/图片布局、没有可靠词数限制、题干无法锚定或选项/答案不完整的材料被记录到审查清单，不能因为转换成功就视为真题或正确答案。写作 Task 2 按文字题干转换；Task 1 仅在本地存在真实配图时转换并以图片附件（SHA256 加扩展名）引用，缺少实际图像的 Task 1 不会用图片描述代替。另可从本地保存的 ieltsliz.com 教育类作文网页快照提取 Task 2，但该来源受第三方版权，仅供本地个人导入，不随项目提交或发布。

## 格式要点

听力导入时选择 `listening-starter.json` 和同目录中的配套音频文件，不选择审查 JSON。当前可用 4 段、40 个计分点，均为生成练习；另有一段缺少题图、三段缺音频，已经排除。实际为 WAV 却使用 MP3 扩展名的来源文件只校正扩展名，不改录音内容。

练习可暂停、回放和调速，播放位置、倍速和答案一起存盘。恢复记录不会自动播放；提交后显示完整来源原文，没有可靠时间戳时不伪造逐句定位。这些练习不等于完整模拟考试。

听力单元可以不含 `groups`：这表示「听音资源」——有音频和原文、但题目只在纸质书上。导入后在听力选题里显示为「听音资源」，点「开始收听」播放并可按需展开原文，不作答也不判分（阅读仍必须带题组）。转换器用 `--listening-resources` 把本地 `*.lrc` 与 `mp3/` 配对成这类资源包，原文为自动转写、可能有误。写作与口语练习页提供「存为我的题目」，把手填题干存进可复用的「我的题目」题包：每次保存生成不可变新版本（最新版在选题中覆盖旧版、历史练习仍指向旧版），写作 Task 1 的配图随保存上传到题库媒体并按内容 SHA256 引用；阅读与听力不支持逐题手动录入。

根对象包含 `version: 1`、`title`、`source`、`units`。来源必须注明 `name` 和 `status`，后者为 `unverified`、`verified` 或 `generated`；`verified` 只是导入材料声明，不是软件审核背书。

每个 unit 有稳定 `id`、`skill`、`part`、`title`、`prompt`、`minutes`。阅读段落保存在 `passages: [{id,text}]`；媒体引用是文件内容 SHA256 加扩展名。听力必须同时有音频和原文。

`groups` 含题组说明、可选上下文、题型和小题。题型为 `text`、`single`、`matching`、`multiple`。选项可属于整个题组或单独小题；选项 ID 与文本分开保存。`answers` 在非多选题中表示允许的答案，在多选题中表示必须选择的所有答案。一道覆盖多个题号的多选题按正确选中项计分，不能超选，不能用选择顺序改变结果。

填空通过 `maxWords`、`allowNumber`、`numberOnly` 表达题面规则。不做语义近似判分。大小写、首尾及重复空白可归一化；拼写、单复数变化必须被来源答案明确允许。

更新题包生成不可变新版本，选题入口对同一来源、同一 unit ID 只显示最近导入的版本；历史练习继续引用旧题包。重新导入完全相同内容不会生成副本。

题组可使用 `table` 二维文字数组保存表格，原题图使用 unit 的 `images` 引用；作答与复盘均保留这些内容。转换器保留来源显示题号，不把跨篇编号重排为从 1 开始，也不会把题干中的年份误删为题号。

整套模拟由题包的可选 `exams` 数组声明，每项包含 `id`、`title`、`skill`、`unitIds`、`minutes`。阅读必须三篇、同属 Academic 或 General、60 分钟；听力必须按 Section 1–4 配齐不同录音，时限按题包设定为 20–60 分钟。各题号必须按顺序连续覆盖 1–40，多选题号范围与计分点数一致。来源为 unverified 的题包不进入整套模拟；生成题始终标记为生成材料，不能用于官方分数校准。

模拟的截止时间由本地服务保存，重新打开不会重置。听力模拟按原速依次播放，作答时没有回退或倍速控件；复盘恢复自由回听。模拟、首答和重练在历史中区分，首页总数覆盖四科。当前下载素材中有缺地图、缺录音及题号/答案结构不一致的内容，不能据目录名称声称已有完整真题试卷。

## 验收

设置页的“导出完整备份”生成包含题库、媒体、练习和旧写说档案的 ZIP，单包最大 1 GB。恢复会在原数据目录下建立新的 `restored-*` 档案并切换绑定，不覆盖原档案。归档路径、校验和、题目引用与媒体完整性全部通过后才切换；API 凭据和启动器配置不在 ZIP 内。旧版 JSON 档案仍可使用原兼容入口。

四科计划保存的是目标上限，实际每日任务会根据时间预算分配并显示暂未排入的项目。整篇写作不会被挤进不足的时间段；已安排时间优先包含复盘，旧计划没有设置听读时不会自动增加这些任务。

常规检查：`go test ./...`、`go vet ./...`（在 launcher 内）、三个现有 Node 冒烟/回归脚本、`python tests/question-bank-converter.py`。

四科时间预算单测为 `node tests/plan-budget.cjs`；完整备份安全与恢复测试包含在 Go 测试中，浏览器的备份与计划验收包含在 `TestLibraryBrowser` 中。

真实浏览器测试使用已有 Playwright 或通过 `ELP_PLAYWRIGHT_MODULE` 指定已安装模块路径，不向项目加入运行依赖：

```powershell
$env:ELP_BROWSER_TEST='1'
$env:ELP_CONVERTED_PACKS=(Resolve-Path question-banks/converted/expanded).Path
$env:ELP_LISTENING_PACKS=(Resolve-Path question-banks/converted/listening).Path
cd launcher
go test -run 'TestLibraryBrowser|TestConvertedLocalQuestionPacks' -v
```

测试使用独立临时数据目录。开启本地素材验证后，所选题包的全部阅读会在浏览器中显示原文、逐题作答并核对结果；测试不改动用户正在使用的数据目录。截图只保存在忽略目录 `dist-test/library-v1/`，后续可按阶段更名。
