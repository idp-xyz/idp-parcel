# `parcel-pricing` 造计价输入快照要的三只输入读口：TF 发生项成员对象 / NO·PS 实重尺寸 / PS 邮编路线——三张提供方侧机制票

Category: chore
Status: in-progress——**2026-09-14 22:1x 通道 1 按用户「你是业务和系统专家，自决」代裁三票「要裁的」（各以 TF / NO·PS / PS owner 口径写入各票「裁决」节）并转 ready-for-agent**：01 裸引用窄口按键精确到版本、不展开集运单元；02 拆成 PS 申报口（本票）+ NO 实际测量登记册与只读口（新票 04，NO 半边作者自立）、两源规则归 PP 消费侧票写进 PP CONTEXT；03 邮编成 PS「地址要素」一格、一口两段、起点先答寄件人、不过作用域。此前——2026-09-14 21:1x 通道 3 按通道 1 派单 task-620bc8e7 立目录与三张子票（全部 draft，各带「要裁的」归提供方 owner；裁前不动代码）；子票全 resolved 本 spec 才 resolved。只写票面未动代码；取证锚 main `db480695`

## 从哪里分出来

sa-cc/11 [`11-pp-inbox-consumer-receives-evaluation-request-envelope.md`](../sa-cc-funds-and-credential-seams/issues/11-pp-inbox-consumer-receives-evaluation-request-envelope.md) 裁决 4 与完成记录判据 4（第二个会话 16:2x 钉 `01974923` 量，通道 1 复核同意）：造计价输入快照要四样——**业务时点**指得到（`Sources().Occurrence.OccurredAt()` 在 SA 评价请求上）；**包裹主体**指不到（发生项成员在 TF `ChargeOccurrenceRegistry.FindByKey` 写侧口上，无只读视图，成员是包裹身份还是集运单元字面分不出）；**分区**指不到（起讫邮编归 PS，PP 无读口）；**计费重量**指不到（发生项只有计数 + 单位，实重 / 尺寸归 NO 实测 / PS 申报，PP 无读口）。落法：`internal/parcelpricing/ports/pricing_input.go` `PricingInputResolver` 只立形不立实现，生产装配不接任何实现，入口对缺席的实现答 `PRICING_INPUT_UNAVAILABLE` 并点名三只读口（`internal/parcelpricing/application/form_evaluation_from_request.go` `missingInputReadPorts` 三条，头注写了各归谁）。

sa-cc/11「进 main 记录」（2026-09-14 20:4x）后继一句「三只输入读口（TF 发生项成员对象 / NO·PS 实重尺寸 / PS 邮编路线）各归提供方上下文立票，接上后在 `formEvaluationOnEvaluationRequestConsumer` 补 `Inputs` 一行」——推送方把立票归自己，20:5x 派给通道 3 立。`docs/domain/CONTEXT-MAP.md` `settlement-accounting → parcel-pricing` 那条边（sa-cc/11 补的计价侧一句）已经把三只口的归属写进权威文档：「那些读口归拥有事实的上下文（运输履约的发生项成员、节点作业 / 小包托运的实重尺寸、小包托运的邮编路线）那一侧立，计价不猜、不填、不拿默认值顶」。本目录是那一句的去处。

**用词**：sa-cc/11 与派单里的「计费重量」在 GLOSSARY 里归 `settlement-accounting`（客户计费重量 / 供应商计费重量的财务采用），PP 自己的词是「计价重量」（从快照的实重与体积重派生），提供方给的是「实际测量」（NO）与申报的重量 / 尺寸（PS）。三张票统一写「实重 / 尺寸」指快照要的原始量，不再用「计费重量」。

## 取证（`db480695`，逐条可重跑；只记命令与结果，结论在各票）

