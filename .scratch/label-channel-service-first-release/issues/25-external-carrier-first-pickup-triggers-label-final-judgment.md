# 25 实际承运商首次有效收寄到达 → `JudgeLabelServiceFinalHandler`：PS 侧 inbox 消费者，以及它今天收不到的那封信

Category: enhancement
Status: ready-for-agent——2026-09-11 14:1x 通道 1 推送方改口：[`31`](./31-carrier-first-effective-pickup-fact-registry-and-handoff.md) 已于 2026-09-10 19:30 进 main（非作者评审两轴 0 阻断，见票 31「进 main 记录」），本票「前提」四件（租户、事实、版本、载运对象 + 读口按版本交指名那一代）全在 main；「要裁的」1 由 ADR-0135 答毕，标已裁；新增「要裁的」2（失效版本重派生，归 PS owner）**不阻开工**——实施时该格先按红线落显式未决并在完成记录点名，owner 裁后另笔。此前 draft——通道 3 于 2026-09-10 按 MCP-1 派单 task-b5dba034 立票，取证锚远端 main `c7e3522c`；**只写票面，未动代码。** PS 半边形状已定（照有效交付那一路）；它要消费的 TF 信封的形状已于同日由 [ADR-0135](../../../docs/adr/0135-carrier-first-effective-pickup-is-a-judged-control-fact-with-its-own-registry-and-enters-the-segment.md) 裁定（见「裁决」），TF 半边立为 [`31`](./31-carrier-first-effective-pickup-fact-registry-and-handoff.md)
Blocked by: 无——`31`（TF 侧「实际承运商首次有效收寄」事实、登记册与信封）**已进 main（2026-09-10 19:30，分支 `mcp6-lc31` tip `1d9a24c0` 作封存出处，main 上 SHA 见票 31「进 main 记录」）**；信封类型 `transport-fulfillment.carrier-first-effective-pickup.registered`、载荷 `{tenantId, fact, version, object}`、读口 `CarrierFirstEffectivePickupRegistry.FindByKey`（指名那一代）与 TF 侧记录形状均已落地，本票可开工。另两件**不阻塞**：`Deps.Validity` 填 [ps-port-remainder/01](../../ps-port-remainder/issues/01-label-validity-rule-is-a-lapse-declaration-on-the-final-rule.md) 的适配器，它未进 main 前填 nil（编排按「未配置」办，不推算失效）；PC 半边 `DeclaredResponsibilityOutcome` 加面单渠道两行未立票，落地前采用停在 `FINAL_RULE_UNCONFIGURED`，是诚实停点不是本票的阻塞

## 缺口

票 [`11`](./11-parcel-final-across-transactions.md) Answer「不在本票」节点名 `JudgeLabelServiceFinalHandler` 的三个调用方各自一张接线票，本票是第一路：**TF 实际承运商首次有效收寄事实到达**。`cmd/` 下 `NewJudgeLabelServiceFinalHandler` 零命中（`c7e3522c`）。

**命令形状已定。** `application/judge_label_service_final.go` 的 `JudgeLabelServiceFinalCommand` 四件：`Identity`（委托来源身份）、`ShipmentRequestID`、`Parcel`、`FirstEffectivePickup`（`domain.CarrierFirstEffectivePickupSpec{Fact, Version, EffectiveAt}`，零值即缺席）。收寄那一路带它进来，引用本体在编排里经 `ReferenceCarrierFirstEffectivePickup` 立起，三件缺一不立（`Handle` 头注：半份引用是提交矛盾，构造期就拦）。`JudgeLabelServiceFinalDeps` 六口：`Transactions`（`LabelTransactionsByParcelView`）、`Registers`（`ContinuedAttemptRegisterView`）、`Cancellations`、`Validity`（可 nil）、`Adoption`（`ParcelFinalAdopter`，生产接 `FormParcelFinalHandler`）、`Clock`。

