# 计价重量要的实重 / 尺寸两源都没有读口：`node-operations` 今天没有实际测量的领域对象、登记册与迁移，`parcel-shipment` 的申报重量 / 尺寸只在授权作用域的查阅面上——`parcel-pricing` 造快照的「实重 + 尺寸」一格指不到

Category: enhancement
Status: draft——2026-09-14 21:1x 通道 3 立票（sa-cc/11 裁决 4 量「计费重量指不到」的提供方半边；task-620bc8e7，通道 1 派单）。只写票面未动代码；取证锚 main `db480695`
Blocked by: 无（sa-cc/11 已进 main 2026-09-14 20:4x；本票是它点名的第二只读口，两源各归各上下文）；**要裁的四条归 NO·PS owner，裁前不动代码**

**用词**：派单与 sa-cc/11 写「计费重量」。GLOSSARY「计费重量」两词条（客户 / 供应商）归 `settlement-accounting` 的财务采用，「计价重量」归 `parcel-pricing`（评价内从实重与体积重派生），「当前有效实测」归 `node-operations`。快照要的是**原始量**——实重与尺寸——本票只写「实重 / 尺寸」，两侧提供方也只给原始量。这一条不是要裁的，GLOSSARY 已定。

## 缺口（取证于 `db480695`，逐符号名）

**NO 半边——读口后面没有登记册：**

- `git grep -n -i -E 'measure|weight|dimension' -- internal/nodeoperations/ports/ internal/nodeoperations/domain/ migrations/node_operations/` → 零命中（exit 1）。
- `git ls-files migrations/node_operations` → `0001_reception` / `0002_collaboration_execution_consolidation` / `0003_consolidation_source_provenance` / `0004_parcel_containment_indexes`；没有测量表。
- `git ls-files internal/nodeoperations/domain | grep -v _test` → 节点收寄 / 实物控制 / 集运单元 / 集运作业事实 / 关务协作 / 查询作用域，没有测量对象。
- `git grep -n 'registry=measurement' -- internal/nodeoperations/adapters/http/query_node_operations_records_test.go` → 用例头注「测量与交接证据两区在存储上没有登记册，它们的名字也在封闭集之外：没有表就没有读法」——NO 自己已经如实记了。
- NO `CONTEXT.md`「实际测量」「当前有效实测」两词条与生命周期「当前有效实测和当前位置」在代码上零落地。

**PS 半边——事实在库上，但没有消费口：**

- `git grep -n -E 'type Declared(Weight|Dimensions|Measurement) struct' -- internal/parcelshipment/domain/declared_measurement.go` → 三型在：毛重必备（数值 + 单位引用），外廓三边可整体缺席（头注「小包申报常只报重量」）；`Dimensions()` 第二返回值如实答缺。
- `git grep -n 'measurementDocument' -- internal/parcelshipment/adapters/postgres/shipment_request.go` → 随委托 `snapshot` 列落库、读回重建。
- `git grep -n -E 'WeightValue|DimensionsUnit' -- internal/parcelshipment/ports/` → 只在 `ports.go` `DeclaredParcelViewRecord`：`ShipmentRequestViews.FindVisibleByID` 的详情行，键是 `AuthorizedQueryScope` + 委托 ID，值「保持客户引用原样的字符串（"2.50" 不规范化）」——这是给人查阅的读面（CONTEXT「授权查询作用域」），不是按（租户，包裹身份）答事实的消费口。
- `git ls-files migrations/parcel_shipment | grep customer_source_data` → `0003_customer_source_data.sql`：客户原始资料版本按资料范围（`SourceDataGroupReference`）落新版本——申报重量 / 尺寸若被修订，「哪一版是申报」要定。
- 单位：PS `MeasurementUnitReference` 是自由引用串；PP `Weight` 的 `WeightUnit` 与 `Dimensions` 的 `LengthUnit`（`IN` / `CM`）是封闭集——对表归 PP 消费侧，不在本票。

**PP 侧要什么：**

- `git grep -n -E '^\s+(actualWeight Weight|dimensions\s+\*Dimensions|members\s+\*MemberManifest)' -- internal/parcelpricing/domain/input.go` → 逐包裹主体：实重必备 + 尺寸可缺（PP `CONTEXT.md`「计价重量」：`MAX` 策略下缺尺寸评价保持待判断，不退回实重）；票级 / 主单级主体：`MemberManifest` 合计实重 + 可缺合计体积重（ADR-0111 Decision 二）。
- `git grep -n -A4 'var missingInputReadPorts' -- internal/parcelpricing/application/form_evaluation_from_request.go` → 第二条点名 `node-operations / parcel-shipment: no read port for measured or declared actual weight and dimensions (pricing weight)`。

