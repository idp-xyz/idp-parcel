# 事实源可填性勘察（FACT-SOURCE-FILLABILITY-SCAN）

取证基准：`7d7e138`（detached worktree，非共享树）。切片 PN-06。只读勘察，未改任何 `.go` / `.sql`。

## 结论先说

**还剩四条两条判据都勾、今天就能接。**「一条都不剩」不成立。

按「离能接最近」排序：

1. `parcel-shipment.final-outcome.formed` — 信封载荷就是 `FinalAdoptionKey`（含 `parcel`），`FinalOutcomeStore.FindByKey` 有真 postgres 实现，`ParcelFinalOutcome` 锚在单个 `DeclaredParcelID` 上。端口注释自己点名下游是「追踪、异常、结算」——**追踪就是 VE**。
2. `network-routing.initial-route.formed` — 载荷带全套 `InitialRouteJudgmentKey`（含 `parcel`），`InitialRouteStore.FindByKey` 有真实现，键本身含 `DeclaredParcelID`。
3. `parcel-shipment.parcel-cancellation.recorded` — 载荷是 `CancellationRequestKey`（含 `parcel`），`ParcelCancellationStore.FindByKey` 有真实现，单包裹。
4. `network-routing.reachability-judgment.formed` — 载荷只带（租户+关联），但按 `FindByCorrelation` 重读回的 `ReachabilityJudgmentRecord.Key` 含 `DeclaredParcelID`。两条判据机制上都过；**语义上需裁定**，见下。

头两条还有一条实际优势：`OutboxInitialRouteHandoff` 与 `OutboxFinalOutcomeHandoff` **已经装在 `wireDispatcher` 里**（分别在 `acceptanceConsumer` 与 `adoptEffectiveDeliveryConsumer` 的 `downstream` 上），也就是说这两类信封今天已经在生产入队、并正撞 `dispatch.no_subscriber`。接它们不需要新建任何发布侧，也不需要动载荷。

四条都不需要往信封里塞快照——包裹引用要么已在载荷键里，要么在按键重读回的本体上。

## 复核 MCP-1 的三个样本

**正例 `node-operations.node-intake.formed`：属实。** `ReceptionStore.FindByKey` 在 `noports`，`nopostgres.NewReceptions` 是真实现；`record.Intake.Association()` 交回 `(ParcelAssociationReference, bool)`，`venodeops` 的 `projectionCommand` 正是拿 `association.String()` 喂 `vedomain.NewTrackedParcelReference`。两条都勾。

**反例 B `node-operations.execution-fact.recorded`：完全属实，字段清单一字不差。** `NodeExecutionFact` 是 tenantID / node / item / unit(`HandlingUnitID`) / action / evidence / performedAt，没有包裹关联。端口注释确实写明消费方是 customs-compliance。

**反例 A `node-operations.sealed-snapshot.recorded`：结论对，但判据①的理由说错了。**

MCP-1 写的是「没有任何 SealedSnapshotStore / FindByKey → 判据①就不过」。「没有 `SealedSnapshotStore`」字面属实，但**判据①实际是过的**：`ConsolidationStore.FindByID(ctx, tenant, id)` 存在，`ConsolidationUnit.Snapshots()` 交回全部历史 `SealedSnapshot`，而 `OutboxSealedSnapshotHandoff` 的载荷恰好是 `tenantId` + `unitId` + `seal` 三件——按租户加单元取回聚合、再按封签在 `Snapshots()` 里挑出那一份，路径是通的。

差别不在这一行的结论（判据②照样不过，范围是 `ConsolidationUnitID`、成员是 `[]HandlingUnitID`，都不是包裹），而在**判据①该怎么问**。按「有没有一个以事件命名的 store」去问会系统性地产生假阴性：本仓多数快照/状态推进类事实的重读口开在**拥有它的聚合仓储**上，不是逐事件各开一个。本次扫描一律按「有没有任何仓储能按信封里的键取回本体」来判，`sealed-snapshot` 与 `regulatory-restriction`、`manifest`、`case-closure` 几条都受这条改写影响。

另有一条附带订正：`customscompliance/ports` 的包注释写「适配器仍阻断在 Bento 持久化闸门之后，今天唯一的实现是测试替身」，**这句已经过时**——九类 CC 事实的 store 都有真 postgres 实现（`CustomsCases`、`ExternalResults`、`DeclarationSubmissions`、`DispositionVerifications`、`Restrictions`、`GateVerifications`、`FollowUps`、`Manifests`、`CaseClosures`）。CC 那一栏的判据①因此全过，卡点全在判据②。

## 一条贯穿全表的事实

三个已接上的 VE 投影**都把 `CarriedObjectReference` 原样当包裹引用**：

