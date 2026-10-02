# 本地题库格式与导入

在导航中的“题库与导入”选择一个完整 ZIP 或文件夹，程序识别 JSON 并自动配对媒体，预览后保存。普通 PDF、Word、网页或散装录音不能直接变成计分题。说明和审查 JSON 自动跳过。题库包与学习档案备份不同，备份须在“AI 与数据设置”恢复。没有题包时，可以先从首页“雅思入门指南”体验原创热身。

## 归档与媒体

ZIP 可包含 `packs/*.json`、`media/*` 和说明文件，不需要嵌套多个 ZIP。上传总量和解压后总量各限 1 GiB，最多 5000 个条目；单媒体限 128 MiB。路径穿越、符号链接和不安全路径被拒绝。

媒体命名为 `SHA256.扩展名`，支持 MP3、WAV、Ogg、WebM、PNG、JPEG 和 WebP。以实际内容识别格式，不能只改扩展名。无 ID3 标签的 MP3 通过连续有效帧头识别，原始字节、哈希和录音质量保持不变。附件流式写盘，完整题包及引用校验成功后才提交。

重复导入相同内容不会生成副本。上传成功但响应丢失时，原回执可重试确认，不需要重新上传。

## 题包字段

根对象为 `version: 1`、`title`、`source`、`units`，可带 `exams`。`source` 必须有 `name` 与 `status`，后者为 `unverified`、`verified` 或 `generated`，可附 `url`、`license` 和 `provenance`。`verified` 是素材来源声明，不是软件或官方认证。

每个单元有稳定 `id`、`skill`、`part`、`title`、`prompt`、`minutes`，可含 `provenance`、`hidden` 和媒体引用。

| 科目 | `part` | 内容要求 |
| --- | --- | --- |
| 阅读 | `academic`、`general` | `passages: [{id,text}]` 与完整 `groups` |
| 听力 | `1`、`2`、`3`、`4` | `audio` 与 `transcript`；缺少题组时为不评分的听音资源 |
| 写作 | `Task-1-Academic`、`Task-1-General`、`Task-2` | Academic 图表题保留真实 `images`；General 保留完整书信情境、三个要点及指定称呼；Task 2 保留完整题干 |
| 口语 | `p1`、`p2`、`p3` | 完整题目；Part 2 保留提示卡，`related` 可引用本包相关单元 |

两种 Task 1 默认 20 分钟、至少 150 词，Task 2 默认 40 分钟、至少 250 词。General 不要求图表附件。写作与口语可在应用中手填并存为“我的题目”，Academic 题图上传后按内容 ID 引用；听读不支持手工造题。

## 来源与年份

`source.provenance` 是默认信息，`unit.provenance` 可逐题覆盖。

```json
{"year":2025,"yearKind":"publication","category":"authentic","evidence":["https://example.org/source"],"note":"年份为版次出版年；填写真实核验说明。"}
```

`yearKind` 为 `exam`（考试年）、`publication`（出版年）、`season`（题季年）、`created`（编制年）或 `collected`（收录年）。`category` 为 `authentic`（核验真题）、`recall`（回忆题）、`generated`（生成练习）、`practice`（练习素材）或 `supplied`（手动录入）。

真题标注必须提供证据链接、非收录年份和说明；程序只校验这些字段，不会自动证实网页内容。官方公开样题也不等于某年考场真题。缺少信息的新导入题包按收录年保守补齐，旧不可变版本保持可读。

## 题组与评分

`groups` 包含 `id`、`kind`、`instruction`、`questions`，可带 `context`、`options`、`table`。题型为 `text`、`single`、`matching`、`multiple`。小题包括 `id`、`label`、`text`、`answers`，可附 `options` 和 `evidence`。

选项 ID 与文字分离。非多选题的 `answers` 是允许的答案变体，多选题则是全部正确项；按正确选中项计分，不能超选。填空用 `maxWords`、`allowNumber`、`numberOnly` 表达题面限制。只归一化大小写及空白，不猜测语义近似答案；单复数和拼写变体必须得到来源支持。

`table` 用二维文字数组保留表格，`images` 保存原图引用。题号保持来源编号，不能把跨篇题号或题干年份误改。答案与原文证据需独立核验，结构通过不等于答案正确。

完整试卷通过 `exams: [{id,title,skill,unitIds,minutes}]` 声明。阅读为同类三篇、60 分钟；听力为 Section 1–4 的四段不同录音、20–60 分钟。题号必须连续覆盖 1–40。未核验来源不进入模拟；生成题始终标为生成，不能用于正式估分。

## 版本与清理

内容修改产生新的不可变题包。选题按同一来源、同一单元 ID 选择最新状态，旧练习继续读取原版本。`hidden: true` 使该题退出选题，恢复时再次保存新版本。

删除 API 只允许移除未被当前档案、练习、独立记录版本、滚动或每日恢复点引用的题包，否则返回 409。不能直接删除 `library/packs` 或媒体目录来清理选题。

## 本地转换、单 ZIP 与验证

`scripts/convert_question_banks.py --help` 列出支持的本地输入。转换只读取数据，不运行外部来源代码；缺图、缺音频、词数规则不明或答案不完整时记入审查清单。听音资源可用 `--listening-resources` 配对录音和 LRC，机器转写需人工核对。

写作转换先根据明确类型、General Training 标记或书信指令区分任务。General 保留完整文字，不要求图片，也不拿旁边的图表充当附件；Academic 仍须配套真实题图。标题中单独出现 general 一词不代表书信题。

整理为 `packs/` 和 `media/` 后，使用标准库脚本生成一个完整 ZIP。脚本校验媒体哈希、重复单元和大小，只收录实际引用附件，可附 `README.md` 与 `quality-audit.json`。

```powershell
python scripts/package_question_bank.py --source 本地整理目录 --output question-banks/完整题库.zip
```

生成的 `archive-manifest.json` 记录数量。验收命令见 [开发与验收](DEVELOPMENT.md)，直接测试最终 ZIP，不依赖下载缓存。