- `git grep -n -A3 'type ChargeOccurrenceRegistry interface' -- internal/transportfulfillment/ports/charge_occurrence.go` → `FindByKey` + `Save` 两方法，头注「只有首登与读回，没有 Update」。
- `git grep -n -E 'type \w+(View|Read|Source|Reader) interface' -- internal/transportfulfillment/ports/` → 9 只只读口；再 `| grep Occurrence` → 0。
- `git grep -n -B2 'type CarriedObjectReference struct' -- internal/transportfulfillment/domain/offsite_pickup.go` → `struct{ requiredValue }`，头注「对正式包裹身份或集运单元的引用——两者分别属 parcel-shipment 与 node-operations，本上下文不铸造它们」。
- `git grep -n 'object_ref' -- migrations/transport_fulfillment/0007_transport_charge_occurrence.sql` → `transport_charge_occurrence_member.object_ref text NOT NULL`，主键 `(tenant_id, occurrence_ref, validity_version, object_ref)`，无种类列。
- `git grep -n -l 'ChargeOccurrence' -- internal/ cmd/ | grep -v '^internal/transportfulfillment/'` → 只命中 `internal/settlementaccounting/**`（自铸 `ChargeOccurrenceID` 作引用）、`internal/parcelpricing/adapters/settlementaccounting/*_test.go`、两处 `cmd/*/assemble*_test.go`、`internal/architecture/production_wiring_baseline.txt`。
- `git grep -n -i -E 'measure|weight|dimension' -- internal/nodeoperations/ports/ internal/nodeoperations/domain/ migrations/node_operations/` → 零命中（exit 1）。
- `git ls-files migrations/node_operations` → `0001_reception` / `0002_collaboration_execution_consolidation` / `0003_consolidation_source_provenance` / `0004_parcel_containment_indexes`。
- `git grep -n 'registry=measurement' -- internal/nodeoperations/adapters/http/query_node_operations_records_test.go` → 用例头注「测量与交接证据两区在存储上没有登记册，它们的名字也在封闭集之外：没有表就没有读法」。
- `git grep -n -E 'type Declared(Weight|Dimensions|Measurement) struct' -- internal/parcelshipment/domain/declared_measurement.go` → 三型在；`git grep -n 'measurementDocument' -- internal/parcelshipment/adapters/postgres/shipment_request.go` → 随委托 `snapshot` 列落库。
- `git grep -n -E 'WeightValue|DimensionsUnit' -- internal/parcelshipment/ports/` → 只在 `ports.go` `DeclaredParcelViewRecord`（`ShipmentRequestViews.FindVisibleByID` 的详情行，键 `AuthorizedQueryScope` + 委托 ID，值为客户原样字符串）。
- `git grep -n -i -E 'postal|zip|邮编|address' -- internal/parcelshipment/domain/ internal/parcelshipment/ports/ migrations/parcel_shipment/` → 非测试零命中；测试命中只有 `CONSIGNEE_ADDRESS`（`SourceDataGroupReference` 合成串）与 `ADDRESS_REJECTED` / `ADDRESS_UNSUPPORTED`（面单拒收原因串）。
- `git grep -n -A8 'type DeliveryPlaceReferenceView interface' -- internal/parcelshipment/ports/delivery_place_reference_view.go` → 按（租户，声明包裹身份）答 `domain.DeliveryPlaceResolution` 封闭四格；交引用不交地址本体。
- `git grep -n -E 'Members\s+\[\]canonicalMemberDocument|Scope\s+\[\]canonicalEntryDocument|Service\s+\[\]canonicalEntryDocument' -- internal/parcelshipment/domain/payload_canonicalization.go` → 客户请求规范化文档：成员（引用 + 重量 + 尺寸）、范围与服务各为 name/value 条目列表。
- `git grep -n -E '分区|邮编' -- docs/domain/network-routing/CONTEXT.md` → 唯一命中「服务区域」词条（「邮编范围……用于把客户地址解析为候选收寄节点、交付节点或尾程注入节点」）；「分区」零命中。
- `git grep -n -c '分区' -- docs/domain/parcel-pricing/CONTEXT.md` → 9；命中含「计价参考目录」词条（「承运商的分区表（目的邮编 → 分区）……绑定了目录的方案，评价从目录解析分区与档位，查不到即评价待判断，不给默认分区或默认档位；未绑定的方案保留由调用方给出分区的路径」）。
- `git grep -n -B2 -E 'type PostalRoute struct' -- internal/parcelpricing/domain/reference_catalogue.go` → `origin` / `destination`，头注「目的邮编必备，始发邮编随目录的始发维度需要而给」。
- `git grep -n -E '^\s+(subject\s+EvaluationSubject|zone\s+string|postal\s+\*PostalRoute|actualWeight Weight|dimensions\s+\*Dimensions|members\s+\*MemberManifest|businessAt\s+time.Time)' -- internal/parcelpricing/domain/input.go` → 快照四样在类型上的形：主体（四种 `EvaluationSubjectKind`）、分区或邮编路线（二者至少一个在）、实重 + 可缺尺寸或聚合主体的成员清单、业务时点。
- `git grep -n -A4 'var missingInputReadPorts' -- internal/parcelpricing/application/form_evaluation_from_request.go` → 三条：`transport-fulfillment: no read-only view of the charge occurrence's member carried objects (evaluation subject)` / `node-operations / parcel-shipment: no read port for measured or declared actual weight and dimensions (pricing weight)` / `parcel-shipment: no read port for the origin / destination postal route (zone)`。
- `git ls-files internal/parcelpricing/adapters | cut -d/ -f4 | sort -u` → `http` / `identity` / `inbox` / `postgres` / `settlementaccounting` / `sourcefeed`；无 `transportfulfillment` / `nodeoperations` / `parcelshipment` 消费侧适配器。

