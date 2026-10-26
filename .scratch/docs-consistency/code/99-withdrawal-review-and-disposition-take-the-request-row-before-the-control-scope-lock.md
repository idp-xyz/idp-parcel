# 99 撤回、主动拒绝、授权处置先锁委托行再取控制作用域锁，与接受链次序相反：同一委托并发可死锁

Category: bug
Status: resolved——2026-10-26 进 main（见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-code99（见文末「完成记录」）。此前 needs-triage——建议 R2（上线后尽快）：同一份已提交委托上，一笔续办的接受链与一笔客户撤回（或主动拒绝、授权处置）并发时两笔互等，PostgreSQL 的死锁检测中止其中一笔、整笔回滚。不损坏数据：消费门会重投那封信封，撤回一侧答未形成、客户重发即可。窗口窄，但已装配在生产上。2026-10-26 协调方据票 [code/77](./77-cancellation-intake-adoption-and-label-final-are-not-serialized-per-parcel.md) 隔离评审（信息 3）立；code/70 越权风险点、code/71 越权风险点里各提过一半。
Blocked by: 无
出处：code/77 隔离评审信息 3（Sonnet 评审，读码取证，未在真库复现）。

## 现象

两条路径对同一份委托取同两把锁，次序相反：

- 撤回、主动拒绝、授权处置：先保存委托（对 `parcel_shipment.shipment_request` 那一行 UPDATE，持行锁 R），再释放受理前控制——`ReleasePreAcceptanceControlHandler.Handle` 无条件先取控制作用域咨询锁 S（`LockControlScope`）。
- 接受链：先走财务控制（`freezeFunds` 先取 S），再在形成受理决定时保存委托（R）。

续办的接受链事务持 S 等 R，客户撤回事务持 R 等 S，两笔互等。S 是（租户，作用域）级，必须是同一份委托才成环。

## 证据

- `internal/parcelshipment/application/withdraw_shipment_request.go`：`Requests.Save` 在 `releaseOccupation` 之前。主动拒绝、授权处置同形（`cmd/parcel-api` 的 `assemble_review.go`、`assemble_disposition.go` 同样装了作用域锁）。
- `internal/parcelshipment/application/advance_acceptance_chain.go`：财务控制（取 S）在 `form_acceptance_decision.go` 的 `Requests.Save` 之前。
- 复现条件：委托状态为已提交，财务控制已施加过、存有作用域认领（例如接受链曾停在人工复核或可达性重校后续办），续办的接受链与客户撤回同时到达。

## 三类

机制。无产品策略、无租户取值。

## 规模

S–M：调整一侧的取锁次序，补真库并发用例。

## 最小的诚实修法

二选一，落地时定：

1. 撤回、主动拒绝、授权处置在保存委托之前先取 S（与接受链同序：S → R）。
2. 接受链在取 S 之前先锁住委托行（R → S），撤回一侧不动。

补一条真库并发用例：同一份委托，一笔接受链续办停在取到 S 之后，另一笔撤回要么等在 S 上、要么已提交，断言不出现死锁中止。

## 要裁的

- 选哪一侧改次序。倾向第 1 条：撤回类是低频的人工操作，提前取 S 的代价小；接受链是信封驱动的主路径，不宜再加一把行锁。

## 完成记录

分支 `claude-code99`，基于 main `821fb421c`（收尾 rebase 之后以回报里的最终 SHA 为准）。

### 做了什么

