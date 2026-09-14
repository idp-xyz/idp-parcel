# `transport-fulfillment` 运输收费发生项的成员对象没有只读视图：`ChargeOccurrenceRegistry.FindByKey` 是带 `Save` 的写侧登记册口，成员 `CarriedObjectReference` 字面分不出正式包裹身份还是集运单元，`parcel-pricing` 造快照的「包裹主体」一格指不到

Category: enhancement
Status: ready-for-agent——**2026-09-14 22:1x 通道 1 按用户「你是业务和系统专家，自决」代裁（TF owner 口径），三条「要裁的」写入下方「裁决」节**：候选 1 窄只读口、成员**裸引用原样交**（不带种类、不按前缀猜）；**不展开**集运单元；键按 `ChargeOccurrenceKey` **精确到有效性版本**。此前 draft——2026-09-14 21:1x 通道 3 立票（sa-cc/11 裁决 4 量「包裹主体指不到」的提供方半边；task-620bc8e7，通道 1 派单）。只写票面未动代码；取证锚 main `db480695`
Blocked by: 无（sa-cc/11 已进 main 2026-09-14 20:4x：`PricingInputResolver` 与 `EligibleSourceReferences.Occurrence` / `OccurrenceVersion` 钥匙已在，本票是它点名的第一只读口）；**要裁的三条归 TF owner，裁前不动代码**

## 缺口（取证于 `db480695`，逐符号名）

- `git grep -n -A3 'type ChargeOccurrenceRegistry interface' -- internal/transportfulfillment/ports/charge_occurrence.go` → `FindByKey(ctx, ChargeOccurrenceKey) (ChargeOccurrenceRecord, bool, error)` + `Save(ctx, ChargeOccurrenceRecord) (ChargeOccurrenceSaveOutcome, error)`，头注「只有首登与读回，没有 Update」。它是写侧登记册；PP 依赖它等于声明自己可能写发生项。TF 自己在同文件 `FailedAttemptSource` 头注里写过为什么不这么干：「单开一个只读口而不直接依赖 `PickupAttemptStore`：那个口带 `Save`，而本用例只读——依赖它等于声明自己可能写揽收尝试……契约窄一格，说的话就准一格」。
- `git grep -n -E 'type \w+(View|Read|Source|Reader) interface' -- internal/transportfulfillment/ports/` → 9 只只读口（`FailedAttemptSource` / `DeliveryPlaceSource` / `DeliveryWindowSource` / `DeliveryConditionSource` / `DeliveryAttemptView` / `HandoverScopeView` / `ReviewCatalogueRead` / `ExternalTrackingFactReviewRead` / `TrackingSource`），名字含 `Occurrence` 的 0 只（两个数都钉 `db480695`——数在这里是论点：TF 有开窄只读口的习惯，只是没给发生项开）。
- `git grep -n -B2 'type CarriedObjectReference struct' -- internal/transportfulfillment/domain/offsite_pickup.go` → `struct{ requiredValue }`，头注「指名一个载运对象。它是对正式包裹身份或集运单元的引用——两者分别属 parcel-shipment 与 node-operations，本上下文不铸造它们」。类型上没有「种类」一格；`git grep -n -E 'Members\s+\[\]CarriedObjectReference' -- internal/transportfulfillment/domain/transport_charge_occurrence.go` → 发生项成员就是这个裸引用的切片，构造门只拒空与重复。
- `git grep -n 'object_ref' -- migrations/transport_fulfillment/0007_transport_charge_occurrence.sql` → `transport_charge_occurrence_member.object_ref text NOT NULL`，主键 `(tenant_id, occurrence_ref, validity_version, object_ref)`；无种类列。库上也分不出。
- `git grep -n -l 'ChargeOccurrence' -- internal/ cmd/ | grep -v '^internal/transportfulfillment/'` → 只有 SA（`internal/settlementaccounting/**`，自铸 `ChargeOccurrenceID` 作供应商预期成本与评价请求的回指）与 PP（`EligibleSourceReferences.Occurrence` 字面串）；两侧都只持引用，今天没有任何上下文读过发生项的成员。
- PP 侧要什么：`git grep -n -E 'Subject(AcceptedPackage|Estimate|Shipment|MasterDocument)\s+EvaluationSubjectKind' -- internal/parcelpricing/domain/input.go` → 评价对象四种（已受理包裹 / 试算 / 委托 / 承运总单，ADR-0111）；`NewShipmentSubject` / `NewMasterDocumentSubject` 头注写身份分属 PS / TF；**没有集运单元一种**。`git grep -n -A4 'var missingInputReadPorts' -- internal/parcelpricing/application/form_evaluation_from_request.go` → 第一条点名 `transport-fulfillment: no read-only view of the charge occurrence's member carried objects (evaluation subject)`。