- `derive_on_transport_handover.go` → `record.Handover.Object().String()`
- `derive_on_offsite_pickup.go` → `record.Pickup.Object().String()`（注释明写「集运单元不猜、不跳过：Object 原样当包裹引用」）
- `derive_on_effective_delivery.go` → `record.Delivery.Object().String()`

而 `CarriedObjectReference` 的领域注释是「对正式包裹身份**或集运单元**的引用」。也就是说生产里判据②的现行标准就是「拿得出 `CarriedObjectReference` 即算数」，集运单元混入被明确接受了。下表凡 TF 一栏判②记「能」的，都是按这条现行标准判的，不是我另立的宽标准。

反过来，`HandlingUnitID` **不**满足判据②，且没有补救路径：`ParcelIdentityView.ResolveParcelIdentity` 的入参是 `ExternalMarkObservation`（外部条码串），不是 `HandlingUnitID`；`ReceptionKey` 是（租户+来源身份），也没有按实物反查收寄的索引。NO 侧三条反例卡的都是这一格。

## 主表

判据①＝提供方有没有能按信封里的键取回本体的仓储。判据②＝取回的记录能不能给出 `vedomain.NewTrackedParcelReference` 要的包裹引用。

### node-operations

| 事件类型 | 判据① | 判据② | 结论 |
|---|---|---|---|
| `collaboration-acceptance.decided` | `CollaborationAcceptanceStore.FindByKey` | 不能。`CollaborationAcceptance` 的范围是 `CollaborationItemReference` + `[]HandlingUnitID`，缺包裹维 | 不可接 |
| `sealed-snapshot.recorded` | `ConsolidationStore.FindByID` + `ConsolidationUnit.Snapshots()`（**订正 MCP-1**） | 不能。范围是 `ConsolidationUnitID`，成员是 `[]HandlingUnitID`，缺包裹维 | 不可接 |
| `execution-fact.recorded` | `ExecutionFactStore.FindByKey` | 不能。`NodeExecutionFact` 无包裹关联，最细到 `HandlingUnitID` | 不可接 |

### transport-fulfillment

| 事件类型 | 判据① | 判据② | 结论 |
|---|---|---|---|
| `offsite-pickup.formed` | `PickupAttemptStore.FindByKey` | 能，但一封信 N 个：`PickupAttemptRecord.Pickups` 是 `[]OffsitePickup`，各自带 `Object()` | 不可接（**非填不出**：对象级 `.registered` 已覆盖同批事实且已接上，两条都登记会把同一份揽收派生两次——`veinbox` 与 `psinbox` 两处注释都写明了这条） |
| `exception-journey.recorded` | `AlternateJourneyStore.FindByKey`（载荷带全套 `AlternateJourneyKey`） | 能，但一封信 N 个：`AlternateJourney.Members()` 是 `[]CarriedObjectReference` | 需裁定 |
| `disposition-execution.recorded` | 同上（同 store、同 record，仅信封 ID 的类型段不同） | 同上 | 需裁定 |
| `regulatory-acceptance.recorded` | `DispositionAcceptanceStore.FindByKey` | 能，但一封信 N 个：`RegulatoryTransportDisposition.accepted` 是 `[]CarriedObjectReference`；且**拒接格该清单为空**，那一格给不出任何包裹 | 需裁定 |
| `transport-commission.submitted` | `TransportCommissionStore.FindByKey` | 能，但一封信 N 个：`TransportCommission.members` 是 `[]CarriedObjectReference` | 需裁定（另有语义问题：委托提交不等于取得控制，未必是 VE 该记的里程碑） |
| `capacity-consumption.recorded` | `CapacityPoolStore.FindByKey` | **不能**。`CapacityPool` 是 pool / schedule / unit / capacity / reservations，整条链上没有任何载运对象维 | 不可接 |

TF 中间四条是同一个形状：填得出，但**一封信带一批对象**，与本仓「一封信一对象」的既有纪律冲突。它们与 `offsite-pickup.formed` 的关键差别是——那一条有对象级替代品可接，这四条**没有**，不接就等于这些事实永远到不了 VE。所以是「需裁定」而不是「不可接」：要裁的是「VE 投影允不允许一封信派生 N 条」，不是「填不填得出」。

### network-routing

| 事件类型 | 判据① | 判据② | 结论 |
|---|---|---|---|
| `initial-route.formed` | `InitialRouteStore.FindByKey`（`nrpostgres.NewInitialRoutes` 真实现；载荷含 tenantId/shipment/**parcel**/baseline/purpose/correlation） | **能**。`InitialRouteJudgmentKey.DeclaredParcelID`，单包裹 | **可接** |
| `reachability-judgment.formed` | `ReachabilityJudgmentStore.FindByCorrelation`（`nrpostgres.NewReachabilityJudgments` 真实现；载荷带租户+关联） | **能**。载荷不带包裹，但重读回的 `ReachabilityJudgmentRecord.Key.DeclaredParcelID` 有，单包裹——正合「信封只带键、本体按键重取」 | 需裁定（机制两勾全过；**语义待判**：键含 `SubmissionVersionID`，这是受理前的可达性三值判断，UC-VE-002 写明「只消费已接受事实」，它算不算已接受源事实要人裁） |

