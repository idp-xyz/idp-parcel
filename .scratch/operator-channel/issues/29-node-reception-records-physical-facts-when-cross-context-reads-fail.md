# 29 收寄遇依赖读不动时答了不落册的`收寄待确认`：改为服务结果读不动照常形成收寄并记「读不动」，身份核对读不动或本方册读写失败时不形成答案

Category: bug
Status: in-progress——2026-10-18 完工；评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-18 作者子代理完工，待评审。此前 in-progress——2026-10-18 会话 `idp-parcel-9b` 立并认领（票 26 序 2 评审遗留；用户授权代决）；分支 `claude-oc29`
Blocked by: 无
父票：[26](./26-non-author-review-backlog-for-claude-code-landings.md)，评审欠账序 2。被取代的那句裁定在 [25](./25-node-reception-carries-parcel-shipment-service-result-markers.md)。
出处：
- 票 25 Comments「评审 ← 会话 idp-parcel-9b 起的隔离子代理 · 钉 0214a62a」：Spec 非阻断 (1)、(2)，Standards (1)–(3)；
- ps-external-mark-relations/02 同名评审：Standards (3)；
- 簇 C1 的研判，以及三份对抗核验（权威一致性、实现与回归、业务后果）。
地盘：
- internal/nodeoperations/application/receive_delivered_unit.go
- internal/nodeoperations/application/receive_delivered_unit_test.go
- internal/nodeoperations/ports/ports.go
- internal/nodeoperations/adapters/postgres/reception.go
- internal/nodeoperations/adapters/postgres/reception_test.go
- internal/nodeoperations/adapters/postgres/（新文件：服务结果读口 savepoint 装饰器及其真库测试）
- internal/nodeoperations/adapters/http/receive_delivered_unit.go
- internal/nodeoperations/adapters/http/receive_delivered_unit_test.go
- internal/nodeoperations/adapters/parcelshipment/identity.go
- internal/nodeoperations/adapters/parcelshipment/service_results.go（或新 doc.go）
- internal/nodeoperations/adapters/identity/intake_result_version.go
- migrations/node_operations/（新迁移）
- cmd/parcel-api/assemble_reception.go
- cmd/parcel-api/assemble_reception_test.go
- cmd/parcel-api/assemble_reception_service_results_test.go
- cmd/parcel-api/assemble_customer_amendment_test.go
- cmd/parcel-frontline-import/intake_import.go
- cmd/parcel-frontline-import/vertical_test.go（或新真库用例文件）
- docs/adr/（新 NO ADR）
- docs/adr/README.md
- docs/application/node-operations/UC-NO-002-RECEIVE-CUSTOMER-DELIVERED-PARCEL.md
- docs/product/MECHANISM-INVENTORY.md（重生成）
- .scratch/operator-channel/issues/25-node-reception-carries-parcel-shipment-service-result-markers.md（Comments）
- .scratch/operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md（评审结果追加）
- .scratch/no-directional-basis/issues/01-consolidation-adds-members-without-any-directional-basis.md（Comments）
- .scratch/ps-external-mark-relations/issues/02-register-and-customer-parcel-numbers-feed-node-identity.md（Comments）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；裁定经隔离子代理研判、对抗核验与合成（研判轮 review26-followups-decide）。

新 NO ADR 用号 0186（本会话已占）。迁移号取实施时 migrations/node_operations/ 的下一个空号；与对端 no-directional-basis/01 约定谁先进 main 谁用 0005，后进的一方顺延。no-directional-basis/01 的票面是对端地盘，本票不改；口径已当面对齐。

## 现象（钉 main 8d22729f，真库实测）

- 身份或服务结果读口报 Go 层错误：ReceiveDeliveredUnitHandler.receive 答 200 `RECEPTION_UNDECIDED`，带续办引用，零行，不交意图。
  - UC-NO-002 结果表里的`收寄判断待确认`，前提是「物理事实已保存」。
  - ADR-0022 让设备在收到 2xx 时出队。于是这一次接收，服务端从此没有记录。