**TF 今天发出去的信封不是这条命令要的那件事实。** lc/11 Answer 写的是「`external-carrier-tracking` 信封的 PS 侧 inbox 消费者」，`unresolved-review-20260904/remaining-work-a3a4814.md`「面单渠道链」第 2 条照抄。按代码与 TF CONTEXT 量，这句话把两件事叠在了一起：

- TF 出的是 `transport-fulfillment.external-carrier-tracking.judged`（`adapters/postgres/external_tracking_fact_handoff.go` 的 `OutboxExternalTrackingFactHandoff`）：只对**有效时间已判断**的版本入队；载荷指针式三维 `{tenantId, fact, version}`；分区键（租户 + 载运对象 + 类型段）。今天唯一消费方是 `veinbox.ExternalTrackingConsumer`（`cmd/parcel-dispatch/assemble.go` 路由表里那一行），PS 侧无消费者。
- TF CONTEXT「外部承运轨迹事实」Rules 明写：「外部承运轨迹事实本身**不构成**实际承运商首次有效收寄、权威运输交接、实际移动或有效交付。它可以作为这些判断的合格来源之一……但判断仍按各自规则、结合其他证据另行形成」；「认领不解释、不映射原始状态词——状态词的含义属该轨迹源的已登记规则，对它的解释是另一次显式判断」。事实上的 `RawStatusReference` 头注同一句话。
- TF CONTEXT「履约主体与承运证据」给了首次有效收寄的判据：「必须明确关联载运对象、实际承运商、业务发生时间，并表达其已经接收实物或取得运输控制。合格来源可以包括承运商直接收寄扫描、承运商收货凭证、权威运输交接结果或保留原始承运来源的可信渠道回传」；「来源冲突且尚不足以裁决时保持待确认，不触发……取消权结束或其他依赖有效收寄的边界」。
- **代码里没有这一类事实。** `internal/transportfulfillment` 全目录搜 `FirstEffectivePickup` / `首次有效收寄`，只命中 `transport_commission.go` 及其测试里「接受不等于收寄」的注释；TF 十种出向事件类型（`adapters/postgres/*_handoff.go` 各自的 `*EventType` 常量）里没有收寄那一种；TF 代码里的「有效收寄」是 `OffsitePickup`（场外揽收，自营网络那一路），`ActualFulfillmentSegment` 的 `ParticipationEntryKind` 封闭二来源也只有场外揽收与`已交接`的权威交接；`actual_carrier_judgment.go` 的实际承运商判断把外部承运轨迹事实当合格证据，但它答的是「这一段的实际承运商是谁」，不是「该对象已被承运商首次有效收寄」。

所以 PS 侧消费者若直接收 `external-carrier-tracking.judged`、把 `{fact, version, effectiveAt}` 折成 `FirstEffectivePickup`，就是 PS 替 TF 做了「这条状态词算收寄」的判断——违反 TF CONTEXT 上引两句，也违反 CONTEXT-MAP「`transport-fulfillment → parcel-shipment`：运输履约提供……实际承运商首次有效收寄……证据冲突或实际承运商待确认时不得提前触发该边界」。**本票 PS 半边的形状能写，但它等的那封信要 TF 先立**。

**另一件 PS 半边要自己解的：信封不带包裹。** TF 信封里只有事实引用与载运对象；`JudgeLabelServiceFinalCommand` 要委托来源身份、委托标识与包裹。先例在 `adapters/transportfulfillment/adopt_on_effective_delivery.go`：按引用读回 TF 记录 → 载运对象串原样进 `psdomain.NewDeclaredParcelID` → `CurrentAcceptedParcelTargetView.FindCurrentAcceptedByParcel` 反查当前已接受委托 → `Identity()` / `ShipmentRequestID()`；该文件头注写明「载运对象……可能指正式包裹身份，也可能指集运单元……今天不替它猜」，反查不中落 `ErrParcelTargetNotFound`。本票照抄这条纪律。

## 做法（PS 半边；TF 半边落地后才可开工）

