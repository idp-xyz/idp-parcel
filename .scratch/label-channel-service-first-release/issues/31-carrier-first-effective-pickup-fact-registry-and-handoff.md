# 31 实际承运商首次有效收寄：TF 侧事实聚合、版本链、登记册、outbox 与显式判断入口

Category: enhancement
Status: in-progress——通道 6 于 2026-09-10 按 MCP-1 派单 task-f3f0d59a 认领实施（隔离树 `D:/tops/idp-parcel-mcp6-lc31`，分支 `mcp6-lc31`，基 `76932b38`）。立票：通道 6 于同日按 MCP-1 派单 task-f05bc5a3，形状由 [ADR-0135](../../../docs/adr/0135-carrier-first-effective-pickup-is-a-judged-control-fact-with-its-own-registry-and-enters-the-segment.md) 裁定、TF CONTEXT 同笔落词条（分支 `mcp6-adr0135`）；**只写票面，未动代码。** 「要裁的」为零
Blocked by: 无（ADR-0135 与 TF CONTEXT 词条已落；本票不等 PS 侧 lc/25，反过来 lc/25 等本票）

## 缺口

票 [`25`](./25-external-carrier-first-pickup-triggers-label-final-judgment.md) 量得：PS 面单渠道服务的默认终局等的那封「实际承运商首次有效收寄」信封，TF 今天发不出来——TF 十种出向事件里没有收寄那一种，`internal/transportfulfillment` 里的「有效收寄」只有 `OffsitePickup`（自营场外揽收），`ParticipationEntryKind` 封闭二格，`actual_carrier_judgment.go` 拿外部承运轨迹事实当证据答的是「这一段是谁在承运」不是「该对象已被承运商收寄」。TF CONTEXT 又明写外部承运轨迹事实「本身不构成实际承运商首次有效收寄」，所以 PS 也不能拿 `external-carrier-tracking.judged` 顶替。

ADR-0135 裁了形状：**新一类独立控制事实**，在合格来源之上一次显式判断形成，自有登记册与 outbox 事件，按（租户，载运对象）一条版本链，已形成即参与进入（`ParticipationEntryKind` 第三格），待确认是带原因的版本但不提供，更正沿来源声明的更正关系换替代 / 失效版本。TF CONTEXT 的词条、Rules 两句、Lifecycles 一节已随 ADR 同笔落下。本票把它做进 `internal/transportfulfillment`。

## 做法

形照有效交付那一路（`EffectiveDelivery` → `EffectiveDeliveryRegistrations` → `OutboxEffectiveDeliveryHandoff`），每一步对着 ADR-0135 的决定号写，票面不复制第二套口径。

1. **领域**（`internal/transportfulfillment/domain/`，新文件 + 改一处）：
   - 聚合 `CarrierFirstEffectivePickup`（ADR-0135 决定二）：租户、载运对象、TF 另铸的事实身份、`CarrierSubject`（复用 ADR-0103 两支）、业务发生时间、判断形成时间、依据引用（一条或多条，每条带 `CarrierEvidenceSource` 与**版本**）、版本、回指的前版、版本种类（首登 / 替代 / 失效）、结果（已形成 / 待确认）与待确认原因（封闭两支：来源冲突、承运主体身份未登记）。构造门：已形成必须承运主体在册引用、业务时间在场、依据至少一条；待确认必须带原因且依据至少一条、不带承运主体已识别值；沿用原版本号即覆盖，构造期拒绝（同 `OffsitePickup.Correct`）。替代与失效两个转换门：只接受「被更正的版本恰是当前版本的依据」（ADR-0135 决定六，判据同 ADR-0112 决定二），失效版本标 voided 并继承对象与事实身份；链尾失效后再次形成从失效版本长出新版本。重建门（`*_rehydration.go`）照既有各聚合的形。
   - `ParticipationEntryKind` 加第三格 `EnteredByCarrierFirstEffectivePickup`（`String()` 答 `CARRIER_FIRST_EFFECTIVE_PICKUP`），头注「刻意没有第三格」那句改写为「第三格是已形成的首次有效收寄，不是扫描」（ADR-0135 决定五）；`segment_rehydration.go` 的 `valid()` 与 `fulfillment_segment_registry.go` 的 `participationEntryKindFrom` 同步认第三个词；`actual_fulfillment_segment_test.go` 里「第三格无字串」那条用例改为对第四格断言。