- 选第 1 条（协调方采纳）：撤回、主动拒绝、授权处置，以及接受链形成决定的自动拒绝、停进`等待受控补充`，保存委托（R）之前先取这份委托各版已成立占用所在作用域的锁（S）。接受链一侧不动。
- 新端口 `ports.PreAcceptanceControlScopeLock`（PS 自有，坐标是控制结果上存下的 `OccupationScope`），头注写全序；消费侧适配器 `settlementaccounting.ControlScopeLocks` 把坐标译给 SA 的同一把锁（`sapostgres.PreAcceptanceControlScopeLocks`），不另建锁。SA 一侧的端口头注加一段指回 PS 头注。
- 编排：`lockScopesBeforeSave`（`application/lock_occupation_scopes.go`）读每一版已记录的控制结果，成立了占用且存下作用域的才取，同作用域一次；`WithdrawShipmentRequestHandler.decide` 在已决态短路之后、领取决定标识之前取，`RejectShipmentRequestHandler`、`DisposeShipmentRequestHandler` 在各自的前置判过之后、领标识之前取，`FormAcceptanceDecisionHandler` 在保存拒绝与保存`等待受控补充`暂停之前取。四只 Deps 各加 `ScopeLock`。
- 装配：`cmd/parcel-api` 的 `assemble_withdrawal.go`、`assemble_review.go`、`assemble_disposition.go` 与 `cmd/parcel-dispatch` 的 `acceptanceChainConsumers` 各接上；首方撤回口（`buildCustomerWithdrawalOrchestration`）与代理人形态共用的 `assembleWithdrawalOrchestration(db, authorizer)` 构造点接了 `ScopeLock`，所以首方撤回口随之带上这一把锁，不需要另接；这条授权器入参接缝同 `rejectionOrchestrationWith`、`dispositionOrchestrationWith`，本票的装配用例直接用它。
- 文档：UC-PS-005「规则、补偿与并发」补一条取锁次序；验收示例新立 `AT-PS-127`、`AT-PS-128`、`AT-PS-129`；UC-PS-001 交接表补一行承接 `AT-PS-035`；盘点三行入①与一段追记（总览计数未动）。

### 锁序全序（全文在 `ports.PreAcceptanceControlScopeLock` 头注）

同一事务里的取锁次序：消费门认领 inbox 行（仅 dispatch）→ S（若干把，按版本次序）→ R（保存委托）→ 其后的行写入与 outbox 插入。

- 接受链：链开头放被取代各版（取各自作用域的 S）→ 施加当前版本的控制（取其 S）→ 形成决定保存委托（R）→ 拒绝或停进`等待受控补充`之后的释放。R 之后取的 S 都是此前已持有的重入。
- 撤回、主动拒绝、授权处置：各版 S（本票）→ R → 释放里的 S（重入）。自动拒绝与暂停的保存之前同样先取。
- 已越过提交边界的重放：不保存委托，只在释放里取 S，不持 R，不倒挂。
- P（取消边界锁，code/77/98）：取消、收寄采用、终局三处不更新委托行、不取 S，持 S 的事务不取 P，P 与 S、R 之间没有边。来源身份锁所在的提交路径不取 S，建单是 INSERT。
- 因此图上不存在「持 R 再去取一把还没持有的 S」的边：同一份委托上链与撤回类路径之间无环。S 与 S：同一份委托各版在不同作用域形成占用时，撤回类按版本形成顺序（被取代在前、当前在后）取，与链开头放被取代各版再施加当前版本一致。
- 没有再加行锁：接受链不动（协调方裁定）。

### 代决

