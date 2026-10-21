# 35 PS 取消与边界事实的先后冲突：面单渠道一侧不比时间、冲突没有解开的决定面

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-code21（见文末「完成记录」；解开冲突的决定面拆到 [code/76](./76-a-cancellation-order-conflict-has-no-resolution-decision-face.md)）。此前 needs-triage——建议 R2（上线后尽快）：看得见的卡住。网络服务一侧，取消已提交之后才到、发生不晚于取消的收寄，采用停在未决 `CANCELLATION_ORDER_CONFLICT`，此后没有任何口能解开它，包裹在节点手里却没有责任起点，也不触发收寄后的路由复核；面单渠道服务一侧，同一情形今天答 `CANCELLATION_STANDS`，客户被告知已取消、承运商却已收走。今天取消授权停在未配置，取消形成不了，两种情形都还到不了；租户一登记取消授权即生效。2026-10-21 文档自洽 06 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc06。分级由协调方定稿。
Blocked by: 无（面单渠道提交边界那一半与 [code/21](./21-label-channel-parcel-cancellation-ignores-carrier-pickup-and-finals.md) 同一编排，宜同批或排在其后）
出处：[audit/ps.md](../audit/ps.md) PS-03（B），代决 [ADR-0254](../../../docs/adr/0254-cancellation-and-boundary-fact-order-is-judged-by-their-business-times-and-a-late-earlier-boundary-fact-is-a-fact-conflict.md)；UC-PS-003「给开发的交接」、UC-PS-004「给开发的交接」`AT-PS-108` 一行、UC-PS-006「给开发的交接」事实冲突一行。

## 现象

ADR-0254 定了取消与边界事实按两者的业务时间比、晚到的更早边界事实是事实冲突。代码在网络服务一侧已经这样判，但冲突之后无路可走；面单渠道一侧还不比时间。

## 证据

- 网络服务：`internal/parcelshipment/application/adopt_network_intake.go` 的 `AdoptNetworkIntakeHandler.Handle` 读到已成立的取消，`cancellation.RequestedAt()` 早于 `source.OccurredAt()` 即不采用（`CANCELLED_BEFORE_INTAKE/<取消标识>`），否则答 `ELIGIBILITY_UNDECIDED`、原因 `CANCELLATION_ORDER_CONFLICT`。注释写「收寄发生早于取消决定」，比的却是取消的业务时间。
- 冲突之后：没有命令口、运营决定面或续办信封能把这一格解开；同一来源再投仍落在同一格。`internal/parcelshipment/adapters/adoptconsume/adoptconsume.go` 的 `Consumption` 把 `IntakeEligibilityUndecided` 一律译成未决哨兵、回滚重投，这一格因此会重投到失败预算耗尽，而重投推不动它。
- 面单渠道：`internal/parcelshipment/domain/label_service_final.go` 的 `JudgeLabelServiceFinal` 在 `input.CancellationStands` 为真时直接答 `CANCELLATION_STANDS`；`application/judge_label_service_final.go` 只读有没有有效取消，不读取消的业务时间，也不比承运商首次有效收寄的发生时间。
- 文档：PS CONTEXT「形成取消决定前……」一条，UC-PS-003 `AT-PS-107`，UC-PS-004 `AT-PS-095`、`AT-PS-108`，UC-PS-006 决定矩阵。

## 三类

机制。产品策略：解开冲突的决定形状（谁有权、去向有哪几个）由本票落地时按 ADR-0254 决定三提出、交 PS owner 认可。租户取值：谁持有这个授权，属 `PAR-COM-14` 一类，未登记答未配置。

## 规模

M：面单渠道终局判断多比一次时间并多一格不形成原因；收寄采用消费门对冲突入账；一个授权决定面（命令、授权、只追加记录、续办信封）；收寄采用那段注释改口。

## 最小的诚实修法

