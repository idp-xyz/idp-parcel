# 31 实际承运商首次有效收寄：TF 侧事实聚合、版本链、登记册、outbox 与显式判断入口

Category: enhancement
Status: resolved——2026-09-10 19:5x 通道 1 推送方重放进 main（非作者评审 ← 通道 2 两轴 0 阻断、Spec 非阻断两条候选后继票；main 上 SHA 与分支 SHA 对照见 Comments「进 main 记录」）。此前 resolved——通道 6 于 2026-09-10 按 MCP-1 派单 task-f3f0d59a 实施完毕（隔离树 `D:/tops/idp-parcel-mcp6-lc31`，分支 `mcp6-lc31`，基 `76932b38`，代码 tip `1d9a24c0`、清点笔 `27bb2dc1`；main 上的 SHA 由推送方重放后在 Comments 补），完成记录见文末，等非作者评审。立票：通道 6 于同日按 MCP-1 派单 task-f05bc5a3，形状由 [ADR-0135](../../../docs/adr/0135-carrier-first-effective-pickup-is-a-judged-control-fact-with-its-own-registry-and-enters-the-segment.md) 裁定、TF CONTEXT 同笔落词条（分支 `mcp6-adr0135`）；**只写票面，未动代码。** 「要裁的」为零
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
- 2026-09-10 · 通道 6（task-f3f0d59a，基 `76932b38`，分支 `mcp6-lc31`）：实施完毕，见「完成记录」。
- **评审 ← 通道 2 · 钉 `579b89ab`（代码 tip `1d9a24c0`，基线 `76932b38`，30 文件 +4144/−20）· 2026-09-10 19:37 中间态即终报**（task-21e07f77，非作者，隔离树 `%TEMP%\idp-review-lc31`；由推送方代落）。评审会话在交终报前 crash（用户 19:4x 告知），终报未到；中间态写明「主评已核完领域 `carrier_first_effective_pickup.go` / 迁移 0020 / application `judge_*` / postgres handoff + registry `Save` / http 分格 / cmd 装配」，只剩评审侧自己的隔离测试 pass 在跑——那一半不是评审的义务（验证归作者与推送方，推送方带 DSN 全量已钉），故以此为评审结论。**八道判断题评审未及逐条作答**，留给 lc/25 作者与 owner 复核时连看。
  - **Standards**：**阻断 无**。**非阻断 2**：(1) `rederive` 的 `object` 形参未用、`_ = object` 压警告（判断题：要么用要么去）；(2) 分支中间几笔单独检出不可编译（作者在 `1d9a24c0` 自报），tip 可编。
  - **Spec**：**阻断 无**。**非阻断 2（都是语义格，候选一张后继票）**：(1) `Judge` 的 `objectUnderControl` 只在 `!found || Voided` 时问；链尾为待确认时（`formOrHold` 从 pending `Supersede` 成已形成、`reconsiderPending`）不再核对象是否已有当前有效参与——对象在待确认期间凭场外揽收 / 已交接进段，之后再登记身份，会落已形成并交意图，进段被拒只留 refusal；按 ADR-0135 决定四「非首次不形成版本」与 TF CONTEXT「已处于运输控制中时新到承运证据不构成首次」，待确认转已形成那一步也该问一次首次判据。(2) `resolveBasis` 对轨迹事实来源在命令未指名时自动把 `corrects` 填成该代的回指（`record.Fact.Supersedes()`），而 `Judge` 对 `corrects != "" && !found` 一律答 `BASIS_NOT_CURRENT`，调用方无法清空——对象上第一条被判的轨迹版本若本身是更正代（v1 未判、v2 更正 v1 才表达收寄），这条链永远形成不了收寄；用例只盖了「链存在但依据不同」那格（`judge_carrier_first_effective_pickup_test.go` 断言 `CarrierPickupBasisNotCurrent` 的那条）。推送方读过 `judge_carrier_first_effective_pickup.go` 的 `Judge` / `resolveBasis` 两段核实两条属实：都是保守失败（拒绝而非误形成）且结果词可见，不挡合入；**修法都在编排一处**（(1) 待确认 → 已形成前补问 `objectUnderControl`；(2) 自动派生的 `corrects` 在 `!found` 时视作首次判断而不是更正），归作者另立 lc 后继票，票号待通道 6 立。
