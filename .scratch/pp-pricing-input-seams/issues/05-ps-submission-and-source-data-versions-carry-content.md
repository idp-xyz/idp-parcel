# PS 提交版本与客户原始资料版本留内容（测量 / 地址要素），使 02 / 03 两口能按基线锚与已采用版本锚答值：今天寄收件 name/value 条目只进 `PayloadDigest`、`CustomerSourceDataVersion` 只留痕不留内容——两只读口的「已采用版本锚」一格只能交锚，03 连基线锚一格也只能答「要素缺席」

Category: enhancement
Status: in-progress——**2026-09-15 13:1x 通道 5 认领**（task-6dbb178b；分支 `mcp5-ppseams05` 基 main `bb7268f0`，隔离树 `%TEMP%\idp-parcel-mcp5-ppseams05`；按 /implement 走，判据 (1) 基线格与 (2) 已采用格先写 red）。此前 ready-for-agent——**2026-09-15 12:5x 通道 1 按用户「代裁」代裁（PS owner 口径），四条「要裁的」写入下方「裁决」节**：只留封闭要素（申报测量、地址要素）不留原文；走做法 1——两份既有 jsonb 快照各加一个可缺席的内容子段，零迁移、旧快照如实答缺；内容与摘要由 `CanonicalizeSubmissionPayload` 同一次调用产出、命令两样一起带、应用层不重算；CONTEXT「客户原始资料版本」词条「保留原始请求」仍指请求指纹，另加一句「保留封闭要素内容」。此前 draft——2026-09-15 11:3x 通道 5 立票（task-9cc0b1a6；通道 1 10:4x 追裁 [03](03-ps-origin-destination-postal-route-read-port.md) 取 A 时点名「顺手立 draft 05」）。只写票面未动代码；取证锚 main `3a21dab7`，代码事实在分支 `mcp5-ppseams02-03` 的 [02](02-billable-weight-actual-measurement-and-declared-dimensions-read-port.md) / 03 完成记录与判断项里逐符号可查。要裁的全归 PS owner，裁前不动代码
Blocked by: 无（02 PS 半边与 03 已在同分支落地——两口的形与答格已定，本票只让它们能答出值；两口的 PP 消费侧适配器票不等本票，先按「输入不可得」停）

**用词**：本票说的「内容」指客户原始资料里**已有消费方的那几格**——申报测量（[02](02-billable-weight-actual-measurement-and-declared-dimensions-read-port.md) 口）与地址要素（03 口，PS CONTEXT「地址要素」词条：邮编、国家 / 地区码）——不是客户原文整份。PS CONTEXT Rules「合成日志和证据索引只保留作用域引用、对象引用……不记录客户原文」那条纪律不因本票松动。

## 缺口（取证于分支 `mcp5-ppseams02-03`，逐符号名；两条都是 02 / 03 作者开工时量到、与两票裁决字面不符之处）

**一、提交版本不携带寄收件条目——地址要素在基线上也无物可读：**

- `git grep -n -E 'Scope\s+\[\]CanonicalContentEntry' -- internal/parcelshipment/domain/payload_canonicalization.go` → `SubmissionPayloadSpec.Scope` 是寄收件范围的 name/value 条目列表，它只被 `CanonicalizeSubmissionPayload` 编进 `PayloadDigest`。
- `git grep -n -E 'type versionDocument struct' -A6 -- internal/parcelshipment/adapters/postgres/shipment_request.go` → 提交版本落库的文档只有版本标识、来源指纹（含摘要）、成员、画像、时间；**没有条目**。`source_submission` 表（`migrations/parcel_shipment/0001_source_submission.sql`）只存 `payload_digest`。
- `git grep -n -E 'DeclaredProfiles|PayloadDigest' -- internal/parcelshipment/application/submit_shipment_request.go` → 命令收画像与摘要，**不收条目**；`adapters/http/isolated_write_intake.go` 的 `scopeEntries` 算完摘要即丢。
- 后果：03 的 `SubmissionVersion.addressElements`（`domain/address_elements_resolution.go`）今天恒答缺席，`ports.AddressElementsView` 对全部快照两段答 `NOT_PROVIDED`；`domain.AddressElementsOf`（按要素名读值的纯函数）无生产调用方，登在 `internal/architecture/production_wiring_baseline.txt` 上等本票。