1. 面单服务终局判断读到有效取消时，读出取消的业务时间，与承运商首次有效收寄的发生时间比：收寄晚于取消，照旧 `CANCELLATION_STANDS`；不晚于取消，不形成终局并指名冲突——宜作 `LABEL_SERVICE_NOT_FINAL` 加一格新的不形成原因（例如 `CANCELLATION_ORDER_CONFLICT`），这样 ADR-0134 决定二的翻译表照旧把它译成已消费，不必另加一格结果；不要答 `JUDGMENT_UNDECIDED`，那一格按 ADR-0134 是重投会改变结果的读口未决。
1a. 收寄采用一侧：冲突那一格入账、留下冲突，不进未决哨兵（按恢复动作，ADR-0254 决定二）；可以另立一格结果，或在 `Consumption` 里按原因分出，判据只放一处。
2. 立一个授权决定面解开冲突，去向至少有：认定取消依据不成立、包裹转入收寄后服务处置（随后重驱收寄采用或终局判断）；认定边界事实有误、交来源方更正。决定只追加，不改写已提交的取消。
3. `adopt_network_intake.go` 那段注释的「取消决定」改为「取消的业务时间」。
4. 补用例：网络服务冲突后经决定解开；面单渠道收寄不晚于取消时不形成终局、指名冲突（`AT-PS-108`）。

## 要裁的

- 解开决定的去向集合与授权：本票落地时提出，交 PS owner 认可（ADR-0254 越权风险点 2）。

## 完成记录（2026-10-21，分支 claude-code21，rebase 于 main `8d68c7bab`；与 code/21 同批）

### 做了什么

- `575017e9c` 判法与入账：
  - 领域 `JudgeCancellationOrder`（`domain/cancellation_order.go`）是取消与边界事实先后的唯一判据：两边都在册之后只比取消的业务时间（`RequestedAt`）与边界事实的业务发生时间，严格晚于取消是取消在先，不晚于（含相等）是冲突，冲突带回两侧各自的那一代与业务时间。
  - 面单服务终局判断（`JudgeLabelServiceFinal`）的输入从「有没有有效取消」换成整份取消；带着收寄来时用它比，冲突答 `LABEL_SERVICE_NOT_FINAL`、新原因 `CANCELLATION_ORDER_CONFLICT`，verdict 交回冲突；取消在先照旧 `CANCELLATION_STANDS`（`AT-PS-095`、`AT-PS-108`）。
  - 收寄采用编排改调这道判据；冲突先记进冲突册（新端口 `CancellationOrderConflictRegister`）再答 `ELIGIBILITY_UNDECIDED`/`CANCELLATION_ORDER_CONFLICT`。新方法 `IntakeUndecidedReason.AwaitsDecision` 标出这一格，采用消费门（`adoptconsume.Consumption`）对它入账、不回滚。那段注释「取消决定」改为「取消的业务时间」。
  - 两侧冲突记不下（册未接入或写不进）都答依赖未决，新原因 `CONFLICT_REGISTER_UNAVAILABLE`，照常回滚重投。
- `e84c3ba62` 冲突册落库与生产接线：迁移 `migrations/parcel_shipment/0029_cancellation_order_conflict.sql`（只追加，主键（租户，包裹，边界种类，边界那一代，取消），CHECK 钉住边界事实不晚于取消、种类封闭），适配器 `pspostgres.CancellationOrderConflicts`；parcel-dispatch 的收寄采用编排与面单终局判断核接真册。
- `d8b3be234` 拆出 code/76（解开冲突的决定面）。
- `7d6c1d7f2` UC-PS-003 一致性一条与交接表、UC-PS-004 面单渠道结果清单与交接表、UC-PS-006 决定矩阵冲突一行与交接表改为已补，决定面指 code/76。
- `2b799b8d2` AT 盘点 `AT-PS-107` 由②、`AT-PS-108` 由④移入①。

### 代决

用户授权「你作为业务和系统专家自决……解决所有的问题」。