## 语言从哪里来

- TF `CONTEXT.md`「载运对象」：「可以被独立分配、交接并参与运输履约的明确实物范围，例如包裹或集运单元。载运对象引用其来源上下文拥有的身份；成为载运对象不会改变包裹身份、集运成员关系或客户责任范围」；Boundaries 首条「`transport-fulfillment` 拥有载运对象引用」。
- TF `CONTEXT.md`「运输收费发生项」：「发生项固定采购责任法人、服务提供方、采用协议、原因、主要业务范围、成员、数量与单位、业务时间、有效性和来源事实」——成员是发生项固定下来的东西，读它读的是 TF 自己的事实，不越界。
- GLOSSARY「载运对象」：「`transport-fulfillment` 引用载运对象身份；包裹身份仍由 `parcel-shipment` 拥有，集运单元及成员关系仍由 `node-operations` 拥有」。
- `docs/domain/CONTEXT-MAP.md` `settlement-accounting → parcel-pricing` 那条边：「那些读口归拥有事实的上下文（运输履约的发生项成员……）那一侧立」。

## 做法候选（两条以内，不选）

1. **窄只读口，成员原样交**：`internal/transportfulfillment/ports/` 新立一只只读接口（形照 `FailedAttemptSource`，不是 `ChargeOccurrenceRegistry` 的再导出），按 `ChargeOccurrenceKey`（租户 + 发生项 + 有效性版本）答成员载运对象清单 + 业务时点 + 主要业务范围；`adapters/postgres.ChargeOccurrences` 天然满足它，不需要第二个实现。成员的「种类」不在 TF 答——持引用方自己去 PS / NO 问「你认不认这个身份」。
2. **成员对象带种类**：TF 领域在 `CarriedObjectReference` 之外立一个「成员对象」值（引用 + 种类，种类封闭为正式包裹身份 / 集运单元），登记发生项时由登记方声明种类，`0007` 成员表加种类列（新迁移，`0007` 不改）；只读口交这个值。改的是写侧的形，代价在登记入口（要多交一维），换来的是 PP 消费侧不必两边试问。

## 红线

- 消费方 PP 不 import `internal/transportfulfillment/application`；成员对象 → `EvaluationSubject` 的翻译只在 PP 侧消费适配器（ADR-0025），不在本票。
- TF 只开只读口：不派生评价对象、不替 PS / NO 铸身份、不替 NO 展开集运成员关系（除非「要裁的」2 裁给 TF 且经消费侧适配器读 NO 的封装成员快照）。
- 只读口不是 `ChargeOccurrenceRegistry` 的再导出：方法集不含 `Save`，不把 `ChargeOccurrenceRecord` 整条交出去。
- 未确认参数不写死：成员对象种类的封闭集随裁决 1 定，不预拟第三种；不按引用前缀猜种类（TF 不拥有那两个标识空间）。
- 真实发生项属实例半边；夹具全 `SYN-`。

## 完成判据（待裁后写实；可 grep）

1. `git grep -n -E 'type \w+ interface' -- internal/transportfulfillment/ports/` 多出一只名字含 `Occurrence` 的只读接口；`git grep -n 'Save' -- <新口文件>` 零命中。
2. 真库适配器 `ChargeOccurrences` 满足新口（编译期断言或真库用例）；按键取到成员对象清单与业务时点；键不存在答 found=false，不造默认成员。
3. 若裁决 1 取候选 2：新迁移在 `migrations/transport_fulfillment/`（`0007` 不改），成员表种类列 + 封闭集 CHECK，读回带种类，既有行的种类怎么补（回填 / 标未声明）在迁移头注写明。
4. `docs/product/MECHANISM-INVENTORY.md` 在干净检出重生成：TF 端口声明 +1（候选 2 再 +1 迁移）。
5. `internal/architecture` 门禁绿；`internal/parcelpricing/**` 零 diff。