- 同样两支报 SQL 级错误时：PS 读与收寄在同一笔事务里，事务被打成中止态，bento 提交时得到 `pgx.ErrTxCommitRollback`，端点答 500 NO_ANSWER_FORMED，零行。前线导入记「编排未形成答案，已回滚：commit unexpectedly resulted in rollback」。
- NO 自身失败，同样答零行的 200 待确认。包括：收寄库 FindByKey 失败、摘要比对未知、Save 失败、并发读回失败，以及版本签发失败。
- 装配缺服务结果读口时，运行期答待确认。
- INCOMPLETE_DELIVERY 与未知接收观察两支，也答 200 待确认。两条渠道的入口都会先挡住它们，所以生产上走不到。
- 服务结果标记的「封闭两种」，只靠 application 的 serviceMarkersOf 里两个字面量前缀撑着。

## 裁定（落地后以新 NO ADR 为单一权威，本节不再维护）

1. 身份唯一，但服务结果读不动：照常`节点收寄已形成`，收寄、控制、关联、交意图都照常，首答 201。
   - 收寄记录新增「服务结果核对态」，只有 INTAKE_FORMED 带，取值为「已查」或「读不动」。
   - 「读不动」指读口交回错误、没取得 PS 的答复。它不是「查过了，没有取消与终局」。
   - 读不动时，标记必为空。
   - 不补查，也不发续办引用。
2. 身份核对读不动：编排交回 error，不形成答案。HTTP 答 500 NO_ANSWER_FORMED，前线导入记未决。
   - 不落册，不拿空候选顶替（ADR-0089）。
   - 目标形态「待识别 + 身份未核对」归no-reception-source/01。
3. NO 自身落不了册：同第 2 条。装配缺依赖，改为在构造期拒绝。
4. 输入缺陷两支，交回被拒类错误：HTTP 答 400 MALFORMED_REQUEST，前线导入记被拒。
5. 编排不再为没落册的尝试签发续办引用。
6. 拒收、仅扫描、零候选、多候选、身份唯一可读这五支，行为不变。outcome 词表不增不减。
7. 服务结果读口经 savepoint 隔离，身份读口不隔离。savepoint 必须与第 1–5 条同票落地，不得单独先落。
8. 标记的合成挪到 ports，作为 ParcelServiceResults 唯一的合成入口，产出的串逐字不变。
   - 记录、存储、响应里仍存串。
   - 不立解析。程序消费方一律经 ParcelServiceResultView 现问，不得解析标记串。

## 做什么

1. 编排：改 ReceiveDeliveredUnitHandler。
   - 按裁定 1–5 改 receive、Handle、commit。
   - NewReceiveDeliveredUnitHandler 改为交回 (handler, error)，任一依赖为 nil 即拒。删去运行期那一支「SERVICE_RESULT_VIEW_UNCONFIGURED」。
   - 新增被拒类哨兵。
   - 结果与 HTTP 响应上的续办引用格，若已无人产出，一并删去。receptionContinuation 只留给意图续办。
   - ReceiveDeliveredUnitDeps 头注只写语义，不写事务机制：服务结果读口报错时编排照常落册；装配须保证读口失败不连带收寄的写，见 transactionalReception。
2. ports：
   - serviceMarkersOf 挪成 ParcelServiceResults 的方法，两个前缀改为包内常量。
   - 新增服务结果核对态的封闭枚举，带 String。ReceptionRecord 加这个字段。
   - 改 ReceptionRecord 与 ParcelServiceResults 的头注：删去「词表见 application 的 serviceMarkersOf」；写明标记串只给人看与审计，不得解析。
3. 存储：新迁移给 node_operations.reception 加核对态列。
   - 封闭取值，用 CHECK 约束；只有 INTAKE_FORMED 带；取值为读不动时，service_markers 必为空数组。
   - 既有的 INTAKE_FORMED 行回填「已查」。迁移头注写明推定依据，并钉 SHA：
     - 0214a62a 早于 cc52e9b6；
     - cc52e9b6 之前，两处装配的身份核对都是未配置桩，明确接收从不落册（eac94e06 提交说明、票 08 的记录表）；
     - 收寄表唯一的生产写方，是 NO 的 postgres 适配器。
   - Receptions 读写都要往返这一列。