前提：TF 侧信封已存在，且它交出的东西至少含（租户、收寄事实引用、版本、载运对象），事实本体可按引用只读取回并带**已判断**的有效时间——`CarrierFirstEffectivePickupSpec.EffectiveAt` 就是它，编排把它当终局的生效时间（`responsibilityOutcomeOf` 的 `OccurredAt: verdict.EffectiveAt()`）。TF 那一类事实叫什么、是新一类还是外部承运轨迹事实之上的一次判断，归 TF owner 建模（「要裁的」1），本票不预判；但 PS 领域 `CarrierTrackingFactReference` 头注今天写的是「指名 transport-fulfillment 拥有的一条外部承运轨迹事实」——TF 若立新一类事实，那句头注要随本票改口，类型名是否随之改归实施时判。

1. **inbox 消费者** `internal/parcelshipment/adapters/inbox/`（新文件，形照 `effective_delivery_consumer.go`）：稳定消费者名（与交付 / 收寄 / 揽收各路分账——inbox 键只由消费者名 + 来源 + 事件 ID 认领，共名会让一路把另一路的投递当重复跳过）；事件类型常量由消费方自己写出字符串，不导入 TF outbox 未导出常量；`Decode` 三维（租户 + 事实 + 版本）缺一即 `ErrPoisonEnvelope`；门走 `inboxconsume.New`。
2. **处理方适配器** `internal/parcelshipment/adapters/transportfulfillment/`（新文件，形照 `AdoptOnEffectiveDeliveryAdapter`）：按（租户 + 事实 + 版本）向 TF 只读口取回那一代——**按信封所指版本取，不取 latest**（票 [`24`](./24-source-correction-version-refused-as-second-responsibility-start.md) 的教训：更正换版后 `FindByKey` 答链尾，信封所指与读回的不是同一代）；键与本体不符、或读回的有效时间仍待判断 → 记录不一致那一格（照 `ErrDeliveryRecordInconsistent`：不进未决哨兵，重投改不了坏行）；读不回 → 可见性滞后那一格（照 `ErrDeliveryNotVisible`：续办，可重投）；载运对象 → `DeclaredParcelID` → `FindCurrentAcceptedByParcel` → 折成 `JudgeLabelServiceFinalCommand{Identity, ShipmentRequestID, Parcel, FirstEffectivePickup{Fact, Version, EffectiveAt}}` → `Handle`。租户串从信封带到两次查询（TF 取回与 PS 反查用同一个，照交付适配器那句注释）。
3. **结果译成消费结论**（按恢复动作分格，ADR-0029）：`LabelServiceFinalAdopted` → 取 `Adoption()` 交 `finalconsume.Consumption`（与交付那一路同一处回答终局采用两格）；`LabelServiceNotFinalOutcome` / `LabelServiceCancellationStandsOutcome` → 已消费（判断到了、不形成，无事可续）；`LabelServiceJudgmentUndecided` → 未决哨兵（四个读口之一答不出，重投会改变结果）；`LabelServiceJudgmentNotAccepted` → 命令立不起（信封与反查都过了还立不起是适配器缺陷）→ 响亮报错不吸收。**实施时逐格对着 `LabelServiceFinalOutcome` 的五值写测试，一格不漏。**
4. **装配** `cmd/parcel-dispatch/assemble.go`：PS 全部 inbox 消费者都装在这里（`psinbox.NewEffectiveDeliveryConsumer` 那一扇门的写法），路由表加一行事件类型。`JudgeLabelServiceFinalDeps`：`Transactions` ← `pspostgres.NewLabelTransactions(db)`（它的 `ListByCoveredParcel` 满足 `LabelTransactionsByParcelView`）；`Registers` ← `pspostgres.NewContinuedAttemptRegisters(db)`（同一适配器满足只读半边）；`Cancellations` ← 交付那一路 `FormParcelFinalDeps` 用的同一个取消视图；**`Validity` ← ps-port-remainder/01 的适配器，它未进 main 前填 nil（未配置）**；`Adoption` ← 交付那一路装好的 `FormParcelFinalHandler`（其 `Rules` 适配器对面单渠道两格今天如实答「终局规则未配置」，采用停在 `FINAL_RULE_UNCONFIGURED`——生产上这条链在 PC 半边落地前走到这里就停，票面要写明这是停点不是失败）；`Clock`。
5. **幂等 / 失败不后退**：inbox 键管一次投递只处理一次；判断本身不写；采用按（租户 + 包裹 + 来源种类 + 来源版本）幂等，来源版本 = 收寄事实版本，同版本重放返原、新版本走重派生（lc/11 Answer 原句）。消费者返错 → dispatch 重投；毒丸 → 入账交 nil；不一致 → 不进 `WithUndecidedSentinels`。