## 地盘

`internal/transportfulfillment/ports/`（新文件）、`internal/transportfulfillment/adapters/postgres/charge_occurrence_registry.go`（若要新方法）；裁决 1 取候选 2 时另加 `internal/transportfulfillment/domain/`、`migrations/transport_fulfillment/`（新序号）与发生项登记入口。不动 `internal/parcelpricing/**`、`internal/nodeoperations/**`、`internal/parcelshipment/**`、`internal/settlementaccounting/**`。

## 要裁的

1. **成员对象的形：引用带种类，还是裸引用**——归 TF owner。TF 今天存的是裸引用（`object_ref text`），「正式包裹身份还是集运单元」在登记里没有声明过。三条路：让登记方声明种类（写侧改形，候选 2）；只读口按引用形状 / 前缀推断（TF 不铸这两种身份，推断等于替 PS / NO 解释它们的标识空间，红线已拒）；只交裸引用、由持引用方自己去 PS / NO 两边问（候选 1，PP 消费侧多两次试问）。这是 TF 领域语言题——「载运对象」词条要不要长出「种类」一格——TF owner 裁；裁前 `CarriedObjectReference` 一字不动。
2. **成员是集运单元时展开到成员包裹归谁**——归 TF owner，NO owner 复核。PP 的评价对象里没有集运单元一种（ADR-0111 四种），成员是集运单元时要么按成员包裹逐件 / 按票评价，要么等 PP 另立评价对象种类（那半归 PP owner，见 spec「不在本目录」）。展开要读 NO 的「封装成员快照」（NO `CONTEXT.md`：「集运单元封装时冻结的成员关系版本」），TF 只引用集运单元身份、不拥有成员关系。本票的只读口是**不展开**、只交集运单元引用（展开归 PP 消费侧再去问 NO），还是 TF 经消费侧适配器读 NO 后代展开（TF 多一条 TF→NO 消费边，且展开用哪一版快照——发生项业务时点那一版还是当前版——要一并定）。
3. **只读口的键**——归 TF owner。按 `ChargeOccurrenceKey` 精确到有效性版本（PP 的 `EligibleSourceReferences` 带 `OccurrenceVersion`，钥匙齐，与 sa-cc/11 受理门「发生项引用身份 + 版本非空」对上），还是按发生项身份答「当前有效版本」（`ChargeOccurrenceKey` 头注写过「不需要任何『哪个是当前』的标记——那种标记会与 corrects_version 形成两个都能回答同一问题的口径」，答当前版本得另立口径，与 TF 既有取舍相抵）。

## 裁决（2026-09-14 22:1x 通道 1 代裁，TF owner 口径；依据是本票取证，钉 `db480695`）