4. savepoint 装饰器：在 adapters/postgres 新建一个文件，包住 ports.ParcelServiceResultView。
   - 先经 RequireExecutor 发 SAVEPOINT；内层报错就 ROLLBACK TO SAVEPOINT，再把错误原样交回；成功就 RELEASE。
   - ctx 不在事务里（bentopg.ErrTransactionRequired）时直通。
   - 写法照 settlementaccounting 适配器里的 SAVEPOINT 先例。
   - 包的位置：cmd/parcel-api 的 buildParcelServiceResultView；cmd/parcel-frontline-import 里抽出一个与 parcelIdentityViewOf 同形的服务结果读口构造函数，在那里包。
   - 身份读口不包。
   - transactionalReception、buildIntakeImporter 与装饰器的头注里，写明「跨上下文读口须经 savepoint 隔离」。
5. HTTP 收寄端点：
   - INTAKE_FORMED 的首答与重放，响应都带核对态信息格。建议字段名 serviceResultCheck，取值 CHECKED 或 UNREADABLE。这一格不进 outcome 词表。
   - 被拒类答 400 MALFORMED_REQUEST。
   - 其余 error 照旧答 500 NO_ANSWER_FORMED，不带 outcome。
6. 前线导入：
   - intakeRowDisposition 的「待确认不带记录 → 未决」一支，不再有业务含义：编排不会再交回这种结果。万一出现，按实现缺陷记未决。
   - describeReception 写出服务结果标记与核对态。读不动时写「服务结果读不动：标记缺席不代表没有取消或终局」。
   - 被拒类记被拒。
7. 注释：
   - 删去三处只陈述事实、不讲理由的说法：buildReceptionOrchestration 头注「都与收寄同一笔事务读」、buildIntakeImporter 内「与收寄同一笔事务读」、真库用例文件头「在收寄那一笔事务里读得出」。改写成真实理由：PS 读不加锁，在 READ COMMITTED 下同事务读既无一致性收益也无代价；收寄的原子范围只有 UC-NO-002 步骤 8「原子提交收寄判断、控制、审计和对应发布意图」。
   - 改正 transactionalReception 头注：意图入队报 SQL 级错误时同样整笔回滚、答 5xx。「结果已提交而意图没跟上」只在入队以 Go 层错误失败时出现，这一格的去留归operator-channel/45。
   - 随新答法改写：parcelshipment/identity.go 的 ResolveParcelIdentity「编排据此停在`收寄待确认`」；identity/intake_result_version.go 的 NextIntakeResultVersion「编排为此留了`收寄待确认`那一格」；http 适配器 receptionResponse 头注「未决带续办引用…等依赖」。
   - service_results.go 包注去掉「两件」这个计数。
8. 测试卫生与序号引用：
   - assemble_reception_service_results_test.go 里，seedParcelShipmentServiceResults 的回调与 TestTheWiredServiceResultViewReadsParcelShipmentCancellationAndFinal 的 read 闭包回调都调了 mustValue，挪到回调外先构造。回调只做 IO，并交回 error。
     - 理由：t.Fatal 系走 runtime.Goexit。bento 的 defer 只在 recover 到 panic 时回滚，遇到 Goexit 时事务既不提交也不回滚。
     - 改用 mustValueNoT 也不行：panic 虽会被回滚，却会中止整个测试进程。
   - 本票碰到的文件里，序号引用一律改成引文：
     - serviceMarkersOf 的「裁定第 2 条」，改引新 ADR；
     - TestAnUnreachableServiceResultViewLeavesTheReceptionUndecided 随新答法改名，改引新 ADR；
     - TestTheServiceResultViewIsAskedOnlyForAUniquelyIdentifiedParcel 改引票 25 裁定原句「按什么键查 → 取 (b)，在编排里、身份唯一成立时按包裹查」；
     - 真库用例的「完成判据第三条」，改引「生产装配接上 PS 真库读口」；
     - assemble_reception_test.go 的「ps-external-mark-relations/02 完成判据第一条」，改引该票判据的原句。
9. 夹具：
   - cmd/parcel-api/assemble_customer_amendment_test.go 里，seedBaggedIntakeFor 的显式列清单补上核对态列。
   - NO postgres 的 reception_test.go 用了词表外的 CANCELLED_BEFORE_ARRIVAL，改成由合成入口产出的取值。
