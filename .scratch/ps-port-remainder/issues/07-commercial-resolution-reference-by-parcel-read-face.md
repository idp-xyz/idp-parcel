# `parcel-shipment` 开一个按（租户，包裹身份）答「商业解析回指」的窄读口——`transport-fulfillment` 交付条件缝里对象走到合同的那一跳

Category: enhancement
Status: ready-for-agent——由 [ADR-0133](../../../docs/adr/0133-delivery-condition-reference-is-the-acceptance-time-commercial-resolution-reference.md) 决定二与 Consequences 第一条拆出（2026-09-09，通道 2，task-f5521768，PS owner 口径代裁）；形状已裁，本票只落读口与 postgres 读面，不动 `transport-fulfillment/**` 与 `partycommercial/**`
Blocked by: 无（建议排在 [06](06-delivery-place-reference-read-face.md) 之后做，复用它的包裹 → 委托解析；不阻塞）

## 缺口

ADR-0133 决定一把交付条件引用定为委托接受时固定的商业解析回指（接受决定上的 `CommercialResolutionID`），决定二把「对象怎么走到它」判给 PS：按（租户，包裹身份）答。今天 PS 从包裹身份走到回指的路只有 ADR-0062 那条——`SourceIdentity` → 取回已接受委托 → `AcceptanceDecision().Basis().ResolutionID()`——键是来源身份不是包裹；`adapters/partycommercial/adopted_stage_owner.go` 用的就是它。TF 手里只有 `CarriedObjectReference`，没有来源身份。票 [tf/14](../../tf-segment-lifecycle-closure/issues/14-delivery-condition-reference-seam-party-commercial.md) 的适配器等这一口。

## 要落的（形状照 ADR-0133 决定二，取舍已裁）

1. **读口 `ports.CommercialResolutionReferenceView`**（名可议，`ports` 新文件）：按（租户，`DeclaredParcelID` 或本上下文对正式包裹身份的既有引用）答封闭三格——`回指`（委托`已接受`且接受决定带 `CommercialResolutionID`，交回它）/ `没有`（对象不属任何已接受委托的成员集合，含集运单元与不可见对象，按统一不可见结果，不区分不存在、他租户、未授权）/ error（委托`已接受`而接受决定缺回指——接受流的装配缺陷，判据同 ADR-0062 决定三「闭包在场却未采用规则包 → error」；读面坏了也是 error）。**没有`不知道`格**：册子都是 PS 自己的。
2. **包裹 → 委托的路**：与 06 同一条——经 `AcceptanceBaseline.covers` 走声明成员；身份谱系未建模，谱系包裹今天落「没有」，读口头注写明这一格是「谱系未建模」的今日形状（同 06 第 3 条与 ps-port-remainder/05 NO 半边的处置）。两票若同人同期做，内部解析可同包同文件，**端口分开一口一问**——ADR-0130 与 0133 都写了「读口不预设按委托或按引用反查的方法」，合成一个宽口就是在预设。
3. **回指的形**：交回的就是 `domain.CommercialResolutionID`（`String()` 不透明串），不拆、不拼合同版本——「对象/版本」两段式只许 PC 一处拼（ADR-0080 决定七）；租户由调用方给、读面按（租户，包裹）取，回指不是能力凭证（ADR-0062 决定三 / ADR-0003）。
4. **postgres 读面**：一条按（租户，包裹）的查询——基线成员表反查委托，再取其接受决定上的回指列（今天已随接受决定快照落库，`CommercialResolutionKeyStore` 与接受决定的 postgres 形状不改）。需不需要新迁移（索引）由实施时量，只加索引不加列。
5. **不做**：按回指反查包裹或委托；替 PC 解闭包（那是 tf/14 适配器的第二只依赖，走 PC 读口）；向 TF 推送任何事件；改 `ShipmentRequestRepository` 一类写口。

## 完成判据

1. `ports` 读口三格各一例用例测试（替身实现）：已接受带回指 → 回指；未接受 / 不属成员集合 / 集运单元引用 → 没有；已接受缺回指 → error。
2. postgres 适配器真库测试（带 DSN）覆盖前两格 + 同一委托两个包裹交回同一个回指；DSN 缺席 SKIP 不 PASS。
3. 不动 `transport-fulfillment/**`、`partycommercial/**`，不改 ADR-0133 / 0130 / 0062 正文，不写真实合同。
4. 验证（作者层）：gofmt 空、`go build ./...` / `go vet ./...` 0、`go test -count=1` PS `ports` + `adapters/postgres`（带 DSN）+ `./internal/architecture/...`；清点在干净检出重生成。

## 参照

ADR-0133 决定二与 Consequences；ADR-0130 决定二（同一句键判据）；ADR-0062 决定一 / 三（回指的既有路与分格判据）；`internal/parcelshipment/domain/acceptance_basis.go` 的 `CommercialResolutionID`；`internal/parcelshipment/adapters/partycommercial/adopted_stage_owner.go`（按来源身份回指的既有实现，本票是它的包裹键姊妹）；本目录 05、06。

## Comments

- 2026-09-09 · 通道 2（task-f5521768）：立票（ready-for-agent）。形状在 ADR-0133，本票不再裁；ADR-0133 越权风险点 5（谱系包裹与集运单元同答没有）若 owner 复核后改口径，本票随之改那一格，不回改 ADR。