## 语言从哪里来

- NO `CONTEXT.md`「实际测量」：「节点对明确作业实物在特定时间、位置和作业依据下取得的重量、尺寸、数量或其他物理测量事实。实际测量不可覆盖；当前有效实测依据有效性、对象范围和业务规则派生。`parcel-pricing` 依据计算目的和已解析商业依据形成计价重量，`settlement-accounting` 再形成客户或供应商计费重量的财务采用」。
- NO `CONTEXT.md` 规则：「每次实际测量必须保留对象、测量项、结果、单位、发生时间、位置、来源和作业依据」；「当前有效实测按明确业务规则从仍有效的实际测量派生。客户声明、监管申报、客户计费重量和供应商计费重量均不得覆盖实际测量，节点也不形成最终计费重量」。
- PS `CONTEXT.md`「客户原始资料」：「客户对寄件人、收件人、货物、申报和服务要求作出的声明性来源资料……不等于节点实测、正式申报资料、监管结果或最终计费资料」；规则「客户原始资料、节点实测、正式申报资料和结算资料具有不同来源与所有权」。
- GLOSSARY「当前有效实测」（所有者 NO；避免使用：最后一次称重、客户申报重量、计费重量）、「计价重量」（所有者 PP；避免使用：计费重量、实测重量）、「客户计费重量」「供应商计费重量」（SA）。
- `docs/domain/CONTEXT-MAP.md` `settlement-accounting → parcel-pricing` 那条边：「……节点作业 / 小包托运的实重尺寸……那一侧立」。

## 做法候选（两条以内，不选）

1. **两只口各归各，同一笔立**：NO 先立「实际测量」登记册（写侧：对象、测量项、结果、单位、发生时间、位置、来源、作业依据，追加不覆盖）与「当前有效实测」只读口，按（租户，作业实物关联的正式包裹身份或集运单元）答重量 + 尺寸 + 单位 + 发生时间 + 来源事实引用；PS 立「申报测量」只读口，按（租户，包裹身份）答 `DeclaredMeasurement` + 所读资料版本锚。两口互不知对方存在；两源并存按谁留在 PP 消费侧按已登记规则做。
2. **PS 先行，NO 拆前置**：PS 申报只读口先立（事实已在库上，只差口）；NO 半边拆成「实际测量登记册」前置票 + 本票只留读口。PP 消费侧在 NO 口到之前只拿得到申报——快照的 `factReferences` 与证据层级要如实写「来源是申报」，评价结果的解释里能看出来。

## 红线

- NO 不形成计价重量或计费重量、不替 PP 挑「用哪个重量」；PS 不把申报当实测、不替客户改值。
- 提供方只开只读口；PP 不 import 两侧 `application`；单位对表与两源择一在 PP 消费侧（ADR-0025）。
- 缺尺寸如实答缺（PS `DeclaredMeasurement.Dimensions` 第二返回值），不填默认尺寸；PP 侧 `MAX` 策略下待判断是 PP `CONTEXT.md` 既定停法，不在提供方绕。
- 当前有效实测的派生规则若属实例半边（租户规则），机制只给槽、读口答「未配置」，不种「最近一次」「来源证明力最高」之类的默认规则。
- 真实实测 / 真实申报属实例半边；夹具全 `SYN-`。

## 完成判据（待裁后写实；可 grep）

1. NO：`git grep -n -E 'type \w+ interface' -- internal/nodeoperations/ports/` 多出实际测量只读口（裁决 1 含登记册时另有 store 口 + `migrations/node_operations/` 新序号迁移）；真库用例按（租户，对象）取当前有效实测，无测量答 found=false，不造默认。
2. PS：`git grep -n -E 'type \w+ interface' -- internal/parcelshipment/ports/` 多出按（租户，包裹身份）答申报重量 / 尺寸的只读口，答案带资料版本锚（裁决 3）；尺寸缺席如实；真库用例一正一缺。
3. 两口方法集都不含 `Save`；`internal/architecture` 边界门禁绿；`internal/parcelpricing/**` 零 diff。
4. `docs/product/MECHANISM-INVENTORY.md` 干净检出重生成：NO / PS 端口声明各 +1（NO 含登记册时迁移 +1）。

## 地盘