10. 文书：
    - 新立一份 NO ADR，落地时取下一个空号，Status 为 Accepted，写明授权与能力边界。
      - Decision 照本票裁定。适用范围写明：只管 UC-NO-002 收寄编排，不管 no-directional-basis/01 的方向性读面与依据记录。
      - Consequences 写五件事：用 savepoint 隔离跨上下文读是新用法；ADR-0081、ADR-0134 那种「读挪出写路径」是结构上的备选；意图入队失败另立票；兄弟编排同形，另立票；设备遇 5xx 逐条退避、不得队首阻塞（依据 ADR-0023）。
      - Alternatives 列出：原方案 (a)、方案 (b)、两支都 5xx、身份读不动即落待识别、把读挪出事务、独立连接、改 bento。
    - docs/adr/README.md 补条目。
    - UC-NO-002 结果语义契约表之后，加一句只引不述的指针。
    - 下列票面记录，立票时已落的就略过：票 25 Comments 记明那句裁定被取代；票 26「评审结果」表后追加序 2 的去向；no-directional-basis/01 与 ps-external-mark-relations/02 的 Comments 各记一句。
11. 机制清点：用 tools/mechanism-inventory 重生成 docs/product/MECHANISM-INVENTORY.md 并提交。CI 会逐字比对这个文件。

## 不做

- 不把身份核对读不动落成待识别，归no-reception-source/01。
- 不补查「读不动」那条的标记。
- 不改复核目录与管理台，归operator-channel/43、C1-T3。
- 不改意图入队失败时的答法，也不把 savepoint 扩到意图交付口，归operator-channel/45。
- 不改承接与集运编排，归operator-channel/46。
- 不新增 5xx 细分码。
- 不改 bento，不扩事务闭包门禁。
- 本票文件以外的序号引用不在这里横扫，归operator-channel/48。

## 完成判据

- [ ] 新 NO ADR 已落，并进了 docs/adr/README.md；UC-NO-002 加了一句指针；四张票的记录已落。
- [ ] 编排回归：身份唯一而服务结果读口报错时，得 INTAKE_FORMED，控制成立，意图一封，核对态为读不动，标记为空。读口答空时，核对态为已查，标记也为空。两条记录要分得开。
- [ ] 编排回归：下列失败各交回 error，零写入、零意图：身份读口报错；收寄库 FindByKey、Save 或并发读回失败；版本签发失败。两支输入缺陷交回被拒类哨兵。缺任一依赖，在构造期报错。改写原有的 TestAnUnreachableServiceResultViewLeavesTheReceptionUndecided 与 TestAMissingServiceResultViewStopsHonestlyInsteadOfRecordingNoMarkers。
- [ ] 幂等：读不动那条落册后，同一来源、同一内容重投答 EXISTING_RESULT，不再落一条、不重签版本，核对态照读回。
- [ ] 存储：真库往返读写核对态。CHECK 要拒三种：集外取值、非 INTAKE_FORMED 却带核对态、读不动而标记非空。迁移把既有 INTAKE_FORMED 行回填为已查。
- [ ] 装饰器真库用例三条：一笔事务里内层执行 SELECT 1/0，装饰器交回错误，同一事务随后的写入能提交、能读回；可读时 RELEASE 后读数照常；无事务时直通，不发 SAVEPOINT。
- [ ] 生产装配真库用例用 DDL 法，不换读口、不改装配签名。cmd/parcel-api 与前线导入各一条：
  - 挪走 parcel_shipment.parcel_cancellation，打一次身份唯一的收寄：答 201 INTAKE_FORMED，核对态读不动，意图一封。重投答 200 EXISTING_RESULT，仍是一行。前线导入首导即已落地，退出码 0，明细写读不动。
  - 挪走 parcel_shipment.external_identifier_relation_change：答 500，零行。前线导入记未决，明细带真实原因。挪回后同一来源重投，答 201 INTAKE_FORMED，带关联，意图一封。