**被否的捷径，写下来免得有人走**：拿 `visibility-exception` 的里程碑分类（VE 对原始状态词的投影映射）当收寄触发。VE 是客户可见投影，CONTEXT-MAP 把「实际承运商首次有效收寄」判给 TF 提供、PS 消费，中间没有 VE；用投影反推事实是拿下游解释顶替上游判断。

## 要裁的

1. **（已裁，见下节「裁决」；TF 半边已落 `31` 并进 main）** ~~TF 侧「实际承运商首次有效收寄」那张票立不立、立在哪。~~ 原题：本目录有 TF 地盘票的先例（`16` / `18` / `19` 都落 TF），也可另立 TF 目录。它的形状（新一类 TF 事实带自己的登记册与 outbox？还是对外部承运轨迹事实之上的一次显式判断形成新版本并另发一种事件？`ParticipationEntryKind` 要不要长第三格？）归 TF owner 走 `/domain-modeling`。答案：ADR-0135 决定一 / 五 / 七——新一类独立控制事实、`ParticipationEntryKind` 长第三格、信封 `transport-fulfillment.carrier-first-effective-pickup.registered` 载荷 `{tenantId, fact, version, object}`；PS 半边的「前提」四件全部成立，`CarrierTrackingFactReference` 头注改口随本票（见「裁决」末条）。
2. **失效版本到达后，已据前版形成的非取消终局怎么重派生**（ADR-0135 决定六 / 越权风险点 4）。TF 会发失效版本（依据被源更正为不再表达收寄）；`LabelServiceFinalOutcome` 五值之外这是第六格。归 PS owner。**不阻开工**：实施时该格先按 AGENTS.md 红线「未确认规则保持显式未决」落**未决哨兵**（不吸收、不重派生、不写任何采用），用例钉住「失效版本 → 未决且零写入」，完成记录点名此格待裁；owner 裁定后（可循 ADR-0117 同来源更正的采用版本链）另笔接。（2026-09-11 通道 1 推送方按 A 类代裁「怎么停」，「怎么重派生」仍归 owner。）

### 裁决（2026-09-10，通道 6 按 task-f05bc5a3 用户授权 TF owner 口径代裁，落 [ADR-0135](../../../docs/adr/0135-carrier-first-effective-pickup-is-a-judged-control-fact-with-its-own-registry-and-enters-the-segment.md)）

**立，落本目录 `31`，形取「新一类 TF 事实带自己的登记册与 outbox」。** 对着上面三问逐条指决定号，正文以 ADR 为准不在此复制：