1. 取哪几把、什么次序：读每一版的已记录控制结果，`OccupationFormed` 且作用域已存下的才取；次序取版本形成顺序而不是键序——链的次序是被取代各版再当前版本，同一份委托上两边一致才不成环，键序与链不一致时反倒会成环。同作用域只取一次。
2. 读不回某一版的判断：不取那一版、也不停下。保存之后的释放读同一份判断、同样读不回，会把那一版留成补偿续办；撤回不等财务释放才生效（UC-PS-005）。
3. 取不到锁与保存委托失败同格（协调方定）：撤回、主动拒绝答未决 `DECISION_NOT_RECORDED`，授权处置如其保存失败上抛，自动拒绝与暂停答未决 `DECISION_NOT_RECORDED`；都不保存、不释放。不新增原因码。「不领决定标识」只对撤回、主动拒绝、授权处置成立（它们的取锁在领标识之前）；Form 的自动拒绝与暂停是先领决定标识、再 `Decide`、再取锁，取锁失败时标识已耗掉。`DECISION_NOT_RECORDED` 也只对非 SQL 级的取锁失败（装配缺口、替身）是真的答复：取锁失败若是 SQL 级错误（死锁受害者），事务已中止，handler 返回的 nil 错误在提交时变成 `commit unexpectedly resulted in rollback`，实际是 HTTP 5xx，来源保全随事务回滚；这仍是诚实的「没形成，客户重发」（见越权风险点 4）。
4. 取锁口没接上而这份委托有占用要排队：按取锁失败处置（哨兵 `ErrControlScopeLockNotAssembled`），不静默放过。静默放过的装配缺口就是死锁本身，且只在真并发下才露头。身份或作用域译不成提供方的值的不取（提供方的释放也在读账本之前就以未受理短路，那一笔该记进释放未受理册，不该变成撤回类命令的失败）。
5. 自动拒绝与暂停也在保存前取：接受链里这些锁施加与链开头放被取代各版已取过，是重入；加上是为了不依赖上游，单驱形成决定这一步（测试夹具、日后别的调用点）先 S 后 R 照样成立。接受与停进别的等待态不释放，不取。
6. **不立 ADR-0290**：核过 ADR-0132 决定一（释放时刻在处置那一刻）、ADR-0282 决定二至四（确定未受理记册、业务事实不回滚、作用域取存下的那份）与 ADR-0122 决定三（释放幂等归提供方账本），没有一条说过「先保存委托、后取作用域锁」或释放内部的取锁次序；释放仍在保存之后、答复三格与登记册不变。docs/adr 里没有任何一条提到这把锁。UC-PS-005「撤回提交成功后不等待财务释放才生效」不被触动：撤回此前也要等 S 才提交（释放在同一事务里等 S，提交在其后），等待只是从保存之后挪到保存之前。有一处口径收窄见越权风险点 1，未到停用 ADR 的程度。
7. 接受链一侧用一笔「占着 S、停住、再保存委托行」的事务代替（`cmd/parcel-api` 三路径用例）；真接受链（真 SA 施加与释放、真账本）的同一场景只做撤回一路（`cmd/parcel-dispatch`）：主动拒绝与授权处置的真链夹具用替身控制，造不出带真 S 的占用，不为凑数另造。
8. 立 code/102（反向时序孤儿占用，本票范围之外；评审后定级 R1，首方撤回口上线前必修，见越权风险点 3）。

### 测试

- 修前红：三路径真库用例全红（等在 S 上时已持着委托行锁；撤回答 `FORMED` 而非输给接受链的未决）；真接受链用例红（撤回提交时 `commit unexpectedly resulted in rollback`）；服务端日志有 `deadlock detected`（撤回等咨询锁 S 被链阻塞，链等撤回持的委托行事务），把取锁挪回保存之后的变异还原出同一幕。
- 新用例：`cmd/parcel-api/control_scope_lock_order_test.go`（三路径，生产装配，池外 `pg_locks` 带期限并限本库认等待者，委托行 `FOR UPDATE NOWAIT` 探针）、`cmd/parcel-dispatch/control_scope_lock_order_test.go`（真链）、`application/lock_occupation_scopes_test.go`、`adapters/settlementaccounting/control_scope_lock_test.go`（含真库证与提供方同一把锁、按租户与作用域分键）。
- 评审修订轮新增（`application/lock_occupation_scopes_test.go`）：`TestAFirstArrivalWithdrawalOverADecidedRequestTakesNoScopeLock`、`TestAnActiveRejectionOverADecidedRequestTakesNoScopeLock`、`TestAnAuthorizedDispositionOverADecidedRequestTakesNoScopeLock`（新来源身份首次到达已接受、已撤回的委托，取锁口零调用、不保存、不领决定标识；这条路径授权先于已决态判断，所以不断言「不问授权」）与 `TestTheScopeLocksOfSeveralSupersededVersionsAreTakenInFormationOrderWithTheCurrentVersionLast`（两个被取代版本）。变异（备份、还原后 `cmp` 一致）全部被杀：撤回把取锁挪到已决态短路之前、主动拒绝挪到 `ActiveRejectionAdmissible` 之前、授权处置挪到 `AuthorizedDispositionAdmissible` 之前（各只红对应新用例，此前全绿）；`versionsInLockOrder` 里被取代各版内部反序（只红新用例）。
- 变异（备份、还原后 `cmp` 一致）全部被杀：撤回仍先 Save 后取 S（应用单测、parcel-api、dispatch 三层红）；只改撤回不改主动拒绝、只改撤回不改授权处置（parcel-api 对应格与单测红）；取 S 放在领决定标识之后（取锁失败格「不领标识」红）；版本次序反过来、去重去掉、「存下作用域」判断去掉、自动拒绝取锁去掉、暂停取锁去掉、适配器作用域译错一段、parcel-api 三处装配各自漏接（各格红）。