2. **端口**（`internal/transportfulfillment/ports/`，新文件）：登记册 `CarrierFirstEffectivePickupRegistrations`——追加一版；按（租户，事实，版本）取回**指名那一代**（形照 `EffectiveDeliveryRegistrations.FindByKeyAndVersion`）；按（租户，载运对象）取回整条链或当前版（供「首次」与更正定位）。handoff 端口 `CarrierFirstEffectivePickupHandoff`。事实身份签发器照 `ExternalTrackingFactIdentity` 那一族。合格来源的读回复用既有口：外部承运轨迹事实按（事实，版本）读回并核有效时间已判断（`ExternalTrackingFacts.FindByKey` 或按版本的那一口，实施时以代码为准）；承运主体在册核对复用 `adapters/partycommercial/carrier_identity_directory.go` 那只消费侧适配器所实现的端口。
3. **应用**（`internal/transportfulfillment/application/`，新文件）：`JudgeCarrierFirstEffectivePickupHandler`——显式判断入口（ADR-0135 决定四 / 八）。输入：一条合格来源引用（带来源种类与版本）、它指名的承运主体（在册引用或名称素材）、判断方对「表达已接收实物或取得运输控制」的显式读法、可选的段引用与段服务动作声明。步骤：读回来源事实并核可用（轨迹事实有效时间待判断 → 不可用，答一格）；查对象当前有效参与（`ActualFulfillmentSegmentRegistry` 在场谓词）→ 在场答「非首次」不落版本；查名称素材在 PC 身份读口 → 不在册落待确认（承运主体身份未登记）；已有待确认版本且新依据指向不同承运主体 → 待确认（来源冲突），全部依据保留；否则已形成，登记后交 handoff；已形成之后走 `enterFulfillmentSegment` 同一道门（入场种类第三格、入场依据指向（收寄事实，版本）；段引用缺席不进段不算失败，进段失败留续办不回滚登记——`enterFulfillmentSegment` 头注原纪律）。结果代数按恢复动作分格（ADR-0029），至少：已形成 / 待确认（两支）/ 不构成 / 非首次 / 来源不可用 / 未决。**重派生挂点**：依据的轨迹事实被源声明更正而形成新版本时，若被更正的那一代恰是某条收寄链当前版本的依据 → 更正后仍表达收寄按替代版本、不再表达按失效版本落新版并交 handoff，参与关系经 `rederiveFulfillmentParticipation` 同一道门重派生（ADR-0135 决定五 / 六）；触发点挂在轨迹事实更正落库之后的一拍，与 lc/16 交接更正 → 参与重派生那条同形，实施时以 `rederive_fulfillment_participation.go` 现有两条来源的挂法为准。
4. **postgres**（`internal/transportfulfillment/adapters/postgres/` + `migrations/transport_fulfillment/`）：追加式版本表（一行一版本），迁移编号取实现落地时 TF 的下一号（不在票面写死）；CHECK 逐条镜像构造门（已形成必有承运主体与业务时间；待确认必有原因；失效必回指前版；同一（租户，载运对象）至多一条链——用事实身份对（租户，对象）的唯一性表达）。`OutboxCarrierFirstEffectivePickupHandoff`：事件类型 `transport-fulfillment.carrier-first-effective-pickup.registered`，载荷 `{tenantId, fact, version, object}`，ID 取（租户 + 事实 + 版本 + 类型段），分区键取（租户 + 载运对象 + 类型段），**待确认版本到此响亮拒绝**（照 `OutboxExternalTrackingFactHandoff` 对待判断版本的那一道门）（ADR-0135 决定七）。
5. **在线登记口**（`internal/transportfulfillment/adapters/http/` + 装配）：`POST /transport-fulfillment-carrier-first-effective-pickup-judgments`——显式判断的入口，形照 lc/21 的 `POST /transport-fulfillment-effective-time-judgments`；查阅读口 `GET /transport-fulfillment-carrier-first-effective-pickups?object=`（按对象看链）。管理台页面**不在本票**（先有事实再谈面，同 lc/16 刻意留下的第三格那条理由；若 owner 要页，另立票）。装配处照 `OutboxEffectiveDeliveryHandoff` 与 lc/21 两个口所在的装配文件。

**刻意不含**（各有归处，不是欠账）：收寄判读规则目录与规则那一路的异步节拍（ADR-0135 决定八——今天没有任何一家真源与任何一版规则，随第一家真源立，照 lc/19 / lc/20 的形）；PS 侧消费者与处理方（lc/25）；有效交付在外部承运轨迹事实之上的判断（另一题）。

## 要裁的

无。ADR-0135 六条越权风险点归 owner 事后复核，不阻塞本票开工；复核若改动决定三（时间取源发生时间）或决定五（段引用由 TF 铸），改的是本票做法 1 / 3 各一格。

## 红线

- 不写任何真实轨迹源、承运商、状态词表或「哪些状态词算收寄」的映射（`PAR-INT-02` 实例半边）；规则未配置如实答未配置，不形成版本。
- 不改 `ExternalCarrierTrackingFact` 的任何导出签名与表结构——收寄不长在它身上（ADR-0135 决定一）。
- 不给 `ParticipationEntryKind` 既有两格改名或改语义；只加第三格。
- 业务发生时间取自依据、不取墙钟、不取接收时间；判断形成时间另记（ADR-0135 决定三）。
- 已形成之后另一来源的相反证据不收回收寄，只进实际承运商判断（ADR-0135 决定六）。
- 不动 `internal/parcelshipment/**`（PS 半边归 lc/25）。
- Go 注释一律中文；跨文件引用用符号名与决定号，不用行号不计数。