1. **成员对象的形——候选 1：裸引用原样交，不带种类。** TF 不铸那两种身份，登记方今天也没被要求申报种类；让「载运对象」词条长出「种类」一格是 TF 语言的扩张，而今天唯一的需求方 PP 连「集运单元」这种评价对象都还没有（ADR-0111 四种），为一个还不存在的用法改写侧、加迁移、改登记入口不值。持引用方怎么分：PP 消费侧拿成员引用去 PS 的 [02](02-billable-weight-actual-measurement-and-declared-dimensions-read-port.md) / [03](03-ps-origin-destination-postal-route-read-port.md) 口按（租户，包裹身份）问，found=false 即「不是本上下文认的包裹」——一次探问，不是两次；是不是集运单元由 PP 消费侧再决定要不要问 NO（今天不问，停「输入不可得」并点名）。**红线照旧**：TF 不按引用前缀 / 形状猜种类。将来 PP 真要按种类分派时，种类由**登记方声明**（候选 2）再另票，不在读侧推。
2. **成员是集运单元时——不展开。** 本票只读口交集运单元引用原样；展开要读 NO「封装成员快照」、要定用哪一版（发生项业务时点那一版还是当前版），两者都是 NO / PP 的题：TF 不拥有成员关系（CONTEXT Boundaries 首条），TF 代展开等于替 NO 解释它的成员关系。归 PP 消费侧票（spec「不在本目录」第一条）与 PP owner 的评价对象题（spec 第二条）。
3. **只读口的键——按 `ChargeOccurrenceKey` 精确到有效性版本。** PP 的钥匙齐（`EligibleSourceReferences.Occurrence` + `OccurrenceVersion`，sa-cc/11 受理门要它非空），且与 TF 既有取舍一致：`ChargeOccurrenceKey` 头注「不需要任何『哪个是当前』的标记——那种标记会与 corrects_version 形成两个都能回答同一问题的口径」；答「当前版本」要另立一套口径，与之相抵。键不存在 → found=false，不答「最近一版」。
4. **口的形（作者按此落，名字作者定）**：`internal/transportfulfillment/ports/` 新文件一只只读接口（形照 `FailedAttemptSource`：一口一问、不带 `Save`、不交整条 `ChargeOccurrenceRecord`），按 `ChargeOccurrenceKey` 答「成员载运对象清单（原样 `CarriedObjectReference` 切片）+ 业务时点 + 主要业务范围 + found」；`adapters/postgres.ChargeOccurrences` 加一个方法满足它（同一结构体实现两口），不新建表、不新迁移；`internal/architecture` 端口清点 +1。PP 一侧零 diff（消费侧适配器归后继票）。
5. **完成判据写实**：照上面判据 1 / 2 / 4 / 5（判据 3「候选 2 的迁移」**不适用**，删）。补一条：`git grep -n 'CarriedObjectReference' -- internal/transportfulfillment/domain/offsite_pickup.go` 零 diff（词条不动）。
6. **能力边界**：裁的是口的宽窄与键；`ChargeOccurrences` 现有 SQL 能不能一次 JOIN 出成员清单归作者。读过本票全文与 spec；**没读** `charge_occurrence.go` / `transport_charge_occurrence.go` / `0007` 正文（经取证引文）、TF 发生项登记入口。作者量到与代码不符，以代码为准并写进判断项。

## 参照

sa-cc/11 [`11-pp-inbox-consumer-receives-evaluation-request-envelope.md`](../../sa-cc-funds-and-credential-seams/issues/11-pp-inbox-consumer-receives-evaluation-request-envelope.md) 裁决 4、完成记录「逐条对裁决」4、「进 main 记录」后继一句；[spec](../spec.md)「不在本目录」（PP 评价对象加集运单元一种归 PP owner）；`internal/parcelpricing/ports/pricing_input.go`（`PricingInputResolver` 头注、`EligibleSourceReferences.OccurrenceReferenced`）；`internal/parcelpricing/application/form_evaluation_from_request.go`（`missingInputReadPorts`）；`internal/transportfulfillment/ports/charge_occurrence.go`（`ChargeOccurrenceRegistry`、`ChargeOccurrenceKey` 头注、`FailedAttemptSource` 头注——窄只读口的先例与理由）；`internal/transportfulfillment/domain/offsite_pickup.go`（`CarriedObjectReference`）、`transport_charge_occurrence.go`（`TransportChargeOccurrence.Members`）；`migrations/transport_fulfillment/0007_transport_charge_occurrence.sql`（成员表）；TF `CONTEXT.md`「载运对象」「运输收费发生项」；NO `CONTEXT.md`「封装成员快照」「集运成员关系」；ADR-0098（发生项登记册）、ADR-0111（评价对象四种）、ADR-0025（消费侧适配器）。

## Comments

- 2026-09-14 21:1x · 通道 3（task-620bc8e7）：立票，未动代码。能力边界：核过 `ChargeOccurrenceRegistry` 方法集、TF 九只只读口无一含 `Occurrence`、`CarriedObjectReference` 无种类、`0007` 成员表无种类列、PP 评价对象四种无集运单元；**没读** TF 发生项登记入口（`internal/transportfulfillment/application`）今天由谁调、登记方手里有没有种类信息——「要裁的」1 候选 2 的代价那半靠 TF owner 与作者开工时量。