### 越权风险点（待 PS owner 复核）

1. **取不到 S 现在挡住业务事实形成。** 此前释放里取 S 失败被折成「释放未形成」，撤回仍成立、留补偿续办；现在 S 在保存之前，取不到（死锁受害者、语句取消、连接坏）撤回就没形成。取舍理由：S 在保存之前是锁序成立的前提；保存之后再「取不到就算了」要么回到倒挂，要么另立一条会倒挂的退路；取不到锁与保存不了同属一个写依赖。UC-PS-005「也不得把释放失败改写为撤回失败」说的是释放的答复，不是排队取锁，但这一处是语义上的收窄，请复核。
2. **S 与 S 之间没有先验全序。** 当前版本的作用域要到施加时才形成，取不出键序；撤回类与链在同一份委托上同序，但不同委托的各版若在同一对作用域上次序相反，S 与 S 仍可成环。它要各版作用域不同（政策回显或账户登记在版本之间变过）才出现，今天没有观察到，未处理。
3. **反向时序的孤儿占用（范围外）。** 撤回类路径先拿到 S、链此前已读到`已提交`并在 S 上排队时，链排到后照常施加并留下一笔挂在已撤回委托上的冻结，没有路径放它。真库实测复现；改前改后都存在（时间窗是链开头读委托到施加之间）。评审另补出窗口（b）：链已写下冻结（未提交）、Form 还没读委托时撤回提交，首版委托没有已提交的占用，撤回不取 S、释放什么也不放，本票也没关上它。[code/102](./102-an-acceptance-chain-round-that-waited-on-the-control-scope-lock-past-a-terminal-decision-leaves-a-fresh-occupation-nothing-releases.md) 立了票，评审后定级 R1，在 Status 写了依据。**首方撤回口上线前必修。**
4. **取锁失败是 SQL 级错误时事务进入中止态。** 释放有 `SavepointedControlRelease` 保护，本票的取锁没有：这时撤回的来源保全随事务回滚、HTTP 5xx，客户重发；handler 交回的未决 `DECISION_NOT_RECORDED` 因此只有非 SQL 级的失败（装配缺口、替身）才真到达客户。取锁失败本就多半伴随保存失败，没为它另包 savepoint。
5. **`cmd/parcel-dispatch` 的 `ScopeLock` 装配没有行为测试，没有用例拦。** 评审变异：删 dispatch 的 `scopeLocks` 与 `ScopeLock:` 全绿。后果是自动拒绝和暂停遇带占用的委托恒答 `DECISION_NOT_RECORDED`，响亮失败而不静默。链里这把锁是重入，漏接不改任何别的结果，要到自动拒绝或暂停遇到有占用的委托那一刻才露头；生产的商业依据是真 PC 权威，走不到`要求拒绝`，造不出生产装配上的真用例。parcel-api 三处接线有变异证明。
6. **取锁口缺位在有占用的委托上是失败（fail-closed）。** 日后新装配（若不复用 `assembleWithdrawalOrchestration` 而自己构造 `WithdrawShipmentRequestDeps`）必须传 `ScopeLock`，否则带占用的撤回答 `DECISION_NOT_RECORDED`。今天仓里撤回编排唯一的生产构造点在 `assemble_withdrawal.go` 的 `assembleWithdrawalOrchestration`，代理人形态与首方撤回口都经它。
7. 每条路径多读一轮各版已记录的判断（`LoadRecordedJudgments`，N 版 N 次读，在保存之前）。撤回类是低频人工操作，没有为它另开只读控制结果作用域的读口。