- **进 main 记录（通道 1 推送方，2026-09-10 19:5x）**：隔离 detached 树 `%TEMP%\idp-replay-lc31` 先叠在 lc/26 重放 tip `83ab2032` 上 pick 八笔（代码七笔 + 认领笔全干净——`internal/architecture/partition_subject_registry_test.go` lc/26 / lc/31 两行分属 PS / TF 两段、git 自动合、gofmt 空；票面笔 `579b89ab` 在 lc spec 子票表撞 main 的对齐版 `dede3c2e`：取 main 版为底、31 行取分支版、其余行不动），作者清点笔 `27bb2dc1` 不带、tip 重生成 `58480cf2`（对作者那份只差 lc/26 的 8 行）。**验证钉 `58480cf2`**：`gofmt -l` 空；`go build ./...` / `go vet ./...` 退 0；带 DSN `go test -p 1 -count=1 ./...` **105 ok / 0 FAIL / 15 无测试 / 0 cached**（19:10:22→19:12:29，127 s）；探针 `platform/migrate` -v 五例 PASS、TF postgres `-run TestThreeGenerationsOfOnePickupChain|TestCarrierPickupsRefuseToRunOutsideATransaction -v` PASS 2 / SKIP 0（后者在 lc/29 预演那跑上量，同一代码树）。lc/26 进 main（`1c48e4fe`）后 `rebase --onto 1c48e4fe 83ab2032` 零冲突，代码树对 `58480cf2` 逐字节同（只多 .md 祖先）：`0b89f99e→51d5f6d9`、`5b6ce319→887a5f2e`、`14608699→9286562e`、`55bfdf15→0c76fc66`、`8795a1b6→4ae53900`、`ce8afd68→788c6473`、`1d9a24c0→d79b65a6`、`579b89ab→0e5ef0ce`、清点 `037d88b8`；本票全部文件（清点与 spec 除外）`git diff 579b89ab 037d88b8 -- <files>` 只差 `partition_subject_registry_test.go` 里 lc/26 那一行。分支 `mcp6-lc31@579b89ab` 作封存出处、改名 `merged/`。原派单 task-f3f0d59a 由推送方代结 done（作者会话 18:43 推出 resolved 笔后重置、未报）。

## 完成记录（2026-09-10，通道 6，分支 `mcp6-lc31`）

八笔，全部在隔离 worktree 里写完、每笔推 origin，共享树上没出现过在途 `.go`：

- `0b89f99e` 认领（Status → in-progress）。
- `5b6ce319` **领域**：`CarrierFirstEffectivePickup` 聚合（三道构造门 / 三个转换门 / 重建门）、`CarrierPickupBasis`、结果与原因封闭词表；`ParticipationEntryKind` 第三格 `CARRIER_FIRST_EFFECTIVE_PICKUP`、`EstablishSegmentWithCarrierPickup` / `JoinWithCarrierPickup` / `RederiveParticipationWithCarrierPickup`、段重建门认第三格并许其失效。
- `14608699` **端口 + 应用**：`CarrierFirstEffectivePickupRegistry` / `CarrierPickupIdentityFactory` / `CarrierFirstEffectivePickupHandoff`；`JudgeCarrierFirstEffectivePickupHandler.Judge` 十一格结果代数（见下）。
- `55bfdf15` **postgres**：迁移 TF `0020`（版本表 + 依据表 + 两序列 + 参与表两条 CHECK 放开第三格）、`CarrierFirstEffectivePickups`、`OutboxCarrierFirstEffectivePickupHandoff`、`ResultVersions` 两个签发口。
- `8795a1b6` **http**：`POST /transport-fulfillment-carrier-first-effective-pickup-judgments`、`GET /transport-fulfillment-carrier-first-effective-pickups?object=`、`UnconfiguredIntake` 同堵一口。
- `ce8afd68` **cmd/parcel-api**：`buildCarrierPickupJudgment` 全缝接真并包事务（`NewCarrierIdentityDirectory` 首次接上生产：PC 参与方册 / 法人册一只 `PartyIdentityRegistrations` 担两口）、路由两行、隔离读放行表一行、unwired 占位两只；架构门禁：分区主体登记一行、TF postgres `Save` 无事务负向用例。
- `1d9a24c0` **领域补漏**：待确认版本携带承运主体名称素材（`Material`）、`HoldPending` 亦可从失效链尾长出——这些改动写于 `5b6ce319` 之后、被随后三笔依赖却漏提，本笔补入；**分支中间几笔单独检出不可编译，tip 可**，推送方重放时若逐笔编译请把 `1d9a24c0` 与 `5b6ce319` 合看。
- `27bb2dc1` 机制清点在 `1d9a24c0` 的干净 detached 检出上重生成。

