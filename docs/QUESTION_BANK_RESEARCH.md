# 免费雅思题库来源核验

核验日期：2026-09-30。目标是为个人本地选题提供材料，不是寻找需要整体部署的新学习平台。

本次核对了 GitHub 仓库目录、实际题目文件、文件提交记录、原始题目页面，以及论坛正文。下文的数量来自实际文件统计或明确标注的站方说明。仓库更新日期不等于题目所属题季。

## 优先候选

### 1. 当季口语：murph232439/ielts-listening

- [仓库](https://github.com/murph232439/ielts-listening)
- [实际数据所在文件](https://github.com/murph232439/ielts-listening/blob/main/index.html)
- [2026-09-19 文件提交](https://github.com/murph232439/ielts-listening/commit/72261fa410b73dccede953f9ffee1479e4a6062b)

页面标注数据来源为新东方口语题库，题季为 **2026 年 9—12 月**。它虽然叫 listening，内容实际是口语问题及题目朗读，不是雅思听力试卷。

题目直接嵌在 HTML 的 `var G` 数组中。无需运行对方程序即可提取。实际统计如下。

- Part 1：43 组话题，236 个问题。
- Part 2：66 张完整题卡，包含 cue card 提示。
- Part 3：66 组配套讨论，共 272 个问题。
- 数组共 175 个分组，不能解释成 175 个相互独立的话题。
- 字段包括 `part`、`cat`、`title`、`tags`、`lines`、`cue`；问题包含英文、中文及音频链接。

原站交叉核验：[嘈杂之地](https://ielts.koolearn.com/20260917/868086.html)、[攒钱购买礼物](https://ielts.koolearn.com/20260917/868087.html) 均发表于 2026-09-17。仓库包含对应题目。只抽查了部分题目，未确认整库完整性或全部题目的准确性，不能称为官方完整真题库。

音频是新东方 CDN 外链，未逐一验证可用性，也没有随仓库保存成离线附件。GitHub 元数据未显示 LICENSE；第三方题目和音频的再分发权限未核实。适合作为个人导入候选，不应直接认定可以随项目发布。

### 2. 阅读：hwttop5/ielts-reading-past-papers

- [仓库](https://github.com/hwttop5/ielts-reading-past-papers)
- [题库清单 manifest.json](https://github.com/hwttop5/ielts-reading-past-papers/blob/main/src/generated/reading-native/manifest.json)
- [题目文件目录](https://github.com/hwttop5/ielts-reading-past-papers/tree/main/src/generated/reading-native/exams)
- [解析目录](https://github.com/hwttop5/ielts-reading-past-papers/tree/main/src/generated/reading-native/explanations)

清单实际包含 **217 篇阅读、2917 道小题**，这是篇数，不是 217 套完整考试。清单生成时间为 2026-07-03，仓库最近推送时间为 2026-08-10；不能据此称为 9 月当季题库。

题目 JSON 包含 `passageBlocks`、`questionGroups`、`questionItems`、`options`、`answerKey` 等字段。抽查 `p1-high-01.json`，文章、13 个题目和 13 项答案均存在，并有独立中文解析文件。未逐题验证全库答案正确性。

可以写转换器提取数据，不必安装它的 Vue、AI 助教或数据库。原文以结构化 HTML 节点保存，转换比简单题目文本多一步。仓库声明 GPL-3.0，但不能把代码许可证自动理解为全部第三方题目素材的授权。

### 3. 写作：IELTS Liz 的 Task 2 题目目录

- [100 IELTS Essay Questions](https://ieltsliz.com/100-ielts-essay-questions/)

页面公开可读，站方以 100 道作文练习题为目录，按话题和作文类型组织，具体内容分布在链接页面。它明确是多年积累的练习材料，不是 2026 年 9 月考试回忆。

适合整理为写作收藏夹。需要继续抓取目录下的题目页，提取题目与分类；目前只核对了目录，未完整遍历、去重或统计全部子页面。

### 4. 官方样题：IELTS Academic sample test questions

- [官方学术类样题页](https://ielts.org/take-a-test/preparation-resources/sample-test-questions/academic-test)
- [官方样题总入口，含培训类](https://ielts.org/take-a-test/preparation-resources/sample-test-questions)

免费公开，适合建立基础题库和验证练习界面。学术类页面提供按题型划分的练习、答案 PDF、听力原文 PDF 等；部分练习在 Inspera 播放器中。不是定期更新的完整当季题库，也不是现成 JSON。

本次确认了页面的附件链接，没有逐个下载全部 PDF 或验证播放器中的媒体离线导出。

## GitHub 补充候选

| 来源 | 核验结果 | 适合程度 |
| --- | --- | --- |
| [HowarYe/HRU-IELTS-QuestionBank](https://github.com/HowarYe/HRU-IELTS-QuestionBank) | `data/questions.json` 有 122 张题卡，其中 51 张 Part 1、71 张 Part 2/3。数据明确标注快照为 2026-06-26，仓库推送为 07-29。记录的原来源页面本次返回 404。 | JSON 很好转换，但应作为旧题练习，不能称为当前题季。 |
| [kekemigo/ielts-speaking](https://github.com/kekemigo/ielts-speaking) | `question-bank.json` 有 4 个话题、18 道 Part 1，附中文、参考回答和词组。文件最近提交为 2026-09-21。README 说明题目来自用户截图，回答为原创示例。 | 适合轻量补充，但体量小，也没有证据表明它覆盖整个当季。 |
| [tutuhua/IELTS-Speaking-Lab](https://github.com/tutuhua/IELTS-Speaking-Lab) | 明确标为 2026 年 5—8 月，README 声称 55 张 Part 2 题卡和 44 个回答素材，题目嵌入 HTML。 | 旧季补充；不把作者自己的回答素材导入个人经历库。 |
| [LuchoBazz/ielts-ai-dataset](https://github.com/LuchoBazz/ielts-ai-dataset) | 明确为 AI 生成；有听读写 JSON、写作图片、听力 MP3。抽查听力 manifest 含 transcript 和 sections，目录有对应的四段 MP3。LICENSE 为 CC BY 4.0。 | 适合测试导入器和补充模拟练习，必须标为生成题，不能当真题或评分校准依据。 |
| [songf3680-bot/ielts-workbench](https://github.com/songf3680-bot/ielts-workbench) | `outputs/data/questions.js` 实际含 1274 项，776 项写作、498 项口语，存于 `GENERATED_QUESTIONS`。本次未核清每题原始来源。 | 数量大不能代替真实性，不列为首选。 |
| [sallowayma-git/IELTS-practice](https://github.com/sallowayma-git/IELTS-practice) | 09-29 仍更新，README 支持阅读和可选本地听力扩展；主分支文件树中未发现 MP3/WAV/M4A 或 PDF。 | 可参考导入设计，但不能宣传为直接附带完整听力音频的免费包。 |

## 论坛与网页来源

- [Reddit：2026-09-25 的资源求助帖](https://www.reddit.com/r/IELTS/comments/1wpqh4f/looking_for_comprehensive_resources/)的已读取回复指向版主资源索引，没有提供新的完整题库包。
- [Reddit 资源索引原帖](https://www.reddit.com/r/IELTS_Guide/comments/1csszkv/practice_resources_for_ielts/)发表于 2024-05-15，包含官方练习入口，也混有付费课程和评估服务。可以用来找线索，不能把整帖概括为免费题库。
- [IELTSNetwork](https://www.ieltsnetwork.com/) 首页的近期题目板块最后帖子标在 2020 年，虽然整个网站显示 2026 年时钟，也有新的课程公告。没有把它列为最新题库来源。
- [IELTS Buddy 论坛](https://www.ieltsbuddy.com/ielts-forum.html)有分科讨论、作文反馈入口；本次未确认现成批量导出的当季题库。
- [IELTS-Blog 考试回忆](https://www.ielts-blog.com/category/recent-ielts-exams/)公开可读，有考生贡献的题目。本次页面显示最后更新为 2026-05-12，不按当前日期推测它已更新到 9 月。
- [新东方口语目录](https://ielts.koolearn.com/kouyu/)存在 2026-09-17 的单题页面，正文无需登录即可读取，适合后续少量增量补题。整包领取入口与单题网页的获取条件不同，本次未登录或领取整包。
- [IELTS Online Tests](https://ieltsonlinetests.com/)宣传免费在线练习，但[听力 PDF 整包页面](https://ieltsonlinetests.com/product/ielts-listening-test-super-pdf-pack)有购买流程。不能把免费在线做题解释为完整下载包免费。

## 对当前项目的建议

第一批优先处理当季口语仓库的文字题目，以及少量经过核对的写作题。阅读 JSON 可作为后续阅读模块的候选输入，不需要把外部项目整体引入。

统一题库数据保留稳定 ID、科目、Task/Part、题目、完整提示、题目组关联、来源 URL、来源题季、获取时间和验证状态。口语 Part 2 与配套 Part 3 的关联应保留。题目保存与用户答题记录分开。

来源中的示范答案不自动进入用户个人语料库。外链音频要与题目文字分开处理，不能仅保存 URL 就声称已经支持离线。

当前项目的“导入备份”仍用于替换学习记录，不支持上述外部格式。此次仅产出来源研究，尚未实现题库导入器。
