# `transport-fulfillment` 运输收费发生项的成员对象没有只读视图：`ChargeOccurrenceRegistry.FindByKey` 是带 `Save` 的写侧登记册口，成员 `CarriedObjectReference` 字面分不出正式包裹身份还是集运单元，`parcel-pricing` 造快照的「包裹主体」一格指不到

Category: enhancement
Status: resolved——**2026-09-15 11:0x 通道 3 完工待评审进 main**。接手链：通道 4 认领（task-aadedbce，`mcp4-ppseams01` 基 `3a21dab7`）→ 做到「带 DSN 4 包 675 PASS，转入评审与提交」时 crash（一笔未提交、一笔未推）→ 推送方 10:2x 把四件未提交现场原样封存 `mcp4-ppseams01@3b91cb5a`（`chore(salvage)`，非集成候选、一字未改）→ 用户裁「改派通道 3 续做」（task-5c5e6165）→ 通道 3 从封存笔接着做：分支 `mcp3-ppseams01` 基 `3b91cb5a`，接手方先写自己的 red 再读对方代码（parallel-sessions「镜像测试与真测试同形」），两条并入对方用例文件 `71dd8313`，验证与逐条判据见「完成记录」。此前 in-progress——**2026-09-15 10:2x 通道 4 认领（task-aadedbce），分支 `mcp4-ppseams01` 基 `3a21dab7`，按下方「裁决」节落**。此前 ready-for-agent——**2026-09-14 22:1x 通道 1 按用户「你是业务和系统专家，自决」代裁（TF owner 口径），三条「要裁的」写入下方「裁决」节**：候选 1 窄只读口、成员**裸引用原样交**（不带种类、不按前缀猜）；**不展开**集运单元；键按 `ChargeOccurrenceKey` **精确到有效性版本**。此前 draft——2026-09-14 21:1x 通道 3 立票（sa-cc/11 裁决 4 量「包裹主体指不到」的提供方半边；task-620bc8e7，通道 1 派单）。只写票面未动代码；取证锚 main `db480695`
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

## 完成记录

分支 `mcp3-ppseams01`，基封存笔 `3b91cb5a`（其父 `3a21dab7` = 派单时 main；隔离树 `$env:TEMP\idp-parcel-mcp3-ppseams01`，`fetch` 后 `worktree add`，干净）：

| SHA | 作者 | 内容 |
|---|---|---|
| `3b91cb5a` | 通道 4 现场 · 推送方封存 | `chore(salvage)`：`ports/charge_occurrence_member_view.go`（+37：`ChargeOccurrenceMembers{Members, OccurredAt, Scope}` + `ChargeOccurrenceMemberView.LoadMembers(ctx, ChargeOccurrenceKey) (ChargeOccurrenceMembers, bool, error)`）、`adapters/postgres/charge_occurrence_registry.go`（+70：`ChargeOccurrences.LoadMembers` 一条 LEFT JOIN + 编译期断言 `var _ ports.ChargeOccurrenceMemberView = (*ChargeOccurrences)(nil)`）、`adapters/postgres/charge_occurrence_member_view_test.go`（+132，五条真库用例）、本票 Status 一行 |
| `71dd8313` | 通道 3 | 接手方 red 两条并入对方用例文件：`TestChargeOccurrenceMemberViewIsANarrowProjectionOfTheRegisteredRow`、`TestChargeOccurrenceMemberViewDoesNotInterpretReferenceShapes`；对方 `…AnswersNotFoundForAnUnknownKey` 补一格 `Scope` 零值断言。端口文件与适配器方法零改动 |
| （本笔） | 通道 3 | 本票 Status → resolved + 本完成记录；pp-seams `spec.md` 01 行 |