## 子票

| 票 | 一只读口 | 提供方上下文 | Blocked by |
|---|---|---|---|
| [01](issues/01-tf-charge-occurrence-member-object-read-view.md) | 运输收费发生项的成员对象只读视图——按发生项键答成员载运对象与业务时点，能分清成员是正式包裹身份（PS）还是集运单元（NO）；不是 `FindByKey` 写侧口的再导出——22:1x 裁：裸引用原样不带种类、不展开、键精确到有效性版本；**resolved，2026-09-15 11:5x 进 main**（首波批 tip `c657a5e7`，重放 `3b91cb5a→28c3c063` / `71dd8313→c88b5e35` / `b33cdbba→d710fada`；评审 ← 通道 6 两轴 0 阻断；此前 11:0x 通道 3 完工待评审）（接手链：通道 4 认领 → crash → 推送方封存 `mcp4-ppseams01@3b91cb5a` → 通道 3 续做 `mcp3-ppseams01` 基 `3b91cb5a`：新口 `ports.ChargeOccurrenceMemberView.LoadMembers` + `ChargeOccurrences.LoadMembers` 一条 LEFT JOIN 在封存笔，接手方先写 red 再读、两条并入 `71dd8313`；带 DSN TF postgres + architecture + parcel-api + parcel-dispatch 677 PASS / 0 FAIL / 0 SKIP；清点预报 TF 生产 +1 / 测试 +1、端口声明 408→409、两口径缺数不变；PP 零 diff；判断项五条见票面） | TF | 无 |
| [02](issues/02-billable-weight-actual-measurement-and-declared-dimensions-read-port.md) | 实重 / 尺寸读口——**PS 半边 resolved，2026-09-15 11:5x 进 main**（首波批 tip `c657a5e7`，重放 `1a30aac5→2acbaac1` / `23a147c4→271973db`；推送方自评两轴 0 阻断，Spec 1 非阻断归 05；2026-09-15 通道 5，`mcp5-ppseams02-03@1a30aac5`）：`ports.DeclaredMeasurementView` 按（租户，包裹身份）答 `DeclaredMeasurement` + 资料版本锚，封闭五格；已采用版本锚那一格只交锚不交测量（客户原始资料版本今天只留痕不留内容，见票内判断项 ①）。NO 半边在 04 | NO·PS | 无；要裁的归 NO·PS owner |
| [03](issues/03-ps-origin-destination-postal-route-read-port.md) | 起讫邮编读口——**resolved，2026-09-15 11:2x 进 main**（第二波批 tip `444513c7`，重放 `2a637107→1bd50208` / `3209c4b3→6e712d64`；评审 ← 通道 6 两轴 0 阻断 / 各 1 非阻断；此前 2026-09-15 通道 5，`mcp5-ppseams02-03@2a637107`，裁决 7 追裁取 A）：PS CONTEXT 长出「地址要素」（`POSTAL_CODE` / `COUNTRY_CODE` 封闭集）；`ports.AddressElementsView` 一口两段（起点 = 寄件资料范围、目的 = 收件资料范围），每段封闭五格；**口对今天全部快照答「要素缺席」**——寄收件条目只进 `PayloadDigest`，内容落库归 05 | PS | 无；要裁的归 PS owner |
| [05](issues/05-ps-submission-and-source-data-versions-carry-content.md) | PS 提交版本与客户原始资料版本留内容（测量 / 地址要素），使 02 / 03 两口能按基线锚与已采用版本锚答值——今天条目只进摘要、`CustomerSourceDataVersion` 只留痕；**2026-09-15 12:5x 通道 1 按用户「代裁」代裁（PS owner 口径），ready**：只留封闭要素（申报测量、地址要素）不留原文；两份既有 jsonb 快照各加可缺席内容子段、零迁移、旧快照如实答缺；内容与摘要由 `CanonicalizeSubmissionPayload` 一次调用成对产出、两条命令一起带、应用层不重算、不设领域构造门；CONTEXT「保留原始请求」仍指指纹、另加一句留封闭要素内容；06 评审两条 PS 残差顺带收进——**resolved，2026-09-15 14:4x 进 main**（第六批，重放 `1d3fb469→3fee7544` / `636afbd3→6fffbd9e` / `6a620e4a→60635fe8` / `2c124b8e→3035765e` / `a893999a→969b2007` / 记录笔 `f7580e47→30113519`，批 tip `de822820`；评审 ← 通道 3 两轴 0 阻断（Spec 唯一「阻断」是清点未重生成，推送方清点笔 `de822820` 兑）/ Standards 1 非阻断 → [07](issues/07-ps-content-fixtures-use-real-postal-and-country-codes-instead-of-syn.md)；接手链：通道 5 13:1x 认领、13:2x–13:43 四笔代码推齐 → 13:5x 占号跑真库后会话结束未释号未留数 → 通道 5 新会话 14:25 重跑 2219 PASS / 0 / 0 并补完成记录 `f7580e47`；`CanonicalizeSubmission` 并列新函数成对交回摘要 + 寄 / 收要素、既有签名与摘要用例零改；`versionDocument.elements` / `sourceDataVersionDocument.content` 子段 `omitempty` 零迁移；`SourceDataVersionContent` 范围 × 形封闭组合、清空必空；两口已采用格带那一版自己的值；`AddressElementsOf` 出接线基线；判断项九条见票面——⑤ 修订路两条命令无生产填写方（等 PAR-INT-01）、⑥ admin-web 提交页未长四格为越地盘残差；`de822820` 带 DSN 全仓 115 ok / 0 FAIL；清点 PS 186→187 / 178→181） | PS | 无（02 / 03 两口的已采用格与 03 基线格由此能答值；PP 消费侧适配器票的「输入不可得」前提解除一半，真渠道 Intake 仍等 PAR-INT-01） |
| [07](issues/07-ps-content-fixtures-use-real-postal-and-country-codes-instead-of-syn.md) | 05 评审 ← 通道 3 Standards ① 尾巴：四份 PS 内容用例收件夹具用真实邮编 `10115` / `20095` / `20097` / `20099` 与 `DE`，寄件格已是 `SYN-200000`；03 落 main 的两份地址要素用例同款——默认全改 `SYN-`（取「公开邮编不算实例半边」则归 PS owner 一句改词条）。只测试夹具，零行为。2026-09-15 14:4x 通道 1 立 ready。**另记残差**（不成票）：05 判断项 ⑥ admin-web 提交页未长寄 / 收邮编与国家 / 地区码四格，归 admin-web 侧随真渠道 Intake / 演示页票 | PS 六份测试文件 | 无 |
| [04](issues/04-no-actual-measurement-registry-and-valid-measurements-read-port.md) | NO 实际测量登记册（追加不覆盖）+「仍有效的实际测量」只读口——从 02 裁决 1 拆出：NO 今天连登记册都没有；只读口交原始量清单、**不派生**「当前有效实测」（派生规则属实例半边）；测量怎么进登记册（登记入口）留后继 | NO | 无（02 裁决 1 拆出；票面由 NO 半边作者按 02 裁决 1 自立） |
| [06](issues/06-tf-ps-ppseams01-03-review-tails-member-order-contract-count-words-and-blank-value-reading.md) | 01 / 03 评审 ← 通道 6 非阻断尾巴合收：01 两处「三件」去数、成员顺序写进 `ChargeOccurrenceMemberView` 契约；03 `AddressElements` / `AddressElementsOf` / `AddressElementsView` 三处头注同口（默认以 `CanonicalContentEntry`「显式清空 = 空串」为准，全空白原样在场）、「全部快照答缺席」收窄到基线格。2026-09-15 11:2x 通道 1 立——**resolved，2026-09-15 12:3x 进 main**（第四批，重放 `69189208→4ca2ccac`，批 tip `676cc09b`；评审 ← 通道 4 两轴 0 阻断 / Standards 1 非阻断 / Spec 2 非阻断；此前 12:3x 通道 3 完工，task-93aa833b，分支 `mcp3-ppseams06` 基 `7160fe67` 单笔六件 +51 −11 零增删；唯一行为改动 `AddressElementsOf` 在场判定 `TrimSpace(v) != ""` → `v != ""`（今天无非测试调用方、运行时不可达）+ 一格新用例；判断项七条见票面；残差三条归 PS owner 随 05 / PP 消费侧票 / TF owner；批 tip 带 DSN 全仓 113 ok / 0 FAIL；清点零差） | TF 一处注释 + PS 三文件 | 无 |

