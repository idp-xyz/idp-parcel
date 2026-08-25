# declaration-submission 出账信封 ID 补版本维（或改由版本认领）

Category: bug
Status: needs-info
Blocked by: 「原案内更正/补充」编排落地票（尚未立案——那张票动存储键与 `SOURCE_CONFLICT` 分支时，本票随之动工；见下）

## 问题

`declarationSubmissionEventID` 按（租户/单元/程序）三维拼信封 ID，不含版本维；`EnqueueOnce` 按（来源＋事件 ID）先查后插、查到即静默成功。一旦同键出现第二个提交版本并走出账口，第二版意图必然不入队、无错误、无日志分格。

核证结论（[report.md](../report.md)，取证基准 `5073851`）：**机制半边坐实，今天无触发路径。** 当前编排同键异内容答 `SOURCE_CONFLICT`、存储一键一行一版本，同键第二版今天造不出来。真正的引信是 CC CONTEXT 明文预期的「原案内更正/补充」——保留申报单元身份、同键形成新提交版本；那条编排落地时存储必改多版本，若信封 ID 不随之补版本维，第二版意图静默丢失。

上票错引订正：触发场景不是「重报」（重报按 CONTEXT 是新逻辑申报目标、新键新信封），是「原案内更正」。

## 修复面（最小爆炸半径，属 CC owner）

- `declarationSubmissionEventID` 拼法（加版本段，或改由版本认领）——单函数；
- 配套 `declaration_submission_handoff_test.go` 断言；
- 载荷不动（versionId 已在）；
- `submit_declaration.go` 里「修订走撤销重报」的注释口径与 CONTEXT「原案内更正」的偏差，随更正编排票一并修（核证报告 (c) 已点名）；
- 消费侧 inbox 立账维度不在本票：按 [ADR-0066](../../../docs/adr/0066-multi-object-envelope-unrolls-per-member-on-the-consumer-side.md) 的 declaration-submission 接入票走，按载荷版本维立账。

## Comments

- 2026-08-24 MCP-1（清余票触审）：`draft` 转 `needs-info`，统一余票口径——按 triage-labels，
  `draft` 只用于两阶段发布过程，本票实为「阻塞者尚未立案」的停放。机制半边坐实、今天无触发
  路径的核证结论不变（票面引 report.md，基准 `5073851`）。重启条件即 Blocked by 那张
  「原案内更正/补充」编排票立案并动存储键与 `SOURCE_CONFLICT` 分支，届时本票随之动工。
- 2026-08-25 MCP-1（**受用户委托裁断**，用户批复「你来确定」）：**维持停放，不提前排期
  更正编排票，也不单修信封 ID。** 两条理由：①信封 ID 的版本段要与多版本存储键同笔定形
  ——提前替一张未立案的编排票预设键形状，正是 ticket-families 族一（键取自消费方问话而非
  提供方发布单位）要防的错误温床；②「原案内更正/补充」是 CC 的一片新机制切片，排期归
  开发主线的优先级序，不由一张今天无触发路径的停放票倒逼。本票状态照旧 needs-info，
  重启条件不变——这是裁定后的停放（裁的是「不提前」），不是无人裁的悬置。