1. 「消费门入账并留下冲突」读作要留存：只入账不留痕，冲突会从重投流里消失、按包裹查不出（ADR-0254 决定三「冲突原因按包裹查得出」），日后的决定面也无从取。所以立了冲突册，入账与记册同一笔事务。
2. 网络一侧不另立结果格，仍答 `ELIGIBILITY_UNDECIDED`/`CANCELLATION_ORDER_CONFLICT`（UC-PS-003、UC-PS-006 都这样写）；「重投推不动」的判据放在原因类型的 `AwaitsDecision` 一处，消费门只问它，不另认原因名。
3. 冲突记不下另立 `CONFLICT_REGISTER_UNAVAILABLE`：与冲突那一格恢复动作不同（修依赖、重投就会记下），不能共用一个原因。
4. 面单一侧照票面建议用 `LABEL_SERVICE_NOT_FINAL` 加一格原因，ADR-0134 决定二的翻译表照旧把它译成已消费，不必另加结果格，也不答 `JUDGMENT_UNDECIDED`。
5. 面单终局判断编排因此有了一个写口（冲突册）。「判断不写」那条结构用例改为只放过 `Conflicts.Record`：册不形成终局，只把判出的冲突留下。
6. 交易与关闭两路不带收寄，比不了先后，改为读冲突册（协调方评审 N1 裁定，照 ADR-0254 第 3 格字面）：册上有一行指向这份取消答 `LABEL_SERVICE_NOT_FINAL`/`CANCELLATION_ORDER_CONFLICT`，查不动或册未接入答未决 `CONFLICT_REGISTER_UNAVAILABLE`，册上没有才答 `CANCELLATION_STANDS`。同一包裹不论哪一路触发都答同一格；这两路只读册，不新记冲突。
7. 取消视图答「有取消」却交不出取消标识时响亮报错（读口坏了），不当作没有取消——当作没有会让收寄盖过一份真取消。
8. 边界那一代的引用：网络一侧 `<来源种类>/<来源版本>`，面单一侧 `<收寄事实>@<版本>`（与执行证据引用同形）。
9. 决定面拆成 code/76，没在本票做：它要先定一个 ADR-0254 有意没定的形状（谁有权、去向有哪几个、各去向驱动什么重判），是要 PS owner 认可的产品策略，体量本身是一张 M 票；与判法捆在一起，判法就得等认可才能进 main，而判法不进 main，冲突照旧回滚重投到失败预算耗尽、在库里不留痕。

### 越权风险点（待 PS owner 复核）

1. 冲突册的归属：册与写口随本票立，读口 `ListByParcel` 今天只有用例与日后的决定面会用。若 owner 认为册该随决定面一起设计，改的是端口与表的形状，判法不变。
2. 交易与关闭两路照册作答（代决 6），所以冲突册成了这两路的读依赖：册读不动时它们停在未决、回滚重投，而改动前它们直接答取消在先、入账。册上只认指向同一份取消的行，取消换了一份（跨委托复用包裹号那一格）就不算。
3. 判断编排多了写口（代决 5），「判断不写」那条不变量放宽了一格。
4. ADR-0254 越权风险点 2 的缓解今天成立了一半：冲突包裹按包裹查得出、不再回滚重投；没有责任起点、不触发路由复核、面单冲突包裹没有终局，等 code/76。
5. 三处之间不按包裹串行：取消提交与收寄采用或面单终局形成并发时，read committed 下两边都看不见对方未提交的写入，可能同时落下取消与责任起点（或非取消终局），冲突那一格根本不被触发、冲突册里也没有这一行。归 [code/77](./77-cancellation-intake-adoption-and-label-final-are-not-serialized-per-parcel.md)（R2）。

### 迁移

`migrations/parcel_shipment/0029_cancellation_order_conflict.sql`（取 main 现有最大号 0028 + 1）。演示种子在一次性库 `code21seed` 上灌过一遍，干净完成、新表已在，库随后删掉。

### AT

- `AT-PS-107` 入①：`TestTheCancellationBoundaryJudgesByBusinessTime`（含相等、记不下、册未接入三格）、`TestAdoptionOutcomesMapToConsumptionSlots`（消费门入账）、真库经生产 wireDispatcher `TestALateEarlierNodeIntakeIsConsumedAsARecordedConflict`。
- `AT-PS-108` 入①：`TestAStandingCancellationIsComparedWithTheFirstPickupByBusinessTime`、`TestALateEarlierPickupIsAFactConflictWithTheStandingCancellation`、`TestALateEarlierPickupIsConsumedAsARecordedConflict`、真库 `TestALateEarlierCarrierPickupIsConsumedAsARecordedConflict`。
- 判据本身：`TestTheCancellationOrderIsJudgedByTheTwoBusinessTimes`；冲突册：真库 `TestACancellationOrderConflictIsRecordedOnceAndListedByParcel`。