三票互不阻：02 / 03 的读口按（租户，包裹身份）键，不必等 01 先到；01 只是让 PP 消费侧知道该拿哪个包裹身份去问 02 / 03。PP 消费侧适配器（不在本目录）Blocked by 三票全部进 main。

## 为什么并成一个目录

三只口都为同一份 `PricingInputSnapshot` 服务：`PricingInputResolver` 只在四样都指到时答 `PricingInputResolved`，任一样指不到就是 `PricingInputUnavailable`（`pricing_input.go` 两格头注）——一只口进了 main、另两只没进，生产图上的评价请求仍停在同一格，用户看不出任何差别。拆到 TF / NO / PS 各自目录，「三只合起来够不够、谁先谁后、集运单元这一根线怎么穿过三只口」这三句就没人写：01 的成员对象种类、02 的集运单元实测、03 的起点邮编是不是节点，是同一根线（发生项成员是集运单元时四样怎么指）在三只口上的三个切面。

## 边界（三票共用）

- 三件全是**机制半边**：让 TF / NO / PS 已拥有（或按 CONTEXT 该拥有）的事实能被 PP 按引用读到。真实实测、真实申报、真实邮编、真实分区表全属实例半边（本仓尚无租户），夹具全 `SYN-`，不写任何默认重量 / 默认尺寸 / 默认邮编 / 默认分区。
- **提供方只开只读口，不为消费方派生判断**：NO 不形成计价重量或计费重量（NO `CONTEXT.md`「节点也不形成最终计费重量」）；PS 不算分区、不解析服务区域；TF 不替 PS / NO 铸身份、不替 NO 展开集运成员关系。选哪个重量、解哪个分区、成员怎么映射评价对象，全在 PP 消费侧按已登记规则或已裁口径做。
- **消费方 PP 不 import 提供方 `application`**；跨上下文翻译只在 PP 侧 `internal/parcelpricing/adapters/<provider>/`（ADR-0025），`internal/architecture` 边界门禁是判据。
- 只读口不是写侧登记册的再导出：不带 `Save`，不暴露整条登记记录；形照 TF `FailedAttemptSource` 头注那条理由（「契约窄一格，说的话就准一格」）。
- 未确认参数保持可配置或显式未决：成员对象种类的封闭集、当前有效实测的派生规则、邮编前缀粒度，任何一个由裁决或实例定，不预拟。