- 新一类还是轨迹事实之上的新版本 → ADR-0135 决定一：新一类独立控制事实，与 `EffectiveDelivery` 对称；不长在轨迹事实上，因为它必须能从四种合格来源中的任一种形成。依据以（来源事实，版本）引用留在事实上（决定二）。
- `ParticipationEntryKind` 要不要长第三格 → 决定五：**长**。已形成即参与进入，走 `enterFulfillmentSegment` 同一道门；头注「刻意没有第三格」的理由（扫描立不起参与）对判过「取得运输控制」的控制事实不成立。
- 信封与 PS 契约 → 决定七：事件 `transport-fulfillment.carrier-first-effective-pickup.registered`，载荷 `{tenantId, fact, version, object}`——恰是本票「前提」那四件；版本进 ID、分区按对象；只有已形成 / 替代 / 失效三种版本入队，**待确认不入队**；读口按（租户，事实，版本）交指名那一代。本票做法 2 的「按信封所指版本取、不取 latest」与「读回的有效时间仍待判断 → 不一致格」都成立——后者在 TF 侧对应「取回的是待确认版本」。
- `EffectiveAt` 是什么 → 决定三：TF 事实上的**业务发生时间**，依据是轨迹事实时取它已判断的有效时间；PS 的 `EffectiveAt` 读的就是这一格，不是第二个时间。
- **本票新增一件要接的**：TF 会发**失效版本**（依据被源更正为不再表达收寄，决定六）。已据前版形成的非取消终局在依据失效后怎么重派生归 PS owner（ADR-0135 越权风险点 4）——实施本票时对着 `LabelServiceFinalOutcome` 五值之外还要答这一格，ADR-0117 同来源更正的采用版本链是可循的形；本票「做法」3 的分格表实施前补这一行。
- `CarrierTrackingFactReference` 头注「指名 transport-fulfillment 拥有的一条外部承运轨迹事实」自 ADR-0135 起不成立：**随本票改口**为「指名 transport-fulfillment 拥有的一条实际承运商首次有效收寄事实（ADR-0135）」，类型名是否随之改（如 `CarrierFirstEffectivePickupFactReference`）归实施时判；改口占 `internal/parcelshipment/domain/` 一处，按本票「地盘」原句另发占号。UC-PS-004 依据表那行括注「（外部承运轨迹事实）」同样要改口（越权风险点 5），归 PS owner 随本票或另笔。

## 红线

- PS 不判「这条状态词算不算收寄」——那是 TF 的判断（TF CONTEXT「外部承运轨迹事实」Rules）；PS 只引用 TF 判过的事实与它的有效时间，一列不复制。
- 不动 `JudgeLabelServiceFinalHandler` 的编排与 `JudgeLabelServiceFinal` 的领域判断；调用方只带四件进来。
- `Deps.Validity` 缺席即 nil，不拿任何时长、不拿墙钟推算失效（ps-port-remainder/01 红线原句）。
- 载运对象不是包裹时不猜（交付适配器头注原句），反查不中如实落 `ErrParcelTargetNotFound`。
- 不写任何真实轨迹源、承运商、状态词表（实例半边 `PAR-INT-02`）。

## 完成判据（非作者评审逐项对）