### parcel-shipment

| 事件类型 | 判据① | 判据② | 结论 |
|---|---|---|---|
| `source-data-version.formed` | `SourceDataVersionRecords.FindVersion`（`pspostgres.NewSourceDataVersions` 真实现；载荷四件凑齐 `SourceIdentity` 加版本号） | **有时能**。`SourceDataScope` 有两个构造器：`NewParcelScopedSourceData` 带包裹，`NewShipmentScopedSourceData` 不带（寄件人一类资料作用于整份委托）；postgres 文档形状里 `parcelId` 是 `omitempty` | 需裁定（委托级那半边结构上给不出包裹，不能靠散成成员份数补——领域注释明确反对） |
| `parcel-cancellation.recorded` | `ParcelCancellationStore.FindByKey`（`pspostgres.NewParcelCancellations` 真实现；载荷 tenantId/requestKey/**parcel**） | **能**。`CancellationRequestKey.Parcel` 与 `ParcelCancellation.parcel` 都是 `DeclaredParcelID`，单包裹 | **可接** |
| `final-outcome.formed` | `FinalOutcomeStore.FindByKey`（`pspostgres.NewFinalOutcomes` 真实现；载荷 tenantId/**parcel**/kind/version 即整个 `FinalAdoptionKey`） | **能**。`FinalAdoptionKey.Parcel` 与 `ParcelFinalOutcome.parcel` 都是 `DeclaredParcelID`，单包裹 | **可接** |

### customs-compliance

九类的判据①**全过**（store 与真 postgres 实现都在，见上文订正）。因此这一栏只看判据②。CC 的通用范围类型是 `DecisionScopeReference`——领域注释是「指名决定明确覆盖的对象或范围」，**并明说「不因合报关系自动扩大」**，所以它既不保证是包裹，也不许展开成成员包裹。

| 事件类型 | 判据① | 判据② | 结论 |
|---|---|---|---|
| `external-result.received` | `ExternalResultStore.FindByKey` | 不能。`ExternalResult.scope` 是 `DecisionScopeReference`，不保证是包裹 | 不可接 |
| `declaration-submission.formed` | `DeclarationSubmissionStore.FindByKey` | 能，但一封信 N 个：`CustomsSubmissionVersion.Members()` 是 `[]DeclaredParcelReference` | 需裁定 |
| `regulatory-restriction.changed` | `RestrictionStore.FindByID` | 不能。`RegulatoryRestriction.scope` 是 `DecisionScopeReference` | 不可接 |
| `manifest.recorded` | `ManifestStore.FindByManifest` | 不能。`ExternalManifestReference` 只有 `scope`（`DecisionScopeReference`）与 `association`（`DeclarationUnitID`）；要到包裹得再跳一层申报单元 | 不可接 |
| `follow-up.recorded` | `FollowUpStore.FindTarget` | 不能。`FollowUpTarget` 是 caseRef / unit / version / scope，无直接包裹维 | 不可接 |
| `gate-verification.recorded` | `GateVerificationStore.FindByKey` | 不能。`ReleaseGateVerification.scope` 是 `DecisionScopeReference` | 不可接 |
| `disposition-verification.recorded` | `DispositionVerificationStore.FindByKey` | 不能。`DispositionVerification` 挂 `RegulatoryDecision`，其范围仍是 `DecisionScopeReference` | 不可接 |
| `customs-case.established` | `CustomsCaseStore.FindByKey` | 能，但一封信 N 个：`CustomsCase.Parcels()` 是 `[]CaseParcelAssociation`，每项带 `Parcel` 与 `Customer` | 需裁定（**CC 里唯一直接拿得出包裹身份的**，还自带客户归属，跨客户隔离那条正好有料） |
| `case-closure.recorded` | `CaseClosureStore.FindByCase` | 不能，**且连间接路都没有**：`CustomsCaseClosure` 只有 `caseRef`，而 `CustomsCaseStore` 的键是（租户+辖区+方向+程序+义务范围）四维监管范围，**没有按 caseRef 反查案件的读口**——拿着 caseRef 走不到 `Parcels()` | 不可接 |

## 覆盖声明

**逐条读过代码取证的（打开文件、读到字段或方法本身）：**

