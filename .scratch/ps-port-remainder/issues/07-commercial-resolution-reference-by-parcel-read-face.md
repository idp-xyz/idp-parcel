# `parcel-shipment` 开一个按（租户，包裹身份）答「商业解析回指」的窄读口——`transport-fulfillment` 交付条件缝里对象走到合同的那一跳

Category: enhancement
Status: resolved——2026-09-10 13:0x 通道 2（单 task-422c077d 派单「接着做 07」半边），分支 `mcp2-psr07` 基 `c2a119c6`（= mcp2-psr06 tip，同链），tip 见 Comments 末条完成记录；待推送方派非作者评审后重放进 main。此前 in-progress（12:4x 通道 2 认领）、ready-for-agent——由 [ADR-0133](../../../docs/adr/0133-delivery-condition-reference-is-the-acceptance-time-commercial-resolution-reference.md) 决定二与 Consequences 第一条拆出（2026-09-09，通道 2，task-f5521768，PS owner 口径代裁）；形状已裁，本票只落读口与 postgres 读面，不动 `transport-fulfillment/**` 与 `partycommercial/**`
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
- 2026-09-10 13:0x · 通道 2（task-422c077d「接着做 07」半边）：**完成记录**，分支 `mcp2-psr07` 基 `c2a119c6`（mcp2-psr06 tip，同链），代码 tip `526f0cdf`（本笔票面在其上），每笔提交后已推 origin。
  - **每笔**：`5018962b` 票面 in-progress；`18b628cf` domain——`ShipmentRequest.CommercialResolutionReferenceFor(parcel) (CommercialResolutionID, bool, error)` 三格 + `ErrAcceptedWithoutCommercialResolution`，外部测试两格 + 同包内部测试钉 error 格；`1e8e2bc0` `ports/commercial_resolution_reference_view.go`（新）+ `adapters/postgres/accepted_request_by_parcel.go`（新，06/07 共用的取行 + 重建 helper `findAcceptedRequestCoveringParcel`）+ `adapters/postgres/commercial_resolution_reference_view{,_test}.go`（新）+ `adapters/postgres/delivery_place_reference_view.go`（本链 06 的文件，改为调 helper，行为不变）；`526f0cdf` 清点在 `1e8e2bc0` 干净 detached 检出重生成。
  - **读口形**（供 tf/14 对）：`ports.CommercialResolutionReferenceView.LoadCommercialResolutionReference(ctx, 租户, 声明包裹) (domain.CommercialResolutionID, bool, error)`——回指 =（id, true, nil）；没有 =（零值, false, nil）；error 三种来源：`domain.ErrAcceptedWithoutCommercialResolution`（已接受而决定缺回指）、`domain.ErrAmbiguousParcelTarget`（两份已接受同时声明一件包裹）、读面本身坏了。回指就是接受决定 `Basis().ResolutionID()`，`String()` 不透明串，不拆不拼。
  - **触及**：`internal/parcelshipment/domain/commercial_resolution_reference{,_test,_internal_test}.go`（新）；`internal/parcelshipment/ports/commercial_resolution_reference_view.go`（新）；`internal/parcelshipment/adapters/postgres/accepted_request_by_parcel.go`、`commercial_resolution_reference_view{,_test}.go`（新）、`delivery_place_reference_view.go`（同链 06 文件，抽 helper）；`docs/product/MECHANISM-INVENTORY.md`；本票面。**未碰**：`ports.go`、`adapters/postgres` 的 main 上既有文件、`transportfulfillment/**`、`partycommercial/**`（含 `adopted_stage_owner.go`，按来源身份那条路原样）、ADR-0133 / 0130 / 0062 正文、`CommercialResolutionKeyStore`、`cmd/*`、`migrations/`（不加迁移：回指随接受决定快照落库，取行走 0006 既有部分 GIN）、`internal/architecture/*_baseline.txt`（新方法是聚合方法不是导出工厂，`CommercialResolutionID` 早已可达——两道棘轮均不响）。
  - **验收对照（完成判据）**：1 △ 三格各一例在 domain 层对进程内实现：`回指`（已接受成员 → `RES-1` = 接受决定的解析标识，两成员同一个）、`没有`（非成员 / 仅已提交）、error（已接受缺回指——公开构造函数造不出这个状态：`Decide` 的依据快照必带解析标识、重建门拒已接受缺产物，故用同包内部测试直接摆状态钉住）；「集运单元引用」在 PS 读口上没有对应的键——本口收 `DeclaredParcelID`，载运对象是不是包裹由 tf/14 适配器在 `CarriedObjectReference` 上分，落到本口的非包裹身份按不可见结果答没有。`ports` 包本身无测试（全仓惯例，同 06 的判断题 a）。2 ✓ 真库：已接受带回指 → `RES-1` 且两成员同一个；从未声明 / 他租户 / 仅已提交 → 没有；另加两例票面没点名的 error：快照上决定的 `resolutionId` 被抹空 → error 不折没有；两份已接受同时声明 → `ErrAmbiguousParcelTarget`。无 DSN 实测 SKIP 不 PASS。3 ✓ 见「未碰」；夹具无真实合同（`RES-1`）。4 ✓ 见验证。
  - **验收对照（要落的）**：① ✓ 端口三格、无「不知道」、与 `DeliveryPlaceReferenceView` 分开一口一问。② ✓ 与 06 同一条包裹 → 委托的路（`baseline.covers`），谱系未建模写在端口头注与方法头注；postgres 侧内部解析同包共用 helper，端口仍分开。③ ✓ 交回的就是 `domain.CommercialResolutionID`。④ ✓ 一条按（租户，包裹）的查询（helper），回指从快照上的接受决定取，不加迁移。⑤ ✓ 未做反查、未替 PC 解闭包、未推送、未改写口。
  - **验证强度**（作者层，带 DSN `-p 1 -count=1 -v`）：gofmt 空；`go build ./...` / `go vet ./...` 0；PS `domain` + `adapters/postgres` + `./internal/architecture/...` + `cmd/{parcel-api,parcel-commercial,parcel-dispatch}` **PASS 1246 / SKIP 0 / FAIL 0**。未跑全量。清点在 `1e8e2bc0` 干净检出重生成、单独成笔。
  - **与 main 碰面干跑**（13:0x，`origin/main = 8bdab82e`，`ls-remote` 同；06 已在 main，其五个文件与 `c2a119c6` 逐字节同）：在 detached 临时树上把 `5018962b`、`18b628cf`、`1e8e2bc0` 依次 cherry-pick 到 `origin/main` **全部干净**；`526f0cdf`（清点）冲突——预期内，推送方跳过后在 tip 重生成（同 06 的处置）。`git merge-tree origin/main HEAD` 报的另两处冲突（06 票面、基线重放注）都是本分支携带的 06 时代簿记 vs main 上推送方的进 main 记录，不在 07 的四笔里。
  - **给评审的判断题**：(a) 判据 1「替身实现」落 domain 层 + postgres 而非 `ports` 包内（同 06 判断题 a，06 评审已认可）。(b) 06 的 postgres 读口被本票改为调共用 helper——行为不变（06 的七例真库用例在本 tip 仍全 PASS），反方是「动了上一票刚评过的文件」。(c) error 格用同包内部测试（`package domain`）钉——仓内先例只有 `operator_registration_wait_test.go`；反方是「内部测试绕过构造门」，正方是那一格本就只有绕过构造门才到得了，而它是票面点名要测的。(d) 返回形取 `(id, bool, error)` 而非另立封闭三格类型——仓内 `FindCurrentAcceptedByParcel` 等同形；反方是 06 用了 `DeliveryPlaceResolution`，两口形不一致——06 有四个业务格，这里两个业务格 + error，Go 惯用形恰好够。
  - **越权风险点**（不改 ADR，随票记）：ADR-0133 越权风险点 5（谱系包裹与集运单元同答没有）原样；本票不新增。