## 不在本目录

- **PP 消费侧适配器**：实现 `PricingInputResolver`，落 `internal/parcelpricing/adapters/<provider>/`（三只口各一只消费侧适配器，或一只编排三口），`docs/domain/CONTEXT-MAP.md` 加 PP→TF / PP→NO / PP→PS 三条消费边，`cmd/parcel-dispatch/assemble.go` `formEvaluationOnEvaluationRequestConsumer` 补 `Inputs` 一行——三只口进 main 后另立，归 PP。单位对表（PS `MeasurementUnitReference` 自由串 → PP `WeightUnit` / `LengthUnit` 封闭集）、成员对象 → `EvaluationSubject` 映射、两源并存时按谁，都是那张票的事。
- **PP 评价对象要不要加「集运单元」一种**（ADR-0111 四种里没有）——归 PP owner，01「要裁的」2 会碰到它，本目录只记不裁。
- **两源并存按谁**（02 裁决 2）：机制规则「实重与尺寸优先取仍有效的实际测量；无实测取申报并在快照事实引用里标来源；实测在时申报不得顶替；某一计算目的是否拒用申报保持可配置或显式未决」——写进 PP `CONTEXT.md`「计价输入快照」，随 PP 消费侧适配器票落，不在提供方四票。
- **起点为节点邮编时的读口**（03 裁决 3）：承运商分区表按注入 / 收寄节点分始发区时，起点邮编的提供方是 NR（节点身份）/ NO（节点收寄），不是 PS——等首份真实分区表声明始发维度（ADR-0109 决定二）再立，归 NR / NO。
- **NO 实际测量的登记入口**（04 不做）：测量怎么进登记册（节点作业事件 / 设备 / 人工）——04 只立登记册与只读口，入口另票归 NO。
- **不重开 sa-cc/11 已裁的任何一格**：入口形（`FormEvaluationFromRequestHandler` 七格）、回指一格、在用价卡解析口三格、`PRICING_INPUT_UNAVAILABLE` 不是领域`待判断`、消费者与消费侧读口的落点。三票只在提供方一侧开口。
- 计费重量的财务采用（SA）、计价重量的派生（PP 评价内）——两个词的归属 GLOSSARY 已定，本目录不碰。