- [ ] HTTP：核对态信息格在 INTAKE_FORMED 的首答与重放里都在场，其他种类不带；被拒类答 400；「dependency failure is undecided」用例改写为：身份读口报错时答 500，且不带 outcome。
- [ ] 「做什么」第 7–9 条逐条完成：注释、序号引用、把 mustValue 挪出回调、两处夹具。
- [ ] 门禁：go build ./... 与 go vet ./... 退 0，gofmt -l . 无输出；带 DSN 跑 internal/nodeoperations/...、cmd/parcel-api、cmd/parcel-frontline-import、internal/architecture，全绿；机制清点重生成后，没有未提交的差异。
- [ ] 走 Go 那条路（docs/agents/parallel-sessions.md）：分支从当时的 origin/main 起；作者自验；非作者评审；推送方在干净检出上带 DSN 跑全量。

## 完成记录（2026-10-18，会话 idp-parcel-9b 的实施子代理，分支 claude-oc29）

基线 rebase 到 origin/main `320d2900`（含对端 no-directional-basis/01 的迁移 0005）。上一个实施代理留下的半成品：ports 一笔（标记合成挪到 `ParcelServiceResults.Markers`、立 `ServiceResultCheck`）与未提交的编排用例都合裁定，整理后留用。

**落点（逐笔）**：`7f7de93d` ports 合成入口与核对态枚举；`f70baa3a` 编排与 HTTP（读不动照常形成并记核对态、没形成答案交回 error、被拒类哨兵、构造期拒缺依赖、删续办引用格；被拒答 400、`serviceResultCheck` 信息格）；`3b7cbe3a` 迁移 `0006_reception_service_result_check` 与收寄库往返、两处夹具；`bf50746a` `SavepointedServiceResultView` 与两处生产装配（parcel-api `buildParcelServiceResultView`、前线导入新抽的 `parcelServiceResultViewOf`，`buildDirectionalBasisJudge` 改调它）、前线导入被拒与明细、装配头注改写、`mustValue` 挪出回调；`6b8514fa` 身份核对与版本签发头注、`service_results.go` 去计数；`279616fa` ADR-0186、adr/README、UC-NO-002 指针、票 25/26/ps-external-mark-relations/02 记录；`adaf1e28` 架构门证据与引文；`e30db7dc` 机制清点。

**完成判据逐格**：
- ADR 与文书：ADR-0186（Accepted）、README 条目、UC-NO-002 指针；票 25 Comments、票 26 评审结果表后、ps-external-mark-relations/02 Comments 各一句；no-directional-basis/01 立票时已记「operator-channel/29 顺延 `0006`」，按「已落的就略过」不动。
- 编排回归：`TestAnUnreadableServiceResultViewStillFormsTheIntakeAndSaysSo`（读不动 vs 答空两条记录只差核对态）、`TestDependencyFailuresFormNoAnswer`（身份读口、FindByKey、Save、并发读回报错与读不到赢家、版本签发）、`TestAnUnknownStoredDigestShapeFormsNoAnswer`、`TestInputDefectsAreRejected`、`TestAMissingDependencyIsRefusedAtConstruction`；原两条已改写。
- 幂等：`TestReplayingAnUnreadableReceptionAnswersTheRecordedCheck`。
- 存储：`TestTheServiceResultCheckIsReadBackUnchanged`、`TestTheReceptionTableRefusesAMisshapenServiceResultCheck`（集外取值、非形成格带核对态、读不动带标记三种 23514）、`TestTheServiceResultCheckMigrationBackfillsFormedReceptionsAsChecked`（空库按序施加到 0005、落旧形状行、再施加 0006）。
- 装饰器三条：`TestASavepointKeepsAFailedServiceResultReadFromAbortingTheReception`、`TestASavepointReleasesAReadableServiceResultUnchanged`、`TestASavepointPassesThroughOutsideATransaction`。
- 生产装配 DDL 法：parcel-api `TestTheWiredReceptionFormsTheIntakeWhenServiceResultsAreUnreadable`、`TestTheWiredReceptionFormsNoAnswerWhenIdentityIsUnreadable`（经真 HTTP 适配器，接入层换成交回固定命令的替身）；前线导入 `TestFrontlineImportLandsTheReceivedRowWhenServiceResultsAreUnreadable`、`TestFrontlineImportLeavesTheReceivedRowUndecidedWhenIdentityIsUnreadable`，另加 `TestFrontlineImportRejectsACommandTheOrchestrationRefuses`。前线导入读不动那条的身份核对用能解析的替身（`buildIntakeImporter` 的身份参数本就如此设计），服务结果读口是生产装配的；身份读不动那条挪回后重跑落地为待识别（前线导入库里没有已接受委托），「201 带关联」那半由 parcel-api 用例证。
- HTTP：`TestTheServiceResultCheckTravelsWithEveryFormedIntake`；传输用例新增「身份读口报错 500 且不带 outcome」「编排被拒答 400」，原「dependency failure is undecided」删去。
- 「做什么」里的注释、测试卫生与序号引用、夹具三项：注释、序号引用、`mustValue`、两处夹具均已改（`seedBaggedIntakeFor` 补列；`reception_test` 改用合成入口产出的标记，同包 `review_catalogue_test` 的比对随之改成比夹具值）。