`internal/nodeoperations/{domain,ports,adapters/postgres}`、`migrations/node_operations/`（新序号）；`internal/parcelshipment/{ports,adapters/postgres}`；NO 登记面若要开在线入口则 `cmd/parcel-api` 装配（共享文件，动前占号）。不动 `internal/parcelpricing/**`、`internal/transportfulfillment/**`、`internal/settlementaccounting/**`。

## 要裁的

1. **NO 读口后面没有登记册：先立「实际测量」登记册是本票前置还是一并做；「当前有效实测」怎么派生**——归 NO owner。NO `CONTEXT.md` 两词条零落地，读口无物可读。「当前有效实测按明确业务规则从仍有效的实际测量派生」那条规则是机制（例如同对象同测量项取最近一次仍有效的）还是实例半边（租户按来源 / 位置 / 设备定证明力），决定读口答的是一条派生结果还是一列原始测量交消费方自判；两条路在 NO 是领域语言题（词条要不要长出「派生规则」一格），NO owner 裁。
2. **两源并存时按谁；这是机制还是租户规则**——归 NO·PS owner 与 PP owner 共。NO `CONTEXT.md`「客户声明……不得覆盖实际测量」已定一半：实测在，申报不能顶它。另一半——实测不在时申报能不能顶、按计算目的分不分（BUY 评价要不要拒申报）——是不是租户的已登记规则（实例半边）而非机制。若是实例半边，两只读口各答各的、选择留在 PP 消费侧读已登记规则；若是机制，要写进 PP `CONTEXT.md`「计价输入快照」词条。本票只列不裁。
3. **PS 申报读哪一版**——归 PS owner。接受基线那份（`委托接受基线` 冻结的成员测量）还是当前采用的客户原始资料版本（测量范围上有修订时）；`收件地点引用` 的锚法（基线锚 / 已采用版本锚 / 待复核不给）可照，与 [03](03-ps-origin-destination-postal-route-read-port.md) 的邮编同一张纸。答案是否带资料版本锚随之定——PP 快照 `factReferences` 要的是版本化事实引用，没有锚它记不下「按哪一版申报算的」。
4. **成员是集运单元时读什么**——归 NO owner。发生项成员是集运单元时读的是单元实测（整袋重）还是成员逐件（要经封装成员快照展开）；与 [01](01-tf-charge-occurrence-member-object-read-view.md)「要裁的」2 同根，两票若同期在途合一裁。

## 参照

sa-cc/11 [`11-pp-inbox-consumer-receives-evaluation-request-envelope.md`](../../sa-cc-funds-and-credential-seams/issues/11-pp-inbox-consumer-receives-evaluation-request-envelope.md) 裁决 4、完成记录「逐条对裁决」4；[spec](../spec.md)「用词」「不在本目录」；`internal/parcelpricing/domain/input.go`（`PricingInputSnapshot` 实重 / 尺寸 / 成员清单三格、`CalculatePricingWeight`、`deriveVolumetricWeight` 头注「尺寸没到是一个之后还可能补上的事实缺失」）；`internal/parcelpricing/application/form_evaluation_from_request.go`（`missingInputReadPorts`）；`internal/parcelshipment/domain/declared_measurement.go`（`DeclaredMeasurement`）；`internal/parcelshipment/ports/ports.go`（`DeclaredParcelViewRecord` / `ShipmentRequestViews`——查阅面，不是消费口的先例）；`internal/nodeoperations/adapters/http/query_node_operations_records_test.go`（NO 自记「没有登记册」）；NO `CONTEXT.md`「实际测量」「当前有效实测」与「测量、位置、状况与核对」规则节；PS `CONTEXT.md`「客户原始资料」「客户原始资料版本」「委托接受基线」；PP `CONTEXT.md`「计价重量」「体积系数」；GLOSSARY「当前有效实测」「计价重量」「客户计费重量」「供应商计费重量」；ADR-0111 Decision 二（成员清单）、ADR-0025。

## Comments

- 2026-09-14 21:1x · 通道 3（task-620bc8e7）：立票，未动代码。**与派单不符**：派单写「PP 无读口」，实测 NO 那半连领域对象、登记册与迁移都没有——缺的不是一只口，是口后面的登记册；PS 那半事实在库上、有授权作用域查阅面，缺的确是一只按（租户，包裹身份）答的消费口。能力边界：核过 NO ports / domain / 四份迁移零测量、PS `DeclaredMeasurement` 三型与 `shipment_request` 快照落库、`DeclaredParcelViewRecord` 是唯一读面、PP 快照三格；**没读** PS 客户原始资料版本对测量范围的修订在 `adapters/postgres` 怎么读回——「要裁的」3 那半靠 PS owner 与作者开工时量。