**完成判据逐条**：

1. 领域：构造门「已形成缺承运主体 / 缺业务时间 / 缺判断时间 / 缺依据 / 缺租户 / 缺对象 / 缺身份 / 缺版本 / 依据重复同一代」各拒 ✓、「待确认缺原因 / 缺素材 / 缺判断时间」各拒 ✓、沿用原版本号拒 ✓；失效只从已形成长出（`ErrCarrierPickupNotFormed`）✓、已形成不回待确认（`ErrCarrierPickupAlreadyFormed`）✓；链尾失效后再次形成回指失效版本 ✓；第三格 `String()` 与解析往返、第四格仍答空串 ✓。**一处与票面措辞不同**：「被更正的必须恰是当前依据」不在收寄聚合的 `Supersede` / `Void` 门上（聚合不知道更正关系），而在编排（`BasedOn` → `BASIS_NOT_CURRENT`）与段的重派生门（`currentParticipationEnteredBy` → `ErrNoParticipationToRederive`）两处守——各有用例。
2. 应用：结果代数**十一格**（票面写「至少六格」）：`PICKUP_FORMED` / `PICKUP_SUPERSEDED` / `PICKUP_VOIDED` / `PICKUP_PENDING` / `NOT_A_PICKUP` / `NOT_FIRST` / `BASIS_UNAVAILABLE` / `BASIS_NOT_CURRENT` / `ALREADY_RECORDED` / `INPUT_NOT_ACCEPTED` / `PICKUP_UNDECIDED`（五个未决理由），逐格有用例；「对象已凭场外揽收进段 → 非首次不落版本」✓；「轨迹事实有效时间待判断 → 依据不可用」「不存在的一代 / 别的对象 → 未受理」✓；「名称素材不在册 → 待确认（未登记）→ 登记后同依据 → 已形成回指前版、此时才进段才发意图」✓；「两条依据指向不同承运主体 → 待确认（来源冲突）全部依据保留、同主体第二条仍是未登记」✓；已形成后 `enterFulfillmentSegment` 铸出第三格参与且实际承运商判断当前版为已识别、依据即收寄依据 ✓；段引用缺席时事实照登、意图照发、不进段 ✓；重派生：替代版本 → 替代参与版本（起点随新业务时间）、失效版本 → 失效参与版本（段内无有效参与）、更正的不是链尾依据 → `BASIS_NOT_CURRENT` 不落版本 ✓；已形成后另一来源 → 非首次不收回 ✓；登记册 / 身份读口不可用 → 未决带续办引用 ✓。
3. postgres（真库 `-v` PASS 非 SKIP）：三代同链往返、`FindByKey` 交指名那一代而不是链尾、`FindCurrentByObject` 链尾、`ListByObject` 整链、另一租户不可见 ✓；同版本重放 / 同对象第二条首登 / 同一前版回指两次各答已登记且撞键后同事务仍能读回链尾 ✓；CHECK 拦「待确认却带承运主体」「不回指的失效」「原因用了无合格证据」✓；参与表接受第三格与其失效版本、第四个词拒 ✓；outbox 三代各入一份 ID 异分区同、载荷四维指名自己那一代、事件类型 ✓、待确认拒绝入队 ✓、无事务拒 ✓；签发走事务、两前缀 ✓。
4. 在线口：POST 新落收寄版本 201、待确认 / 不构成 200 且 outcome 区分、严格解码（自报租户 / 来源词集外 / 时刻解不出 / 分支词集外 400）、GET 用 POST 405、未配置 Intake 403 ✓；GET 按对象交回整链、对象缺席 400 ✓；隔离读放行按 ADR-0078 判入格（`isolated_read_test.go` 表加一行）✓。
5. `gofmt -l cmd internal migrations` 空、`go build ./...` / `go vet ./...` 退 0；**带 DSN** `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/... ./internal/architecture/... ./migrations/... ./internal/platform/migrate/...` 22 包 ok；TF postgres + cmd/parcel-api + platform/migrate 三包 `-v` 下 **PASS 391 / SKIP 0 / FAIL 0**；机制清点在 tip 的干净检出重生成 ✓。未跑全量（按派单）。
6. 本记录即「刻意不含」核对：收寄判读规则目录与规则那一路的节拍（随第一家真源，ADR-0135 决定八）、PS 半边（lc/25，本票 resolved 后其 Blocked by 解除——已在 lc/25 票面回填）、有效交付在轨迹事实之上的判断（另一题）、管理台页面（先有事实再谈面）——四件都没做，各有归处。