**偏离与说明**：迁移的 CHECK 只拦票面「要拒三种」，不拦「INTAKE_FORMED 不带核对态」——收紧要连带改 `cmd/parcel-dispatch`、前线导入集运用例等处直接落形成格的夹具，越出地盘；编排对每条形成格都写，回填后既有行也都带。ADR-0186 的 Alternatives 里「原方案 (a)」「方案 (b)」按票 25 裁定原文与票面现象复原措辞，研判轮原稿未在仓内。operator-channel/46 与 no-reception-source/01 落地时尚未立票，ADR 如实写明。

**先红后绿**：每条新用例先在旧实现下跑出红，红落在行为上（编排答 RECEPTION_UNDECIDED 而非 error / INTAKE_FORMED；HTTP 缺 `serviceResultCheck`、被拒答 500；库无此列、迁移不存在；直通桩下同事务写入得 25P02；装饰器未接时 parcel-api 读不动答 500、前线导入记未决），再实现到绿。

**门**：`gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退 0；带 DSN `go test -p 1 -count=1` 跑 `internal/nodeoperations/...`、`cmd/parcel-api`、`cmd/parcel-frontline-import`、`cmd/parcel-dispatch`、`internal/parcelshipment/adapters/nodeoperations`、`internal/visibilityexception/adapters/nodeoperations`、`internal/architecture/...`、`migrations/...`、`internal/platform/migrate/...` 全绿；机制清点重生成已提交，无未提交差异。未跑全仓全量。

**证据层级**：`S`（隔离合成，夹具只用 `SYN-` 值）。

评审：待会话读 diff 审。

## Comments

- **评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审）· 钉 `9585b719`（基线 `320d2900`）· 2026-10-18**：用户选「精简续做」，评审由会话本身读 diff 做；代码出自本会话起的实施子代理，不算非作者评审，如实记下。
  - **Spec**：阻断无。服务结果读不动照常形成收寄、核对态记 `UNREADABLE`、标记为空；身份读不动与本方落册失败交回 error（5xx / 前线导入未决）；输入缺陷经 `ErrReceptionCommandRejected` 答 400；续办引用格删去；装配缺依赖在构造期拒。savepoint 只包服务结果读口（`SavepointedServiceResultView`），不在事务里直通；parcel-api 与前线导入（新抽 `parcelServiceResultViewOf`，集运 `buildDirectionalBasisJudge` 同改）都装上。迁移 0006 封闭 CHECK、读不动不带标记、既有形成行回填「已查」。ADR-0186 Accepted，写明授权与能力边界。
  - **Standards**：阻断无。非阻断 2：(1) 迁移不拦「INTAKE_FORMED 不带核对态」（读回映射为「不带」），生产写方恒带；要在库里拦得改 parcel-dispatch 等处地盘外的夹具，留作后续。(2) ADR-0186 Alternatives 里票 25 原方案的措辞是实施者按票 25 原文复原的，会话读过，与研判原稿方向一致。
  - **越出地盘**：`review_catalogue_test.go` 断言随共用夹具改比夹具值；`consolidation_import.go` 那处调用经对端同意。
