# 121 先于段登记的出发永不补问门禁——本该阻断也不形成阻断信号

Category: enhancement
Status: in-progress——2026-10-26 作者完工，待隔离评审与进 main，分支 `claude-dcg121` tip `e3bba49c7`（其后一笔只改本票面与票 114 追记），见文末「完成记录」。此前 in-progress——2026-10-26 会话 `tops-20`（Claude Code）认领（票 135、136 已进 main），分支 `claude-dcg121`（worktree `idp-parcel-dcg121`）。此前 ready-for-agent——2026-10-20 代决复核撤掉用户决定阻断（M5），见 [owner-review-20261020](../../owner-review-20261020/report.md)。此前 needs-triage——2026-10-20 票 [53](./53-movement-facts-recorded-before-their-segment-never-reach-tracking.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [ADR-0224](../../../docs/adr/0224-tracking-backfills-movement-facts-recorded-before-their-segment-when-a-control-fact-puts-the-object-in-it.md) 决定五与越权风险点 3。
Blocked by: 无——用户决定一格已于 2026-10-20 代决撤掉（M5）（此前 用户决定（TF、CC owner：补问机制的形状——ADR-0205 越权风险点 2「补问要另立机制」））
地盘：TF 出发门禁读数（`internal/transportfulfillment/application/record_movement_fact.go`、`domain/departure_gate_reading.go`、移动事实登记册的读数列）；VE 出发门禁阻断信号（`DeriveOnMovementFactAdapter.signalBlocked`）。

## 现象（读码钉票 53 的分支，只记 `S`）

- 出发的门禁读数在登记那一刻问关务（ADR-0205 决定二）。声明的段此刻不在册时，本侧停下不问，读数落未决、停点 `SEGMENT_NOT_REGISTERED`，没有逐对象那一格。
- 票 53 让这类出发在对象日后凭交接或对象级场外揽收进段时补进投影（ADR-0224），但不补问关务：读数仍是那一份未决，阻断信号只看逐对象那一格的阻断，于是这件包裹即使关务本该阻断，也永不形成「出发门禁阻断」信号、不进分诊，追踪上显示成一次普通的 `DEPARTED`（ADR-0205 越权风险点 17）。
- 段已在册、对象后来才凭交接或揽收进段（控制事实的业务时间早于出发）时同理：登记那一刻段里没有它，读数里也没有它那一格。

## 三类

- 机制：对停在本侧（段不在册、那一刻段里没有在控对象）的出发，在段与参与齐了之后按原出发时点重问，追加一版读数；追加的读数里出现阻断就形成信号。与读不动的补问是同一套机制（ADR-0205 越权风险点 2），要先把确定性缺陷与暂时读不动分开，否则会对确定性缺陷无限重问。
- 产品策略：补问的唤醒（交接进段那一刻、定时扫、人工触发）与次数上限。
- 租户取值：无。

## 要裁的

- 读数是登记那一刻的记录、只插不改（迁移 0026）：补问是追加一版读数（新形状），还是另立「补问记录」挂在事实上。
- 补问来的阻断形成信号时，发作期的业务时间取出发时点还是补问时点。

## 既有票

- [53](./53-movement-facts-recorded-before-their-segment-never-reach-tracking.md)、[114](./114-deterministic-failures-answered-as-retryable-undecided-across-consumers.md)（确定性缺陷被答成可重试未决）

## 代决复核追加（2026-10-20）

出处：[owner-review-20261020](../../owner-review-20261020/report.md) 的 M5。

- **撤掉「Blocked by 用户决定」**：票面自己把补问归为机制、把唤醒与次数归为产品策略，租户取值为无，按 AGENTS 的三类都由开发方现在就定。
- **两问按机制自决**：补问追加一版读数，不改首版；补问得到的阻断信号以出发时点为业务时间。
- **前置条件**：先把段册、班次册的「装不回」与「连不上」分开，这一步并进票 114 或在本票内做。
- 分级：上线后尽快（关务读不动期间本该阻断的出发不会补问）。

## 完成记录（2026-10-26，Claude Code 作者子代理，协调方 `tops-20`，分支 `claude-dcg121`）

起点 main `a01755e11`（认领那一笔）；收尾前重放到当时的 origin/main `803b97e5f`（票 126、code/71 等已进 main），重放时 UC-TF-005 验收表与 AT 盘点两处与票 126 的新行冲突、两边都留，票 126 新加的段行「关段人」那一读接上段坏行哨兵（`e3bba49c7`）。单一权威：票面「代决复核追加」与复核报告 M5，两者不相抵。

### 提交

| SHA | 内容 |
|---|---|
| `fdae7ccba` | 代码：前置（段册、班次册装不回与连不上分开，两个停点）＋补问机制（TF 补问册与信封、补问编排、两扇补问门、读数路径按对象收窄）＋VE 补问信号一路＋派发装配与路由；迁移 TF 0052 |
| `2a95eaa17` | 文档：ADR-0305（新）；ADR-0205、ADR-0224 Status 注记与 Links；TF CONTEXT 两句；UC-TF-005 `AT-TF-342`–`346`；UC-VE-004 `AT-VE-205`–`207`；UC-VE-002 `AT-VE-182` 半句；AT 盘点八行与追记 |
| `e3bba49c7` | 重放后接上票 126 新加的段行「关段人」那一读（装不回也包住段坏行哨兵） |
| 本笔 | 本票完成记录、票 114 追记 |

### 改了什么

- **TF 领域**（`domain/departure_gate_reading.go`）：停点加 `DepartureSegmentRecordUnreadable`（`SEGMENT_RECORD_UNREADABLE`）、`DepartureScheduleRecordUnreadable`（`SCHEDULE_RECORD_UNREADABLE`），属读不动；`DepartureGateStop.Reaskable` 判哪几种停点补问时值得再问。
- **TF 端口**：`ports.ErrFulfillmentSegmentRecordUnreadable`（`fulfillment_segment.go`）、`ports.ErrTransportScheduleRecordUnreadable`（`transport_schedule_reader.go`）；新文件 `ports/departure_gate_reask.go`（补问键、记录、补问册与交出口、坏行哨兵）。
- **TF 存储**：`FulfillmentSegments.FindByKey` 构造门不过的几处包住段坏行哨兵；`scanTransportSchedule` 扫描之后的构造门不过包住班次坏行哨兵；`departureGateStopFrom` 认新两词；新文件 `departure_gate_reask.go`（补问册，只插不改、撞键答已登记、按出发列出排序契约）、`departure_gate_reask_handoff.go`（信封 `transport-fulfillment.departure-gate-reask.recorded`，载荷键五维，分区同那条出发）；迁移 `migrations/transport_fulfillment/0052_departure_gate_reask.sql`（新表、外键挂移动事实、CHECK 同 0026 形状纪律）。
- **TF 编排**：`read_departure_gate.go` 停点按坏行哨兵分开；`readIntent` 加按对象收窄（`DepartureGateReader.Reask`，出发登记与问询照旧不收窄）；新文件 `reask_departure_gates.go`（`ReaskDepartureGatesHandler`：`PlanOnHandover` / `PlanOnPickup` 在写事务之外扫段、判该不该补、问关务，`Record` 在写事务里追加并交信封；`MaxDepartureGateReasks = 3`；何时补问的判法 `departureGateReaskDue`）。
- **TF 消费门**（新文件 `adapters/inbox/departure_gate_reask_consumers.go`）：交接与对象级揽收各一扇，门每次现建——先在消费门那一笔认领，再用进事务之前的 ctx 跑 Plan（问关务），回到那一笔里 Record；哨兵 `ErrDepartureGateReaskUndecided` / `ErrDepartureGateReaskDefect`。
- **VE**：`derive_on_movement_fact.go` 的信号那一段抽成 `raiseDepartureGateSignals`，首版那一路照旧调它（行为不变）；新文件 `raise_on_departure_gate_reask.go`（`RaiseOnDepartureGateReaskAdapter`，按键回读补问与出发，为补问读数里新判阻断的那件形成信号，更早一份读数已判阻断的不再形成）、`adapters/inbox/departure_gate_reask_consumer.go`（消费门，五维译码）。
- **cmd/parcel-dispatch**（装配）：新文件 `assemble_departure_gate_reask.go`（关务准入判断与 `cmd/parcel-api` 同一批只读册与判断册另装一份、包自带事务的壳；补问编排接真段册、班次册、交接册、揽收册、移动事实册、补问册、节点作业成员关系与关务；`departureGateReasks` 接口留给真库用例换关务）、`routes_departure_gate_reask.go`（交接与揽收两条信封各加一路 `positionProjection + 20`，补问信封一路 `positionProjection`，各自的未决名单）。
- **internal/architecture**：`partition_subject_registry_test.go` 登记 `departure_gate_reask_handoff.go` 的分区主体（租户/移动事实）。
- **文档**：ADR-0305（新）；ADR-0205、ADR-0224 只动 Status 行注记与 Links，正文不动；ADR 索引、AT 盘点总览计数、机制清点没重生成（推送方的事）。
- **TF CONTEXT**（「实际移动与出发门禁读数」，逐句）：
  - 原「本上下文这一侧说不出问谁、自哪到哪时（段未声明、段不在册、出发那一刻段里没有在控对象、班次不在册）不问关务，读数答未决并写明缺什么；段册或班次册读不动答读不动。关务读不动同样答读不动、出发照记。」→ 在「段册或班次册读不动答读不动」之后加「，读回的那一行装不回（库面被绕过构造门写过）与连不上各记一个停点、同属读不动」。
  - 新加一条（紧跟「门禁读数是判断不是登记方报的内容……」那条之后）：「出发落册那一刻读数里还没有某件对象的定论时，由这件对象此后的控制事实补问：……同一条出发替同一件对象至多补问三次。……补问读数里的阻断由 `visibility-exception` 形成出发门禁阻断信号，因果条件与事实依据是那条出发（ADR-0305）。」
  - VE CONTEXT、CC CONTEXT、GLOSSARY、CONTEXT-MAP 都没动。
- **UC**：UC-TF-005 状态行加「出发门禁补问」、规则节加一条指向 TF CONTEXT、「一致性、幂等与并发」那一条后补「补问只追加、不改首版……」一句、验收表新立 `AT-TF-342`–`346`；UC-VE-004 验收表新立 `AT-VE-205`–`207`、段末验收说明加一句；UC-VE-002 `AT-VE-182`「补问归补问机制」改指 ADR-0305 与 `AT-TF-343`。
- **票 114**：追记一段，写明段册、班次册两处由本票收了、其余各处未动。

没碰票 126 名单里的口（关段、建派送任务、装载分配、终止参与、两个判断口、运输委托提交）与票 176 的 NR 连接登记口；碰了票 126 也改过的 `fulfillment_segment_registry.go`（`FindByKey` 里构造门不过那几处的错误包装）与 UC-TF-005 验收表、AT 盘点（追加行），重放时两边都留。没碰 tops-26 在途的 code/12+46（SA）、code/71（已进 main）。VE 迁移没用；TF 迁移取 0052。

### 代决与越权风险点

代决（逐条：决定了什么、读了什么没读什么、owner 若不同意改哪里）：

1. **补问追加一版读数、首版不改**（M5 原文）：落成 TF 新册一次一行（迁移 0052），不挂成移动事实的新版本。读了 ADR-0205 全文、迁移 0026、移动事实册与重放回显代码；owner 若要并列在移动事实查阅面，改的是查阅读口（越权风险点 7），册不动。
2. **补问的业务时间是出发那一刻**（M5 原文）：问关务的时点取事实的业务发生时间；信号的因果条件与事实依据取那条出发（`movement-fact/<对象>/<事实>` 引用与那一版事实版本），判断标识取补问那一格，命中时刻照旧取判断那一刻（与票 135 定下的形状一致，没给信号另加业务时间一维、没动 VE 迁移）。读了 `RaiseDepartureGateSignalHandler`、`SignalFactBasis`、VE CONTEXT 信号各条、UC-CC-013 动作时点一格；owner 若要信号另存一维「业务时间」，要动 VE 发作期册（VE 迁移）。
3. **补问只问唤醒它的那一件**（机制）：整条出发重问在一车几百件、各件各自唤醒时成平方次关务询问；班次与段的核对随之只核这一件进段那次交接（越权风险点 3）。owner 若要整条重问，改 `ReaskDepartureGatesHandler.reaskOne` 交给 `Reask` 的范围与 `departureGateReaskDue` 的判据。
4. **何时补问的判据**（机制）：看这件对象最近一份读数（最近一次补问，没有则首版）；可再问的停点与读不动列在 `DepartureGateStop.Reaskable` 与 `deterministicGateCauses`。关务答的未决算定论、不补问（关务按动作时点判，事后补登的配置是否回溯由关务定，不由本侧猜）；存量回填不补问。owner 若要对关务未决也补问，在 `departureGateReaskDue` 里放开那一格。
5. **唤醒复用控制事实信封**（产品策略，M5 授权自决）：TF 消费自己的 `transport-handover.registered`（只认转出控制的那一版）与 `offsite-pickup.registered`，按对象扫每一段（ADR-0302 同一扫法）；不新增常驻进程、不定时扫、不开人工补问口。读了 `cmd/parcel-dispatch` 路由登记、FanOut、消费门、结算续办门先例；没读运维对派发进程的部署形态要求。owner 若要定时扫或人工口，另立票（越权风险点 1、2）。
6. **次数上限三次**（产品策略，不是租户取值）：`MaxDepartureGateReasks = 3`，理由见 ADR-0305 决定七。owner 若要别的数，改这一个常量；若要做成租户取值，要另立登记册并在未登记时答未配置。
7. **问关务在消费门事务之外、追加在消费门那一笔里**（机制）：先认领再问，已处理的不白问；派发里另装一份自带事务的关务准入判断壳。owner 若要把补问挪成「先追加后异步问」的两段，要另起一类待问册。
8. **补问来的阻断由 VE 另一路形成信号**（机制）：TF 交一类新信封、VE 新消费一路，不让补派生顺手做（FanOut 不保先后、补派生在事务里问不了关务）；更早一份读数已判阻断的那件不再形成。
9. **前置只做段册、班次册两处**（派单）：交接册那一读与 VE 各消费方的同族缺口留给票 114，票 114 追记写明。
10. **ADR-0205 决定二停点名单算部分停用**：多两个停点照 ADR-0237 的先例记为部分停用（被停原句「读不动：段册或班次册读不动。」），其余答越权风险点、无条款停用。

越权风险点（详见 ADR-0305 文末）：

1. 补问要等这件对象的控制事实，末段（此后只剩有效交付）照丢（TF、VE owner）。
2. 次数上限三次是代决的产品策略，关务持续读不动跨过三程时阻断永不形成信号；到上限不告警（TF owner）。
3. 补问只问唤醒它的那一件，班次与段只核这一件进段那次交接（TF、CC owner）。
4. 补问按此刻的册子读：事后登记、生效回溯的限制补问读得到、首版读不到（TF、CC owner）。
5. 交接册那一行装不回仍与连不上同记，补问会为它再问（至多三次；票 114）。
6. 存量回填不补问；上线前停在本侧的出发只由此后的控制事实补（TF、VE owner）。
7. 运营查阅看不到补问读数（TF owner）。
8. 多读与坏行波及（运维、TF owner；票 89、114）。
9. 补问那一拍在消费门事务开着时问关务，慢时整封投递可能超时重投（运维）。
10. 「更早一份读数已判阻断」按补问册次序判；首版因规则未配置没形成的，补问也不再形成（VE owner）。
11. 读不动的出发仍不另立一类信号（VE owner，ADR-0205 补记越权风险点 17 后一半）。

### 修法 → 用例

| 修法（M5 / 票面） | 用例或落点 |
|---|---|
| 撤掉用户决定阻断 | 票面 Blocked by 已是「无」（认领前已改） |
| 前置：段册、班次册装不回与连不上分开 | `AT-TF-342`：`domain/departure_gate_reading_test.go::TestOnlyStopsThatCanChangeAreReaskable`、`application/read_departure_gate_test.go::TestADepartureStopsBeforeCustomsWhenItCannotSayWhatOrWhere`（两个新子用例）、`adapters/postgres/registry_record_unreadable_test.go`（真库，两本册） |
| 补问追加一版读数，不改首版 | `AT-TF-343`：`adapters/postgres/departure_gate_reask_test.go::TestADepartureGateReaskAppendsAndRoundTrips`（首版不动、撞键不覆盖、外键）、`application/reask_departure_gates_test.go::TestADepartureRecordedBeforeItsSegmentIsReaskedForTheObjectThatEntered`、派发真库 `TestABlockedDepartureRecordedBeforeItsSegmentIsReaskedAndSignalledThroughTheProductionRoute` |
| 对停在本侧（段不在册、那一刻段里没有在控对象）的出发在段与参与齐了之后按原出发时点重问；读不动的补问同一套机制 | `AT-TF-343`、`AT-TF-344`：`TestAReaskOnlyComesWhileTheReadingHasNoVerdictForTheObject`、`TestAUnitIsReaskedOnlyWhileAMemberCouldStillAnswer`、`TestTheReaskReadsTheOneObjectThroughTheDepartureReadingPath`；消费门 `TestTheReaskGateAsksCustomsOutsideTheInboxTransactionAndWritesInsideIt` |
| 唤醒与次数上限（产品策略） | `AT-TF-345`：`TestReaskingFollowsTheLatestReaskAndStopsAtTheCap`、`TestReaskSkipsWhatIsNotAnOriginalDepartureUnderControl`、`TestRecordKeepsTheFirstComerAndHandsOffOnlyWhatItAppended`、`TestAPickupWakesTheReaskToo` |
| 追加读数里出现阻断就形成出发门禁阻断信号，以出发时点为业务时间（票 135 的形状） | `AT-VE-205`、`206`：`raise_on_departure_gate_reask_test.go` 三个用例；派发真库用例核发作期的因果条件、依据（引用、v1、补问那一格的判断标识）、一期一命中、进人工复核 |
| 失败分格 | `AT-TF-346`、`AT-VE-207`：编排、真库坏行、消费门、派发分区各用例 |

提交前变异抽验：把补问那一拍的 Plan 改用消费门事务里的 ctx，消费门用例与派发真库用例都转红（后者补问读数落成读不动——关务那层壳拒了事务里的询问）。

### 自验（含 PG，`IDP_PARCEL_POSTGRES_DSN` 指 55432 一次性库）

重放到 `803b97e5f` 之后、`e3bba49c7` 上：

- `go build ./...`、`go vet ./...` 通过；新写与改过的 `.go` 都过 `gofmt -l`（无输出）。
- `go test -count=1`：`./internal/transportfulfillment/...`、`./internal/visibilityexception/...`、`./internal/customscompliance/...`、`./internal/architecture/...`、`./migrations/...`、`./internal/platform/migrate/...` 全绿；依赖 TF/VE/CC 的 `cmd` 包（`go list` 反查）`./cmd/parcel-api`、`./cmd/parcel-customs-register`、`./cmd/parcel-dispatch`、`./cmd/parcel-transport-register`、`./cmd/parcel-ve-register` 全绿。新真库用例以 `-v` 确认 PASS、无 SKIP。
- `go run ./tools/adr-index -check`、`go run ./tools/at-coverage-counts -check` 只报生成物过期（退 1，预期；缺行会退 2，没有）。
- 全量 `scripts/test-full.sh` 未跑（推送方的事）。