**接手纪律（先写 red 再读对方）**：读票面「裁决」与 spec 后、未打开对方三件之前，接手方在独立文件写了三条真库用例并对真库跑 **PASS 非 SKIP**（`-run TestTheMemberView -count=1 -v`）：(a) 同发生项另一有效性版本的键 found=false、不存在的键零值（含 `Scope`）、v1 / v2 各与 `FindByKey` 同版本逐项相等；(b) 三条形状各异的裸引用原样交回、不多不少；(c) 他租同键 found=false。然后才读对方的口、方法与用例，**对判据不对条数**：对方五条钉的是——按键取成员 / 时点 / 范围（判据 2）、未知键 found=false 不造默认（判据 2）、键精确到版本、修订后两版各答各的（裁决 3）、他租不可见、本体在册而成员表空 → 响亮报错（对方实现头注自陈的库面不一致格）。对上：(a) 的版本半边与对方第三条同判据，(c) 与对方第四条同判据同切法——**弃 (c)**；(a) 的「与登记册整条读回逐项相等」与 (b) 的「形状各异原样交」对方没有那一切法——**并入**为两条；对方 NotFound 一条只断了成员与时点两件，接手方补 `Scope` 第三件。**对不上的地方：无**——两人从裁决 1 / 3 与判据 2 切出的断言彼此印证，没有暴露谁想错了；这是交叉验证的「印证」一格，不是「找到分歧」一格。

**逐条对完成判据（裁决 5，钉 `71dd8313`）**：**(1)** `git grep -n -E 'type \w*Occurrence\w* interface' -- internal/transportfulfillment/ports/` 命中 `ChargeOccurrenceRegistry`（既有）与 `ChargeOccurrenceMemberView`（新）；`git grep -n 'Save' -- internal/transportfulfillment/ports/charge_occurrence_member_view.go` **零命中**（头注写「首登方法」而不提名，刻意）。**(2)** 编译期断言在适配器（`charge_occurrence_registry.go`）与用例文件各一处；真库：按键取到成员清单 + 业务时点 + 主要业务范围（对方第一条 + 接手方投影条）；键不存在 found=false、零值不造默认成员（对方第二条 + 接手方补的 `Scope` 格）；版本不同 found=false、不答最近一版（对方第三条 + 接手方投影条的 v1 / v2 各答各的）。**(3)** `tools/mechanism-inventory` 在提交后的干净树重生成到 `%TEMP%`（不落仓）与 main 上那份 diff：TF 生产 140→**141**、测试 128→**129**、合计 978→979 / 930→931；**端口声明 408→409**；基线口径缺 14 / 精确口径缺 7 **不变**（新口已被 `ChargeOccurrences` 实现，两口径都不缺）——预报数，推送方在 tip 兑底。**(4)** `internal/architecture` 带 DSN **ok**；`git diff --stat 3a21dab7 -- internal/parcelpricing/` **空**。**(5)** `git diff --stat 3a21dab7 -- internal/transportfulfillment/domain/offsite_pickup.go` **空**（`CarriedObjectReference` 一字未动）。

**逐条对裁决 1–4**：**1** 裸引用原样、不带种类——`ChargeOccurrenceMembers.Members` 是 `[]domain.CarriedObjectReference`，SQL 只 `SELECT member.object_ref` 不派生任何列，头注写明不按前缀 / 形状猜；接手方 `DoesNotInterpretReferenceShapes` 钉之 ✓。**2** 不展开集运单元——口上没有任何 NO 读口、无第二次查询，头注写「展开归持引用方的消费侧」✓。**3** 键精确到有效性版本——`WHERE tenant_id = $1 AND occurrence_ref = $2 AND validity_version = $3`，零行即 found=false；对方第三条 + 接手方投影条钉之 ✓。**4** 口的形——新文件、一口一问、不含 `Save`、不交整条 `ChargeOccurrenceRecord`（只交成员 / 时点 / 范围三件，头注写「读成员的一方不该看见协议、数量与修订三件」）；`ChargeOccurrences` 同一结构体满足两口；不新建表、不新迁移；PP 零 diff ✓。