### 评审记录与修订轮

- 独立评审结论：可合入；合入前修 N1 至 N5，另有对 code/102 的评估（N6）。
- N1：评审变异把撤回、主动拒绝、授权处置的取锁挪到已决态判断之前，application 与 parcel-api 全绿（存活），因为现有重放用例在进 `decide` 之前就短路。补三条首次到达已决委托不取锁的用例，变异现在被杀。
- N2：被取代各版内部反序的变异存活，因为现有用例最多一个被取代版本。补两个被取代版本的用例，变异现在被杀。
- N3：「不领决定标识」只对撤回、主动拒绝、授权处置成立，Form 的自动拒绝与暂停先领标识；SQL 级取锁失败实际是回滚与 5xx，只有非 SQL 级失败才答 `DECISION_NOT_RECORDED`。`AT-PS-129`、UC-PS-005、代决 3、越权风险点 4 已改准。
- N4：`ErrControlScopeLockNotAssembled` 的注释改写实（哨兵只在授权处置露出）；dispatch 装配无用例拦，写入越权风险点 5。
- N5：位置词改成符号名或引文。
- N6：code/102 按评审改为 R1，补窗口（b），推荐修法为方案 1；依赖本票进 main，单独做，不并入本票。

## 进 main 记录（2026-10-26，协调方）

- 分支 `claude-code99` 由作者 rebase 到 origin/main `29ef8aa1e` 后快进，tip `a7658141f`。无迁移、无 ADR（核过 ADR-0132 决定一、ADR-0282 决定二至四、ADR-0122 决定三，都没有规定「先保存后取锁」或释放内部的取锁次序）。
- 协调方分诊：采「撤回、主动拒绝、授权处置在保存委托之前先取控制作用域锁 S（与接受链同序 S → R）」；与首方撤回口同批，R2 升为上线前必修。新增 PS 端口 `ports.PreAcceptanceControlScopeLock`，坐标取控制结果上存下的 `OccupationScope`，适配器译给 SA 的同一把锁；自动拒绝与暂停在 Save 前也取（链里是重入）。
- 隔离评审（Sonnet）可合入，无阻断。评审核实锁序全序成立（没找到持 R 再取一把还没持有的 S 的路径），语义收窄（取不到 S 现在挡住业务事实形成）可接受——等锁与原来一致、取锁失败不是「释放失败」、生产上能出错的情形很窄；N1 补三条「已决态首次到达不取锁」用例（此前变异存活）、N2 钉死被取代各版按形成顺序、N3 文档校正（不领决定标识只对撤回、主动拒绝、授权处置成立；SQL 级取锁失败实际是回滚加 5xx）、N4–N5 注释与位置词，由 Sonnet 子代理收掉。
- 评审另评估作者实测发现的 code/102（反向时序孤儿占用）：比票面写的更宽（窗口 b：链已写下冻结未提交、Form 未读委托时撤回提交），定级 R1，首方撤回口上线前必修；原「取到 S 之后重读」的方案 2 不够，推荐方案 1（Form 的 `existing` 路径上，委托已撤回/已拒绝时先取锁再逐版释放，放不收口则整轮回滚）。code/102 票面已按此改写，紧接本票开工。
- 协调方单独提交重算的 AT 计数与重生成的机制清点；ADR 索引无变化；检查均退 0。
- 全量 `scripts/test-full.sh` 退 0、158 个包 ok；`go build ./...`、`go vet ./...` 干净。无迁移，种子未灌。
- 去向：code/102（R1）。S 与 S 之间没有先验全序（不同委托、各版作用域相反时仍可成环，后果是死锁检测中止一笔，不损数据）；`cmd/parcel-dispatch` 的 `ScopeLock` 装配没有行为测试，漏接时自动拒绝与暂停遇带占用的委托恒答 `DECISION_NOT_RECORDED`（响亮失败）——均记在越权风险点。