## Comments

- 2026-09-14 21:1x · 通道 3（task-620bc8e7）：立目录与三张子票，全部 draft。**只写 .md，未动代码。** 实测与派单不符之处（接受与否推送方判）：① 派单写 NO·PS「PP 无读口」，实测更深——NO 今天没有实际测量的领域对象、登记册与迁移（取证第 6–8 条），读口后面无物；PS 的申报重量 / 尺寸在领域与库上都在，但唯一读面是 `ShipmentRequestViews` 授权作用域查阅面，没有按（租户，包裹身份）答的消费口。② 派单写 PS 邮编「PP 无读口」，实测更深——PS 非测试代码与迁移里没有任何结构化邮编字段，寄收件资料是 name/value 条目；`DeliveryPlaceReferenceView` 只交引用，CONTEXT 说的「持引用方按引用向本上下文取」那只口今天不存在。③ 派单让查 NR `CONTEXT.md` 分区归属再写——NR 无「分区」一词，只有路由用的「服务区域」；分区解析归 PP（PP `CONTEXT.md`「计价参考目录」+ ADR-0109 Decision 四），03 因此不把「PP 算还是 NR 算」列为要裁的，只记作防重开。④ 01 顺带量到 PP `EvaluationSubjectKind` 四种里没有集运单元，发生项成员是集运单元时评价对象无处落——归 PP owner，记在「不在本目录」。能力边界：读过三份 CONTEXT + NR / PP CONTEXT 相关词条、GLOSSARY 相关词条、三个上下文的 `ports/` 与相关 `domain/` 文件、`0007` / NO 四份迁移 / PS `0003`；**没读** TF 发生项登记入口（`application`）与 PS 客户原始资料版本的读回路径（`adapters/postgres`），各票「要裁的」里靠 owner 与作者开工时量的部分已点名。
