# declaration-submission 出账信封 ID 补版本维（或改由版本认领）

Category: bug
Status: resolved
Blocked by: 「原案内更正/补充」编排落地票（尚未立案——那张票动存储键与 `SOURCE_CONFLICT` 分支时，本票随之动工；见下）——该编排已于 2026-08-25 随用户授权直接落地（未另立票），本票同笔关坑

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
- 2026-08-25 MCP-1（同日晚些，用户批复「继续」把更正编排排入第二档并授权施工，票转
  resolved）：**编排与修复同笔落地，与上一条「同笔定形」的裁定一致**——`5a14afb`
  （CorrectDeclarationHandler + 迁移 0012 多版本存储 + `declarationSubmissionEventID`
  补版本段/分区键保持三维 + `FindByVersion`/`SaveCorrection` + VE 消费侧按版本读回并把
  `CorrectedFrom` 登记为来源事实替代关系）、`efdbfa7`（dispatch 期望 ID、分区键门禁例外
  表摘行、parcel-api 播种补 is_current 三处涟漪）。本票修复面四项逐一落实：eventID 拼法
  加版本段✓、handoff 测试断言（idempotent/atomic/rollback/foreign 及版本维专项）✓、载荷
  未动（versionId 原在）✓、消费侧按载荷版本维立账并按版本读回✓；submit_declaration.go
  「修订走撤销重报」注释口径已随编排修正（保留身份的修订指向更正编排）。验证：提交态
  worktree 全仓 go test -p 1 -count=1 零 FAIL（79 包 ok，含真库；更正往返用例 -v PASS
  非 SKIP 出示）。原「重启条件」由用户直接授权兑现，未另立编排票——工作在本票与提交信
  中记账。
