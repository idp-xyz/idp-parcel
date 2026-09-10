# `parcel-shipment` 开一个按（租户，包裹身份）答「收件地点引用」的窄读口——`transport-fulfillment` 派送任务地点一格的提供方半边

Category: enhancement
Status: in-progress——11:14 通道 2 认领（单 task-0dab9f5a-ed5a-4407-ba1c-45d67d53350e），分支 `mcp2-psr06` 基 `84e89dc7`。此前 ready-for-agent——由 [ADR-0130](../../../docs/adr/0130-delivery-place-reference-is-a-shipment-level-composite-reference-anchored-to-a-source-data-version.md) Consequences 第一条拆出（2026-09-09，通道 2，task-ddb77473，PS owner 口径代裁）；形状已裁，本票只落读口、值对象与 postgres 读面，不动 `transport-fulfillment/**`
Blocked by: 无

## 缺口

ADR-0114 决定三让 TF 按 `DeliveryPlaceSource.LoadDeliveryPlace(tenant, object)` 拉收件地点引用；ADR-0130 定了引用是什么、按什么键答、`待复核`与「没有」怎么分。PS 今天没有任何一条路从包裹身份走到「收件地点引用」：`ports` 上没有读口，`domain` 里没有这个值对象，收件地址只在接受基线快照与客户原始资料版本里作为内容出现。票 [tf/12](../../tf-segment-lifecycle-closure/issues/12-delivery-place-reference-seam-parcel-shipment.md) 的 TF 适配器等它。

## 要落的（形状照 ADR-0130，取舍已裁）

1. **值对象 `domain.DeliveryPlaceReference`**：四段——租户、委托、收件资料范围（`SourceDataScope` 形，委托必填、包裹可空、资料组）、资料版本锚（`SourceDataBasis` 形：接受基线，或某份 `SourceDataVersionID`）。`String()` 交规范串，串自带形状版本前缀（先例 `PSC-1:` / `PCC-1:`，取新号如 `DPR-1:`），四段之外不多一字，地址内容一字不进串；带重建门（ADR-0028），非规范串拒不规范化。
2. **读口 `ports.DeliveryPlaceReferenceView`**（名可议，落在 `ports` 新文件）：按（租户，`DeclaredParcelID` 或本上下文对正式包裹身份的既有引用）答封闭四格——`基线锚引用` / `已采用版本锚引用` / `收件地点未定`（范围上 `CurrentSourceDataAdoption` 为 `待复核`）/ `没有收件地点`（对象不属任何已接受委托的成员集合，含集运单元与不可见对象，按统一不可见结果，不区分不存在、他租户、未授权）。前两格带 `DeliveryPlaceReference`，后两格不带。**没有`不知道`格**：四本册子都是 PS 自己的，答不出就是读面坏了，上抛 error。
3. **包裹 → 委托的路**：经 `AcceptanceBaseline.covers` 走声明成员；包裹身份谱系（拆分 / 合并后的新包裹）今天领域里没有模型，本票先只按声明成员解析，谱系包裹会落「没有收件地点」——在读口头注写明这一格是「谱系未建模」的今日形状，谱系落地那票要补这一路（同 ps-port-remainder/05 NO 半边「从未关联 → 不在」那条越权风险点的处置）。
4. **收件资料范围怎么指名**：`SourceDataGroupReference` 是开放引用（ADR-0120 决定七），「收件」这个资料组在 PS 里今天没有自有词。本票在 PS `domain` 立一个自有常量作收件资料组的原词（机制半边——引用里要能写出这一段），矩阵登记侧（`PAR-COM-13`）用什么词是实例半边，两侧对不上时读口按 PS 原词查、查不到修订版本就答基线锚——这是如实答案（PS 没见过那个范围上的修订）而不是默认。
5. **postgres 读面**：一条按（租户，包裹）的查询——基线成员表反查委托，再按（委托，收件范围）取资料版本与派生采用判断；派生规则复用 `CurrentSourceDataAdoption`，不在 SQL 里第二套实现。需不需要新迁移（索引）由实施时量，只加索引不加列。
6. **不做**：按引用解析回地址内容的读口（第二个消费方，另立）；按委托或按引用的反查方法；向 TF 推送任何事件；改 `ShipmentRequestRepository` 一类写口。

## 完成判据

1. `domain.DeliveryPlaceReference` 构造 / 重建 / `String()` 往返测试：同一委托两个包裹串相等；基线锚与版本锚串不等；非规范串重建被拒。
2. `ports` 读口四格各一例用例测试（替身实现）：基线锚、已采用版本锚、`待复核`不给引用、不属成员集合答没有；`待复核`那例用两条分叉修订构造（`CurrentSourceDataAdoption` 已有的分叉路）。
3. postgres 适配器真库测试（带 DSN）覆盖同四格；DSN 缺席 SKIP 不 PASS。
4. 不动 `transport-fulfillment/**`、不改 ADR-0130 / 0114 / 0075 正文、不写真实地址。
5. 验证（作者层）：gofmt 空、`go build ./...` / `go vet ./...` 0、`go test -count=1` PS `domain` + `ports` + `adapters/postgres`（带 DSN）+ `./internal/architecture/...`；清点在干净检出重生成。

## 参照

ADR-0130 决定一 / 二与 Consequences；PS CONTEXT「收件地点引用」词条与 Rules 那条；`internal/parcelshipment/domain` 的 `AcceptanceBaseline`、`SourceDataScope`、`SourceDataBasis`、`CurrentSourceDataAdoption`；`internal/transportfulfillment/ports/delivery_requirement.go` 的 `DeliveryPlaceSource`（消费方形状，只读不改）；本目录 05（按包裹键读面的先例与「没有不知道格」的判据）。

## Comments

- 2026-09-09 · 通道 2（task-ddb77473）：立票（ready-for-agent）。形状在 ADR-0130，本票不再裁；ADR-0130 越权风险点 2（「没有」藏完整性问题）与 4（基线上已有值的首次更正走哪一格）若 owner 复核后改口径，本票随之改读口那一格，不回改 ADR。
