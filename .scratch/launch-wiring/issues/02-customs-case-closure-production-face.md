# 02 CC 关案编排无装配

Category: enhancement
Status: resolved——2026-10-28 进 main（`2e24f5941`，launch-wiring 01–04 同批；全量 160 ok / 0 FAIL，种子退 0），见文末「进 main 记录」。此前 in-progress——2026-10-28 作者完工，待隔离评审与进 main，分支 claude-lw-cc（代码最后一笔 1017dc84d）。此前：in-progress——2026-10-28 会话 tops-20（idp-parcel-91）立并认领（用户「现在就接线」），分支 claude-lw-cc
Blocked by: 无

## 做什么

把 `CloseCustomsCaseHandler` 接成生产入口（UC-CC-010 定触发方：运营命令口或派发消费者，照 UC 选），带 code/03 的冲突项阻止关闭；生产装配真库用例。授予与种子同 01 的写法。

## 完成判据

- 生产路径能关案；有未解决或冲突项时答被阻并列出各项（真库用例）。


## 完成记录（2026-10-28，作者子代理，分支 claude-lw-cc）

起点 origin/main `2d0193336`（本票叠在 01 之后）；代码两笔 `d7ebe073a`、改号 `1017dc84d`。

### 改了什么
- 触发方照 UC-CC-010「关闭触发：有权角色请求关闭」选**运营命令口**，不是派发消费者（`cmd/parcel-dispatch` 不动）。
- accessidentity：运营决定加 `CUSTOMS_CASE_CLOSURE`；迁移 `access_identity/0032`（取 0030 全集并入 `origin/claude-lw-no` 在 0031 加的 `NODE_COLLABORATION_ACCEPTANCE`，再加一词）。首版取的 0031 与 NO 分支撞了，后改取更大的 0032；0031 在本分支空着，等 NO 分支进来补上。
- customshttp（新文件 `case_closure_endpoint.go`）：`POST /customs-case-closures`，Intake 挂在既有 `OperatorDecisionIntake` 上，租户与决定人取认证信封，载荷只有案件与业务截点且封闭；被阻答 200 `BLOCKED` 带 `blockingItems`（未解决项与 code/03 的冲突项，冲突项带冲突依据）。
- parcel-api（新文件）：`endpoints_case_closure.go`、`assemble_case_closure.go`、未接线占位。缝全接真：`ObligationInventoryView`、`CaseClosures`、`OutboxCaseClosureHandoff`、系统时钟；事务壳复用 `customsDecisionTransaction`，关闭记录与关闭意图同笔，未决整笔回滚。共享装配文件零改动。
- 演示种子：access 30/31、governance 67、步骤 485（首版取 28/29、65、484，与 NO 分支撞，后改）。

### 代决与越权风险点
1. 授予与建案分开（关务 CONTEXT「关闭决定角色不由管理员或通用关务角色代替」）；受控重开是另一种权力（原关闭责任来源约束），本票不接，仍无生产入口。
2. 业务截点由请求方在载荷里给，产品不拿处理时刻代替（盘点按截点取当时适用的义务）。
3. 决定人记作「发行方#sub」，只进关闭记录的 decidedBy。未接「逐项关闭责任来源与授权范围」的校验（UC-CC-010 待决项「关闭决定角色、逐项关闭责任来源和授权范围」，租户取值，PAR-CUS-06）。
4. 义务目录与逐项依据是租户取值，种子不种任何一行；目录没登记时口答 `UNDECIDED` + `OBLIGATION_INVENTORY_UNCONFIGURED`。
5. 未立新 ADR、未立新 AT 号（测试 `Covers:` 引 UC-CC-010）。

### 判据对用例
- 「生产路径能关案」：真库用例 `TestACustomsCaseIsClosedOrBlockedThroughTheProductionFace`——全部终结或有效承接答 201 `CLOSED`，关闭记录带操作者，关闭意图同笔入 Outbox，重放答 `ALREADY_CLOSED` 且不多入意图。
- 「有未解决或冲突项时答被阻并列出各项」：同用例——一项未解决、一项冲突（带 `SYN-FACT-A`、`SYN-FACT-B`）答 200 `BLOCKED` 列两项，库里无关闭记录。
- 目录未登记答未决、不落记录；只持建案授予关案答 403。

### 自验
`go build ./...`；`go vet` customscompliance、accessidentity、cmd/parcel-api；`gofmt -l` 无输出；带 PG DSN `go test -count=1 -p 1` 跑 `./internal/customscompliance/...`、`./internal/accessidentity/...`、`./internal/architecture/...`、`./migrations/...`、`./cmd/parcel-api/...` 全绿；种子灌一次性库退 0 后删库；`at-coverage-counts -check` 无输出。

### 留给协调方
- 无需立票的新发现。受控重开仍无生产入口（UC-CC-010 的另一半），不在本批。

## 评审记录（2026-10-28，隔离子代理，钉 `f977c587c`，01–03 同一评审）

两轴无阻断。关闭与建案分开授予；目录未登记答未决不落记录；BLOCKED 列未解决与冲突项（带依据）；CLOSED 落 `decidedBy`、记录与意图同笔、重放意图恰一份。受控重开仍无入口；逐项关闭责任来源与授权范围不校验（租户取值 PAR-CUS-06），已披露。

## 进 main 记录（2026-10-28，推送方 tops-20 / idp-parcel-91）

- 与 launch-wiring 01、03、04 同批：隔离树上先 cherry-pick `claude-lw-cc`（tip `f977c587c`）、再 `claude-lw-no`（tip `272f98c15`）到 main `322b5695c`；`internal/accessidentity/operation_decision.go` 的 `decisionKinds` 文本冲突按并集解。access_identity 迁移 0030（01）、0031（04）、0032（02）、0033（03）依序施加，0033 的 CHECK 为全集（评审逐词核过）。
- 重生成 ADR 索引、AT 盘点计数与机制清点，有变化的各成一笔；两项 `-check` 退 0。
- 全量 `scripts/test-full.sh`：160 ok / 0 FAIL。种子灌一次性库退 0，用后删库。
- 推确切 SHA `2e24f5941` 到 main。