**判断项（归 TF owner / 推送方）**：
① **现有 SQL 一次 JOIN 出成员清单——成立**（裁决 6 留给作者的题）：`LoadMembers` 一条 `LEFT JOIN` 同时取本体两列与成员逐行，不经 `FindByKey` 再投影（那条路要把整条发生项过重建门），`ORDER BY member.object_ref` 与 `FindByKey` 的 `loadMembers` 同序——接手方投影条按序逐项比对通过，两口读同一行的顺序一致。
② **LEFT JOIN 带来一个新格**：本体在册而成员表为空 → 报错而不是 found=true 空清单。这是库面不一致（领域构造门拒空成员、`Save` 两表同笔落），对方选「响亮报错」并钉了用例；接手方认可——把空清单当答案交出去会让 PP 把「没有成员」当成事实。它不在票面裁决里，是实现层的诚实格，写在此供 owner 知悉。
③ **对方第一条用例断言了 `ORDER BY object_ref` 的字面顺序**（`SYN-PARCEL-1, SYN-PARCEL-2, SYN-UNIT-7`）：这钉的是实现细节，不是票面规则——排序变了它会碎而行为没错。接手方未改（对判据不对条数；改对方断言不在接手纪律内），投影条改用「与 `FindByKey` 同序」来表达同一件事，不依赖具体排序。owner 若要收窄，删那一条的顺序断言即可。
④ **夹具前缀**：对方用例复用 `charge_occurrence_registry_test.go` 的 `occurrenceRecord`（租户 `tenant-1`、旅程 `journey-1` 等既有非 `SYN-` 前缀夹具），发生项与成员引用用了 `SYN-`；接手方并入时沿用同一套，未另立 `SYN-` 租户夹具（独立文件里那套随弃）。全部为合成登记，无真实发生项。
⑤ **消费侧仍缺**：`parcelpricing.PricingInputResolver` 在清点里仍是「精确口径缺」，本票不动 PP（裁决 4「PP 一侧零 diff」）；PP 消费侧适配器归后继票（spec「不在本目录」）。

**验证（隔离树 `$env:TEMP\idp-parcel-mcp3-ppseams01`，`71dd8313`）**：`gofmt -l .` 空、`go build ./...` 0、`go vet ./...` 0；反查 `go list -deps ./cmd/...` 含 `internal/transportfulfillment/adapters/postgres` 的只有 `cmd/parcel-api` 与 `cmd/parcel-dispatch`（与通道 4 所报一致，接手方自己反查一次）；**带 DSN** `-p 1 -count=1 -v` `internal/transportfulfillment/adapters/postgres` + `./internal/architecture/...` + `cmd/parcel-api` + `cmd/parcel-dispatch` → **677 PASS / 0 FAIL / 0 SKIP**（对方报的 675 + 接手方两条；对方那 675 不作本记录的证据）；占 / 释 55432 均已广播。

**接手方能力边界**：只读 `3b91cb5a` 隔离树；对方三件按接手纪律在 red 之后读，逐字读了口文件、适配器 +70 行 diff 与用例全文；`0007` 迁移正文未重读（经票面取证引文与 SQL 列名）；`internal/transportfulfillment/application` 发生项登记入口未读（本票不动它）；未跑全仓带 DSN（只跑派单点名的四包），全量由推送方在重放 tip 上兑；`tools/mechanism-inventory` 只跑了一次、输出落 `%TEMP%` 未入仓。

## Comments

- 2026-09-14 21:1x · 通道 3（task-620bc8e7）：立票，未动代码。能力边界：核过 `ChargeOccurrenceRegistry` 方法集、TF 九只只读口无一含 `Occurrence`、`CarriedObjectReference` 无种类、`0007` 成员表无种类列、PP 评价对象四种无集运单元；**没读** TF 发生项登记入口（`internal/transportfulfillment/application`）今天由谁调、登记方手里有没有种类信息——「要裁的」1 候选 2 的代价那半靠 TF owner 与作者开工时量。
- 2026-09-15 11:0x · 通道 3（task-5c5e6165，接手通道 4 封存现场 `3b91cb5a`）：接手方 red 先写后读、两条并入 `71dd8313`；判据 (1)–(5) 逐条、裁决 1–4 逐条、判断项五条、验证与能力边界见「完成记录」；Status resolved，评审从 2 / 5 / 6 里挑先交活的（接手方与通道 4 同为作者不评）。