**二、客户原始资料版本只留痕不留内容——两口的已采用版本锚一格只能交锚：**

- `git grep -n -E 'type CustomerSourceDataVersionSpec struct' -A12 -- internal/parcelshipment/domain/customer_source_data.go` → 版本收 `Request SourceSubmissionFingerprint`（身份 + `PayloadDigest` + 两时）与留痕清单（范围、基准、意图、原因、请求方、决定方、授权快照、适用 / 形成时间）；**没有那一版报了什么**。
- `git grep -n -E 'PayloadDigest\s+domain.PayloadDigest' -- internal/parcelshipment/application/amend_customer_source_data.go` → 修订命令同样只带摘要。`customer_source_data_version.snapshot`（`migrations/parcel_shipment/0003_customer_source_data.sql`）存的是这份留痕清单。
- 后果：`DeclaredMeasurementAnchoredOnAdoptedVersion` 与 `AddressElementsAnchoredOnAdoptedVersion` 两格只交锚不交值（两型头注写了为什么：交基线值等于把已被客户更正的申报当现行申报送出去）。PS CONTEXT「收件地点引用」词条许诺「旧锚永远解析到那一版内容」——版本今天没有内容可解析，这句在代码上不成立。

## 语言从哪里来

- PS `CONTEXT.md`「客户原始资料版本」：「针对明确委托、包裹、字段或资料范围形成的不可覆盖客户来源版本。版本保留**原始请求**、基础版本、补充/更正原因、对象范围、来源、业务时间和适用关系」——「保留原始请求」今天落成保留请求的**指纹**。
- PS `CONTEXT.md` Rules「客户寄收件、货物和申报原始资料在接受时形成快照。后续更正形成新版本」；「收件地点引用……旧锚永远解析到那一版内容」。
- PS `CONTEXT.md`「地址要素」（03 落的词条）：「要素的版本就是客户原始资料版本……要素在那一版上缺席即如实答缺」。
- PS `CONTEXT.md`「规范化业务内容摘要（`PayloadDigest`）」：摘要覆盖成员、范围、基础版本、服务要求与 `requestEffectiveAt`——本票不改它的算法（03 裁决 1 / 7）。

## 做法候选（两条以内，不选）

1. **只留封闭集、进既有 jsonb 快照**：提交版本从 `Scope` 条目里按 `AddressElementEntryName` 挑出寄 / 收两范围的封闭要素作规范化文档的一个子段，随 `shipment_request.snapshot` 的 `versionDocument` 落库（画像已是先例）；客户原始资料版本同法，在 `customer_source_data_version.snapshot` 里带上那一版的测量 / 地址要素子段；其余条目照旧只进摘要。不加列不加迁移；旧快照缺子段如实答缺。代价：修订版本的内容要由修订命令带进来——`AmendCustomerSourceDataCommand` 今天只带摘要，接单入口（隔离形态与将来的真渠道 Intake）都要把内容也交出来。
2. **内容另立表**：新迁移 `parcel_shipment/00NN` 一张「资料范围内容版本」表，键（租户、委托、范围、版本锚），值封闭要素子段；提交与修订两条路各写一行。代价：多一张表与一道迁移，两口读值多一跳；好处是内容与聚合快照分离、可按锚直接索引。

## 红线

- 只留有消费方的封闭要素，不留客户原文整份；日志与证据索引纪律不动。
- `PayloadDigest` 算法与既有摘要用例零改（03 裁决 1 / 7）；`0003` 不改，迁移只新序号。
- 两口（`DeclaredMeasurementView` / `AddressElementsView`）的形与封闭答格不动：本票让「已采用版本锚」一格开始带值、让 03 的基线格开始带值，格名与消费方契约不变。
- 修订版本的内容仍由客户声明、本上下文不替客户改值；节点实测、正式申报不覆盖它。
- 真实邮编 / 真实申报属实例半边；夹具全 `SYN-`。

## 完成判据（待裁后写实；可 grep）