**判断题（交非作者评审）**：

1. **待确认版本携带名称素材（`claimed_material` 列）**——ADR-0135 决定二只列五件，我按 CONTEXT「不据名称铸身份，名称作为素材随依据保留」把素材放在待确认版本上（已形成 / 失效不带，CHECK 同形），并用它分「同主体第二条证据（仍是身份未登记）」与「不同主体（来源冲突）」：素材大小写不敏感相等即同主体。这是对 ADR 的一处加法，不是改法；若评审认为素材该落依据表逐条而不是版本一列，改的是一列的位置。
2. **`HoldPending` 亦可从失效链尾长出**——ADR-0135 生命周期只写「再次形成从失效版本长出新版本」（已形成）；失效之后新到证据身份未登记时，我让链上长一版待确认而不是答未受理（后者会让「等身份登记」变成「改输入」，ADR-0029 分格不对）。
3. **重派生的触发是显式判断，不是轨迹更正落库后的自动一拍**——票面「做法」3 写了「触发点挂在轨迹事实更正落库之后的一拍」，但更正后「仍表达 / 不再表达收寄」本身是一次读法（决定四），规则那一路未立时没有人能自动给；所以更正走的是：判断方对更正后那一代再次 `POST`（`expressesControl` 为真 → 替代、为假 → 失效），编排从读回的回指取「更正了哪一代」（命令也可显式指名）。ADR-0135 决定八「显式判断先行、规则那一路另立」与此一致；规则那一路落地时挂自动一拍到同一个 `Judge`。
4. **「首次」的判据**——对象有当前有效参与（`FindActiveSegments` 非空）即非首次；已离场的历史参与不算（对象一程走几段，后一段的承运证据仍可能是它的首次有效收寄）。若评审认为「曾进过任何段」即非首次，改 `objectUnderControl` 用 `FindSegmentsForObject`。
5. **进段 vs 重派生的分路**——新版本回指的前版**已形成**才走重派生（`priorEnteredSegment` 按键读回前版核结果），前版是待确认 / 失效时新的已形成版本走进段（这条链第一次成为收寄）。前版读不回按「没进过」办，进段那道门会拒对象已在段内。
6. **业务发生时间取依据有效时间**——ADR-0135 越权风险点 3 原样落地；非轨迹事实来源（扫描 / 凭证 / 交接）本上下文没有登记册可读，由判断方连同证据交来 `occurredAt`，缺席拒。
7. **`NewCarrierIdentityDirectory` 首次接上生产**——此前 TF 对 PC 身份的消费侧适配器没有任何 cmd 调用点；本票把它接进收寄判断与随之的实际承运商判断（`FormActualCarrierJudgmentHandler` 作为收寄依据交给判断的口，也是首次进 cmd）。两只都以 PC 的 `PartyIdentityRegistrations` 作参与方册与法人册两口的实现。
8. **HTTP 状态码**——只有已形成 / 替代 / 失效答 201；待确认答 200（即便新落了一版待确认），因为 201 在这里说的是「新落一版**收寄**」，待确认不构成收寄。若评审认为「新落一版（任何结果）」才是 201 的判据，改 `writeCarrierPickupJudgmentOutcome` 一处。