- 路由表已登记的六类：`cmd/parcel-dispatch/assemble.go` 的 `wireDispatcher` 整个读完，含 `NewDirectPublisher` 的 map 与全部 `deriveXxxConsumer`。
- 判据②的靶子：`vedomain.NewTrackedParcelReference` 本体，以及四个消费适配器里实际喂它的那一行（`venodeops` 的 `projectionCommand`、`vetf` 的 `pickupProjectionCommand` / `handoverProjectionCommand` / `projectionCommand`）。
- node-operations 全部三条：`ports/ports.go` 整个读完；`domain/customs_collaboration.go`、`domain/consolidation_unit.go` 整个读完；`domain/node_intake.go` 的 `Association()` 与 `HandlingUnitID` 段；`adapters/postgres/sealed_snapshot_handoff.go` 整个读完（含载荷形状）。
- transport-fulfillment 全部六条：`ports/ports.go` 整个读完；`domain` 侧的 `AlternateJourney`、`RegulatoryTransportDisposition`、`TransportCommission`、`OffsitePickup`、`CapacityPool`、`CarriedObjectReference` 结构体逐个读到字段；`exception_journey_handoff.go` 与 `capacity_consumption_handoff.go` 整个读完。
- network-routing 两条：`ports/ports.go` 的可达性与初始路由两段；`domain` 的 `ReachabilityJudgmentKey` 与 `InitialRouteJudgmentKey` 读到字段；`initial_route_handoff.go` 整个读完，`reachability_handoff.go` 读了载荷与信封段。
- parcel-shipment 三条：`ports/ports.go` 的取消、终局、资料版本三段；`domain` 的 `ParcelCancellation`、`ParcelFinalOutcome`、`SourceDataScope`（两个构造器）读到字段；`final_outcome_handoff.go` 整个读完，`parcel_cancellation_handoff.go` 读了载荷与信封段。
- customs-compliance：`ports/ports.go` 整个读完（九类的 store 与 key 全在里面）；`domain` 侧 `CustomsCase` / `CaseParcelAssociation`、`DeclarationUnit` / `CustomsSubmissionVersion`、`RegulatoryRestriction`、`ExternalManifestReference`、`FollowUpTarget`、`ReleaseGateVerification`、`ExternalResult`、`DispositionVerification`、`CustomsCaseClosure`、`DecisionScopeReference` 逐个读到字段。
- 事件类型清单：全仓 grep 事件类型常量声明，`internal/**` 非测试文件全覆盖。
- postgres 实现在不在：全仓 grep `var _ ports.X =` 断言，外加对 CC 九个 store 结构体逐个 grep 确认。

**按同类归并推断、没有逐条打开的（推断不是取证）：**

- CC 九条的**信封载荷形状**没有逐个打开看。我确认了 store 与 key 的存在，也确认了 postgres 实现在，但没有逐个读 `*_handoff.go` 验证「载荷里确实带齐了 FindByKey 所需的每一维」。判据①因此对 CC 是**推断为过**，不是取证为过。TF/NR/PS 的载荷我是逐个读过的。CC 那九条真要开票前，这一步得补。
- `disposition-execution.recorded` 的载荷我没单独打开，是按它与 `exception-journey.recorded` 共用 `AlternateJourneyKey`、共用 `alternateJourneyPayload`、仅信封 ID 类型段不同（`exceptionJourneyEventID` 里的注释明写「把本口与同键的 DispositionExecutionHandoff 错开」）推断的。
- `nopostgres.Receptions`、`nrpostgres.InitialRoutes`、`nrpostgres.ReachabilityJudgments`、`pspostgres.FinalOutcomes`、`pspostgres.ParcelCancellations`、`pspostgres.SourceDataVersions` 这几个没有 `var _ ports.X =` 断言，我是按构造函数存在（且前三个在 `assemble.go` 里被真正装上）判定为真实现的，没有逐个核对方法签名与端口逐字对齐。
- **完全没有扫**：settlement-accounting（11 类）、parcel-pricing（1 类）、pilot-governance（3 类）、以及 visibility-exception 自己发布的 8 类。票面没要求，我也没看。其中 settlement-accounting 有若干类可能带包裹维，若日后要把「费用/理赔」也做成追踪维度，那一批需要单独一轮——本报告对它们**零覆盖**，不要拿本报告的沉默当「已确认不可接」。
- 判据之外的东西一律没判：映射目录（`MilestoneMappings`）里有没有对应 `SourceFactKind` 的行、这些事实该映成哪个标准里程碑、以及接上之后 inbox 消费者名怎么取，都不在两条判据内，本报告不表态。

## 不建议的两件事（票面红线，此处只记不展开）

- 没有为了让任何一条看起来可接而建议往信封载荷里塞本体快照。四条「可接」的包裹引用要么本来就在载荷的键里，要么在按键重读回的记录上。
- 没有建议登记 `visibility-exception.tracking-projection.derived`。