1. 提交版本携带地址要素子段：接单带 `DELIVERY_PLACE.POSTAL_CODE` 一类条目 → `ports.AddressElementsView` 目的段答 `ANCHORED_ON_BASELINE` 带值；接单不带 → `NOT_PROVIDED`；旧形快照（夹具直写）→ `NOT_PROVIDED`；`domain.AddressElementsOf` 从 `production_wiring_baseline.txt` 出名单。
2. 客户原始资料版本携带内容：测量 / 地址要素范围上的修订被采用后，两口的已采用版本格 `Measurement()` / `Elements()` 第二返回值为真且值是那一版的；`待复核` 仍不给值。
3. 既有摘要用例（`payload_canonicalization_test.go`）零改；`internal/architecture` 门禁绿；`internal/parcelpricing/**` 零 diff。
4. `docs/product/MECHANISM-INVENTORY.md` 干净检出重生成（若走做法 2：迁移 +1）。

## 地盘

`internal/parcelshipment/{domain,ports,application,adapters/postgres,adapters/http}`、`migrations/parcel_shipment/`（仅做法 2，新序号）；`docs/domain/parcel-shipment/CONTEXT.md`（「客户原始资料版本」词条若要把「保留原始请求」写实成「保留封闭要素内容」）。不动 `internal/parcelpricing/**`、`internal/nodeoperations/**`、`internal/transportfulfillment/**`。

## 要裁的（全归 PS owner）

1. **留哪些内容**——只留 02 / 03 两口消费的封闭要素（测量、邮编、国家 / 地区码），还是把寄收件两范围的全部条目留成内容？前者是「有消费方才留」，后者会把客户原文整份落进快照，与日志 / 证据索引的纪律要对齐。
2. **留在哪里**——做法 1（既有 jsonb 快照的子段，零迁移）还是做法 2（另立内容版本表）。判据是「按锚直接读」的读法与聚合快照的体量。
3. **修订版本的内容从哪里来**——`AmendCustomerSourceDataCommand` 今天只带摘要；要带内容，接单入口（隔离形态 `isolated_write_intake.go` 与将来的真渠道 Intake）要把 `SubmissionPayloadSpec` 的条目交出来。摘要算法不动是定的（03 裁决 1 / 7），但「内容与摘要必须由同一份规范化输入产出」要不要成为构造门（防止内容与摘要各说各话）。
4. **CONTEXT「客户原始资料版本」词条「保留原始请求」那句**——是改成「保留请求指纹与封闭要素内容」，还是让「原始请求」继续指指纹、内容另立一句。

## 裁决（2026-09-15 12:5x 通道 1 按用户「代裁」代裁，PS owner 口径；依据是本票两条缺口的取证、02 / 03 完成记录与判断项、06 评审残差，钉 `e1ab9fb5`）