## 完成判据（非作者评审逐项对）

1. 领域：构造门对「已形成缺承运主体 / 缺业务时间 / 缺依据」「待确认缺原因」「沿用原版本号」各拒一格；替代与失效转换门对「被更正的不是当前依据」拒；链尾失效后再次形成回指失效版本；`ParticipationEntryKind` 第三格 `String()` 与解析往返，第四格仍答空串——各有用例。
2. 应用：显式判断六格结果各有用例；「对象已有当前参与 → 非首次不落版本」有用例（先经场外揽收或`已交接`进段再判）；「轨迹事实有效时间待判断 → 来源不可用」有用例；「名称素材不在册 → 待确认（未登记）→ 登记后同依据形成已形成」有用例；「两条依据指向不同承运主体 → 待确认（来源冲突）」有用例；已形成后 `enterFulfillmentSegment` 铸出参与且实际承运商判断首版为已识别、依据即收寄依据——有用例；段引用缺席时事实照登、事件照发、不进段——有用例；重派生：依据更正后替代版本 → 替代参与版本、失效版本 → 失效参与版本——各有用例。
3. postgres：真库往返（首登 / 替代 / 失效三代同链）、`FindByKeyAndVersion` 交回指名那一代而不是链尾、CHECK 对「待确认却带承运主体」拒；outbox 对待确认版本拒绝入队、对已形成 / 替代 / 失效三代各入一份且 ID 不同、分区键相同——各有用例，`-v` 下 PASS 非 SKIP。
4. 在线口：显式判断的 POST 走到处理方并按结果分格答 HTTP；GET 按对象交回链；隔离读放行按 ADR-0078 判入格。
5. `gofmt -l` 空、`go build` / `go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库；机制清点在干净检出上重生成。
6. 票面 Comments 写完成记录与「刻意不含」核对，并在 lc/25 的 Blocked by 处回填「31 已 resolved」。

## 地盘

`internal/transportfulfillment/domain/`（新文件 + `actual_fulfillment_segment.go` 的 `ParticipationEntryKind` 三处 + `segment_rehydration.go` 一处 + 对应测试）、`internal/transportfulfillment/ports/`（新文件）、`internal/transportfulfillment/application/`（新文件 + 重派生挂点）、`internal/transportfulfillment/adapters/postgres/`（新文件 + `fulfillment_segment_registry.go` 的 `participationEntryKindFrom` 一处）、`internal/transportfulfillment/adapters/http/`（新文件）、`migrations/transport_fulfillment/`（新迁移）、装配文件里对应的两扇门、本票面。**不动** `internal/parcelshipment/**`、`internal/visibilityexception/**`、`docs/**`（落文已随 ADR-0135 完成）。

## 参照

[ADR-0135](../../../docs/adr/0135-carrier-first-effective-pickup-is-a-judged-control-fact-with-its-own-registry-and-enters-the-segment.md) 全文；TF CONTEXT「实际承运商首次有效收寄」词条、「履约主体与承运证据」Rules、同名 Lifecycles 节；ADR-0102 决定三（两种判断来源）、ADR-0103（`CarrierSubject` / `CarrierEvidenceSource`、段成立时铸首版）、ADR-0112（替代 / 失效参与版本）、ADR-0114（异步一拍）、ADR-0029（按恢复动作分格）；`internal/transportfulfillment/domain/effective_delivery.go`、`offsite_pickup.go`、`actual_fulfillment_segment.go`、`actual_carrier_judgment.go`、`external_carrier_tracking_fact.go`；`internal/transportfulfillment/application/enter_fulfillment_segment.go`、`rederive_fulfillment_participation.go`、`register_offsite_pickup.go`；`internal/transportfulfillment/adapters/postgres/effective_delivery_handoff.go`、`external_tracking_fact_handoff.go`、`fulfillment_segment_registry.go`；`internal/transportfulfillment/ports/ports.go` 的 `EffectiveDeliveryRegistrations`；票 `16`（`03 → 16` 先落文再实现）、`19` / `21`（规则目录与在线判断面的形）、`24`（按版本取回）、`25`（PS 半边）。

## Comments

- 2026-09-10 · 通道 6（task-f05bc5a3，基 `062f5228`，分支 `mcp6-adr0135`）：立票。**只写票面，未动代码。** 能力边界写在 ADR-0135 的 Status 行，此处不复制；票面「做法」里凡写「实施时以代码为准」的地方，是我没读全文只按符号名与头注判的：`rederive_fulfillment_participation.go` 两条来源的挂法、`ExternalTrackingFacts` 按版本读回的那一口、lc/21 两个口的装配文件。