### 验证（分支 claude-code21，两票同批）

- `go build ./...`、`go vet ./...` 干净，`gofmt -l cmd internal migrations tests tools` 无输出。
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh ./internal/parcelshipment/... ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./cmd/parcel-commercial/ ./cmd/parcel-frontline-import/ ./cmd/parcel-settlement-register/ ./tests/bentocontract/... ./internal/architecture/... ./migrations/... ./internal/customscompliance/adapters/parcelshipment/ ./internal/networkrouting/adapters/... ./internal/nodeoperations/adapters/parcelshipment/ ./internal/parcelpricing/adapters/... ./internal/settlementaccounting/adapters/parcelshipment/ ./internal/transportfulfillment/... ./internal/visibilityexception/adapters/...`：75 个包 ok，零失败（改动包与 PS 包的反向依赖全在内，含接线与类型可达两道棘轮）。
- 新用例另跑 `-count=3`（同一脚本，`-run` 点名本两票全部新用例与被改的既有用例，范围 `./internal/parcelshipment/... ./cmd/parcel-api/ ./cmd/parcel-dispatch/`）：全部 ok。

### 未决

- 解开冲突的业务决定面：code/76。
- AT 盘点总览计数未改，由推送方重算；机制清点等生成物未重生成。

## 评审记录

### 第 1 轮（2026-10-21，隔离评审；协调方转达，无阻断）

判法与冲突册与 ADR-0254 逐格一致，变异全红；admin-web 的 tsc、598 条测试与 vite build 由评审方跑过。本票修订：

- N1（协调方裁定照 ADR-0254 第 3 格字面修）：面单终局判断的交易与关闭两路带取消、不带收寄时先查 `Conflicts.ListByParcel`——册上有指向这份取消的行答 `LABEL_SERVICE_NOT_FINAL`/`CANCELLATION_ORDER_CONFLICT`，查不动答未决 `CONFLICT_REGISTER_UNAVAILABLE`，没有照旧 `CANCELLATION_STANDS`。领域输入加 `CancellationConflictRecorded`；用例 `TestTheClosurePathsAnswerTheRecordedConflictToo`（命中、没有、指向别的取消、读不动、册未接入五格）与领域一格。代决 6、越权风险点 2 改写——`d65e36d26`。
- N2：错位的旧头注挪回 `TestLabelServiceFinalRecoveryDiscipline`，改成「除冲突册外没有写口」；新用例配自己的头注——`d65e36d26`。
- N7：「判断不写」守卫放过冲突册时，在字段名之外再断言类型是 `ports.CancellationOrderConflictRegister`——`d65e36d26`。
- N8：UC-PS-003「一致性」一条补 `CONFLICT_REGISTER_UNAVAILABLE`；UC-PS-004 结果清单写明两路照册作答——`1b20cc9e6`。
- N4：另立 [code/77](./77-cancellation-intake-adoption-and-label-final-are-not-serialized-per-parcel.md)（R2），越权风险点 5 指向它——`cfc3472d2`。
- 收口前 `git rebase main`（`8d68c7bab`），无冲突；完成记录里的 SHA 已换成 rebase 后的。迁移号 0029 在 rebase 后仍是 main 最大号 + 1。

修订后验证（分支 claude-code21）：

- `go build ./...`、`go vet ./...` 干净，`gofmt -l cmd internal migrations tests tools` 无输出。
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh ./internal/parcelshipment/... ./cmd/parcel-api ./cmd/parcel-dispatch ./tests/bentocontract/... ./internal/transportfulfillment/... ./internal/architecture/... ./migrations/...`：36 个包 ok，零失败。
- 新用例与被改用例另跑 `-count=3`（范围 `./internal/parcelshipment/... ./cmd/parcel-api ./cmd/parcel-dispatch`）：全部 ok。