1. **要裁的 1——只留封闭要素，不留原文。** 内容 = 该资料范围上**由本上下文定名的封闭要素**：测量范围留 `DeclaredMeasurement`（重量 + 可缺席尺寸，02 已定形），寄 / 收两范围留 `AddressElements`（`POSTAL_CODE` / `COUNTRY_CODE`，03 已定形）；其余 name/value 条目照旧只进 `PayloadDigest`。**为什么不留整份**：CONTEXT Rules「不记录客户原文」那条纪律写的是日志与证据索引，快照不在字面内，但把寄收件条目整份落进快照等于让快照替代来源系统当原文库——机制半边没有任何消费方读它，实例半边（租户字段映到要素名）却要靠它长；「有消费方才留」让每多留一格都要先有词条，与 03 裁决 1「PS 定要素名的封闭集」同一把尺。封闭集要扩只走 CONTEXT「地址要素」词条加要素名，不走「顺手多留」。
2. **要裁的 2——做法 1：两份既有 jsonb 快照各加一个可缺席的内容子段，零迁移。** 提交版本 `versionDocument` 加 `elements`（按资料范围分的封闭要素 name/value，寄 / 收各一段，缺席即 `omitempty`）——画像 `Profiles` 已是「内容随提交版本进快照」的先例（ADR-0048）；客户原始资料版本 `sourceDataVersionDocument` 加 `content`（按该版本的 `DataGroup` 只带一种：测量范围带 `profileDocument` 同形的测量，寄 / 收范围带要素 name/value）。**为什么不另立表**：两口今天的读法是「先解析锚、再从锚指的那一版读内容」（`AddressElementsFor` / `addressElementsInGroup`、`DeclaredMeasurementAnchoredOnAdoptedVersion`），锚已经是版本身份，「按锚直接读」在 jsonb 上就是读那一行的子段，另立表多一跳、多一道迁移却不多任何一种读法；内容体量是几个要素，不是聚合快照的负担。**重开条件**：封闭集长到一只手数不过来、或出现「按要素值反查委托」的消费方——那时再立表，本票不预留。旧形快照缺子段 → 如实答缺（判据 1 第三句），不回填。
3. **要裁的 3——内容与摘要由同一次规范化产出；命令两样一起带；应用层不重算，也不设领域构造门。** `domain.CanonicalizeSubmissionPayload` 扩成一次调用交回**摘要 + 内容**（返回一个成对的结果类型，摘要算法零改、既有用例零改——加返回值不改哈希），寄 / 收要素由既有 `AddressElementsOf` 从同一份 `entries` 挑出、测量由既有画像路。接单入口（隔离形态 `isolated_write_intake.go` `scopeEntries` 处，将来真渠道 Intake 同位）**只调这一次**，把两样一起放进命令：`SubmitShipmentRequestCommand` 加 `DeclaredElements`（与 `DeclaredProfiles` 并列、同「允许缺席或部分覆盖」口径）；`AmendCustomerSourceDataCommand` 加 `Content`（按 `Scope` 的范围只允许一种形：测量范围一份 `DeclaredMeasurement`、寄 / 收范围一份 `AddressElements`，形与范围不配 → `未受理`；`Intent` 为显式清空时内容必须为空，非空 → `未受理`——清空就是「那一版上要素缺席」，两口的已采用格随之如实答缺）。**为什么不做领域构造门**：领域对象要验「内容与摘要出自同一份输入」就得拿到整份载荷重算摘要，与要裁的 1 相抵；结构上只留一条产出路径（唯一调用点 + 成对返回）比事后校验更守得住，接单入口用例断言「同一份 `SubmissionPayloadSpec` 一次调用得到的摘要与内容一起进了命令」。真渠道 Intake（PAR-INT-01）落地时照同一条路，本票不替它写。
4. **要裁的 4——「保留原始请求」仍指请求指纹，内容另立一句。** 「原始请求」在 ADR-0014 与 `SourceSubmissionFingerprint`（身份 + 摘要 + 两时）上已是「请求身份」的读法，改它会让引用这个词的地方一起换义；在词条末补一句：「版本另保留该资料范围上由本上下文定名的封闭要素内容（申报测量、地址要素），持锚方按锚解析到那一版内容；其余条目只进摘要，不留原文。」「收件地点引用」词条「旧锚永远解析到那一版内容」由此在封闭要素上成立，不改字。
5. **顺带收进本票（06 评审残差，PS owner 口径）**：`AddressElements` 头注「AddressElementsOf 读在场只认这一种缺席」收成「值层面只认这一种缺席」（同名矛盾条目那一路缺席由 `AddressElementsOf` 自己的头注讲）；`AddressElementsOutcome` 头注「所以对今天全部快照口都答 NOT_PROVIDED」随本票落地删掉「今天恒缺席」的整句——它不再为真。
6. **做法写实**：(1) `domain`：成对返回类型 + `DeclaredElements`（寄 / 收两段 `AddressElements`）+ `SourceDataVersionContent`（按范围一种形，构造门如裁决 3）；`CustomerSourceDataVersionSpec` 加 `Content`；`SubmissionVersion.addressElements` 从版本自己的要素子段读（删「今天恒为缺席」头注）；`DeclaredMeasurementAnchoredOnAdoptedVersion` / `AddressElementsAnchoredOnAdoptedVersion` 两格开始带值（第二返回值为真），`待复核` 仍不给值。(2) `application`：两条命令加字段，`SubmitShipmentRequest` 把要素随版本落、`AmendCustomerSourceData` 把内容随版本落，不重算摘要。(3) `adapters/postgres`：两份文档各加子段，`omitempty`；读回缺子段即零值。(4) `adapters/http` 隔离形态：`scopeEntries` 处改为一次调用取成对结果。(5) `internal/architecture/production_wiring_baseline.txt`：`AddressElementsOf` 那一行随生产调用点出现而出名单。(6) CONTEXT 词条一句（裁决 4）。**不动** `PayloadDigest` 算法、`0003`、两口的形与格名、`internal/parcelpricing/**`。
7. **完成判据写实**（替换上方「待裁后写实」四条）：(1) 接单带 `DELIVERY_PLACE.POSTAL_CODE` 一类条目 → `ports.AddressElementsView` 目的段 `ANCHORED_ON_BASELINE` 带值；不带 → `NOT_PROVIDED`；夹具直写旧形快照 → `NOT_PROVIDED`；`AddressElementsOf` 从接线基线出名单（`internal/architecture` 门禁绿）。(2) 测量范围与收件范围各一条修订被采用后，`DeclaredMeasurementView` / `AddressElementsView` 已采用格第二返回值为真且值是那一版的；显式清空的版本被采用后已采用格如实答缺；`待复核` 仍不给值；形与范围不配、清空带内容 → `未受理`。(3) `payload_canonicalization_test.go` 既有用例零改、摘要哈希零变（新用例只加）；接单入口用例断言摘要与内容出自同一次调用。(4) `internal/parcelpricing/**` 零 diff；迁移零新增；`docs/product/MECHANISM-INVENTORY.md` 在 tip 干净检出重生成（新增 domain 文件则生产 +N，作者预报）。(5) 带 DSN PS adapters/postgres + application + `cmd/parcel-api` PASS 非 SKIP。
8. **能力边界**：裁的是留什么、留在哪、从哪来、词条怎么写；具体不变式（`SourceDataVersionContent` 对「范围 × 形」的封闭组合、清空版本的内容零值怎么表达、`versionDocument.elements` 的 JSON 键名）归作者按代码定并写进判断项。读过：本票全文、02 / 03 完成记录与判断项、06 评审、`versionDocument` / `sourceDataVersionDocument` / `CustomerSourceDataVersionSpec` / 两条命令的结构体段、CONTEXT「客户原始资料版本」「地址要素」「收件地点引用」词条；**没读**：`isolated_write_intake.go` 正文、`amend_customer_source_data.go` 编排正文、ADR-0014 / 0048 正文、PAR-INT-01 交接。作者量到与代码不符，以代码为准并写进判断项，不回头等我。