1. `adapters/inbox/` 新消费者：三维缺一毒丸、事件类型不符拒收、消费者名与既有各路不同——各有用例。
2. `adapters/transportfulfillment/` 新处理方：按信封所指**版本**取回（用例里放两代事实，断言读的是信封那一代）；键与本体不符 / 有效时间待判断 → 不一致格且不在未决哨兵；读不回 → 可见性滞后格；反查不中 → `ErrParcelTargetNotFound`；`LabelServiceFinalOutcome` 五值逐格译成消费结论，各有用例。
3. `cmd/parcel-dispatch/assemble.go`：路由表有该事件类型；`Deps.Validity` 按 ps-port-remainder/01 是否进 main 填适配器或 nil，装配处注释写明是哪一种；`assemble_test.go` 照既有各路补一条「该类型有路由」。
4. `CarrierTrackingFactReference` 头注与 TF 实际立的事实类别一致（若 TF 立新一类，头注改口）。
5. `gofmt -l` 空、`go build` / `go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库（本票不动 `.sql`，真库只证既有适配器读回）。
6. 票面 Comments 写「无生产调用点之外的欠账」核对：PC 半边未落时采用停在 `FINAL_RULE_UNCONFIGURED`，作为停点写进完成记录。

## 地盘

`internal/parcelshipment/adapters/inbox/`（新文件）、`internal/parcelshipment/adapters/transportfulfillment/`（新文件）、`cmd/parcel-dispatch/`（`assemble.go` 路由表与一个装配函数 + `assemble_test.go` 一条）、本票面。**不动** `internal/transportfulfillment/**`（TF 半边归「要裁的」1 那张票）、不动 `internal/parcelshipment/application/**` 与 `domain/**`（`CarrierTrackingFactReference` 头注若要改口，另发占号）。

## 参照

票 `11` Answer「不在本票」节；`internal/parcelshipment/application/judge_label_service_final.go`（`JudgeLabelServiceFinalCommand` / `JudgeLabelServiceFinalDeps` / `Handle` 头注 / `responsibilityOutcomeOf`）；`internal/parcelshipment/domain/label_service_final.go`（`CarrierTrackingFactReference` 头注、`CarrierFirstEffectivePickupSpec`、`ReferenceCarrierFirstEffectivePickup`）；`internal/transportfulfillment/adapters/postgres/external_tracking_fact_handoff.go`；`internal/transportfulfillment/domain/external_carrier_tracking_fact.go`（`RawStatusReference` 头注、`AdoptExternalCarrierTracking`）；`internal/visibilityexception/adapters/inbox/external_tracking_consumer.go`；`internal/parcelshipment/adapters/inbox/effective_delivery_consumer.go`；`internal/parcelshipment/adapters/transportfulfillment/adopt_on_effective_delivery.go`；`cmd/parcel-dispatch/assemble.go` 交付那一扇门；TF CONTEXT「外部承运轨迹事实」Rules、「履约主体与承运证据」；PS CONTEXT 生命周期「`transport-fulfillment` 提供实际承运商首次有效收寄事件 → 面单渠道服务非取消终局结果」；CONTEXT-MAP `transport-fulfillment → parcel-shipment` 那一行；ADR-0025、ADR-0029、ADR-0102；票 `24`（按版本读回的教训）；ps-port-remainder/01。

## Comments

- 2026-09-10 · 通道 3（task-b5dba034，取证锚 `c7e3522c`）：立票。**只写票面，未动代码。** 能力边界：读过 lc/11 Answer 全文、`judge_label_service_final.go` 全文、TF `external_carrier_tracking_fact.go` / `ports/external_tracking_fact.go` / `external_tracking_fact_handoff.go` 全文、VE 与 PS 的 inbox 消费者各一只、交付那一路的处理方适配器全文、`cmd/parcel-dispatch/assemble.go` 的路由表与交付那一扇门、TF CONTEXT 上引两节、`actual_carrier_judgment.go` 头注；**没读** TF `actual_fulfillment_segment.go` 全文与 `register_offsite_pickup.go` 编排（「TF 有效收寄今天只有场外揽收」按 `ParticipationEntryKind` 头注与 grep 结果判，TF owner 开工以代码为准）。**票面与 lc/11 Answer 的一处口径差**：lc/11 把本路写成「`external-carrier-tracking` 信封的 PS 侧 inbox 消费者」，本票量出那封信不是收寄事实，以本票为新；已在 lc/11 Comments 追一条指过来。
- 2026-09-10 · 通道 6（task-f05bc5a3，基 `062f5228`，分支 `mcp6-adr0135`）：「要裁的」1 由 ADR-0135 答，裁决要点写进「要裁的」下的「裁决」小节；Blocked by 改为 `31`；新增一件本票要接的（失效版本）与 `CarrierTrackingFactReference` 头注改口的落法。**只写票面，未动代码。** 未改「做法」正文与「完成判据」——它们对着 ADR-0135 逐条核过仍成立，只多出失效版本那一格，记在「裁决」里等实施时补进分格表。
- 2026-09-11 14:1x · 通道 1 推送方：`31` 已进 main（09-10 19:30）而本票仍 draft、「要裁的」1 仍写着立不立——按 11:2x 节记下的「先按 ADR-0135 改口」办：要裁的 1 标已裁并抄答案；失效版本那一格从「裁决」末段抬成「要裁的」2（归 PS owner，怎么停已按红线定为显式未决，怎么重派生待裁），Status → ready-for-agent，Blocked by 改「31 已进 main」，lc spec 25 行同步。**只改 .md，未动代码。**