## 参照

[02](02-billable-weight-actual-measurement-and-declared-dimensions-read-port.md) 完成记录判断项 ①（已采用版本锚只交锚的理由）；[03](03-ps-origin-destination-postal-route-read-port.md) 裁决 7（取 A、内容落库归本票）与完成记录；[spec](../spec.md)「边界（三票共用）」「不在本目录」；`internal/parcelshipment/domain/payload_canonicalization.go`（`SubmissionPayloadSpec.Scope`、`AddressElementsOf`）、`address_elements_resolution.go`（`SubmissionVersion.addressElements` 头注「是 pp-seams/05 接内容的那一处」）、`declared_measurement_resolution.go`（`DeclaredMeasurementAnchoredOnAdoptedVersion` 头注）、`customer_source_data.go`（`CustomerSourceDataVersionSpec`）；`internal/parcelshipment/adapters/postgres/shipment_request.go`（`versionDocument`）、`source_data_version.go`（`sourceDataVersionDocument`）；`internal/parcelshipment/application/submit_shipment_request.go` / `amend_customer_source_data.go`；`internal/parcelshipment/adapters/http/isolated_write_intake.go`（`scopeEntries`）；`internal/architecture/production_wiring_baseline.txt`（`AddressElementsOf` 那一行）；PS `CONTEXT.md`「客户原始资料版本」「地址要素」「规范化业务内容摘要」「收件地点引用」；ADR-0014（规范化版本）、ADR-0048（画像随版本）、ADR-0130（收件地点引用与锚）。

## Comments

- 2026-09-15 11:3x · 通道 5（task-9cc0b1a6）：立票，未动代码。两条缺口都是 02 / 03 开工时对 `3a21dab7` 量到的、裁决字面没预见的事实（02 裁决 3 / 5 与 03 裁决 1 都默认快照里有那一版的内容）；02 按裁决 7 以代码为准落成「已采用格只交锚」，03 经通道 1 10:3x 取 B → 10:4x 改 A（理由：基线与修订两半是同一道「版本留不留内容」的领域题，要一次定、不从一只读口里只开一半）。能力边界：读过 02 / 03 全文与两票实现、`payload_canonicalization.go` / `shipment_request.go` / `source_data_version.go` / `customer_source_data.go` / `submit_shipment_request.go` / `amend_customer_source_data.go` / `isolated_write_intake.go` 正文；**没读** 真渠道 Intake 的设计交接（PAR-INT-01），「要裁的」3 里接单入口那半靠 PS owner 与作者开工时量。
