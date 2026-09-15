# pp-seams/01 与 03 评审非阻断尾巴：01 两处「三件」计数、成员顺序未写进口契约；03 `AddressElements` 头注「不去空白」与 `AddressElementsOf` 的 `TrimSpace` 不同口、`AddressElementsView` 头注「全部快照答缺席」过宽

Category: chore
Status: ready-for-agent——2026-09-15 11:2x 通道 1 立票（01 评审 ← 通道 6 Standards ① + Spec ①；03 评审 ← 通道 6 Standards ① + Spec ①；推送方处置「合一张 A 类尾巴」）。除 03 那一处读值分支外零行为
Blocked by: 无（01 进 main `1e74aaaf`，03 进 main `a19ac630`）

## 缺口（评审各钉 `71dd8313` / `2a637107`；进 main 后在 `c88b5e35` / `1bd50208` 同形）

**01（TF）**

1. `internal/transportfulfillment/ports/charge_occurrence_member_view.go` `ChargeOccurrenceMembers` 头注「CONTEXT『运输收费发生项』固定下来的三件」与 `ChargeOccurrenceMemberView` 头注「协议、数量与修订三件」——都已点名，「三件」是冗余的数（AGENTS「改文档」：数别处的东西）。去掉两个「三件」。
2. 成员切片的顺序：实现 `ChargeOccurrences.LoadMembers` `ORDER BY member.object_ref`，与 `FindByKey` 的 `loadMembers` 同序；对方首条用例 `TestChargeOccurrenceMemberViewAnswersMembersBusinessTimeAndScopeByKey` 断了字面顺序，而口头注没说切片有无顺序保证——断言钉的是实现细节。评审建议**写进契约**而不是删断言：PP 消费侧要拿成员清单进计价输入快照的指纹，确定序是它的需要。做法：`ChargeOccurrenceMemberView` 头注补「成员按引用字面升序，与 `FindByKey` 同序」一句；那条断言随之成契约断言，不动。

**03（PS）**

3. `internal/parcelshipment/domain/address_element.go` `AddressElements` 头注「客户给的串本上下文不去空白，一个全是空白的值与『没报』得分得开」；而 `internal/parcelshipment/domain/payload_canonicalization.go` `AddressElementsOf` 用 `strings.TrimSpace(entry.Value()) != ""` 判在场——全空白的值读作缺席，与「没报」分不开；`CanonicalContentEntry` 定的「显式清空」是值为空串（值保真不 trim），`AddressElementsOf` 头注引它却把空白串也归了进去。**三处要同口。** 本票取「以 `CanonicalContentEntry` 的定义为准」：`AddressElementsOf` 只把 `entry.Value() == ""` 读作显式清空 / 缺席，全空白的串原样在场（PS CONTEXT「地址要素」词条「本上下文只保存客户给的串，不校验、不规范化」——为判在场而 trim 也是一次规范化）；头注两处随之改口。今天值恒缺席（pp-seams/05 未落），零运行时影响；单元例 `TestAnExplicitlyClearedOrDuplicatedAddressElementReadsAsAbsent` 若含全空白一格要随之改口，`TestAddressElementsAreReadByClosedEntryNameOnly`「值原样不去空白」加一格全空白在场。**若 PS owner 认为全空白该读作缺席**，改法是只改 `AddressElements` 头注那半句并让 `AddressElementsOf` 头注写明「空白串同显式清空」——两路都行，本票默认前者，评审可翻。
4. `internal/parcelshipment/ports/address_elements_view.go` 头注「今天对全部快照两段都答『要素缺席』」——实现 `addressElementsInGroup` 先锚后内容：`已采用`答已采用版本锚、`待复核`答未定、非成员答无，只有锚在基线上才答「要素缺席」（适配器头注与 `AddressElementsOutcome` 头注写对了）。改为「锚在基线上的那一格今天恒答要素缺席」。

## 红线

- 01 两处、03 第 4 处只注释；03 第 3 处若取默认改法，行为差只在「全空白值」一格，今天无数据到得了它；不动 `PayloadDigest`、不动迁移、不动 `application` / `adapters/http`。
- 注释中文；不写行号、不数别处。
- `internal/parcelpricing/**` 零 diff。

## 完成判据

1. 01：`git grep -n '三件' -- internal/transportfulfillment/ports/charge_occurrence_member_view.go` 零命中；口头注含「按引用字面升序」一句。
2. 03：`AddressElements` / `AddressElementsOf` / `AddressElementsView` 三处头注同口；`AddressElementsOf` 的在场判定与头注一致；`go test ./internal/parcelshipment/domain/` ok（含改口 / 新增那一格）。
3. `gofmt -l` 空；`go vet ./internal/transportfulfillment/... ./internal/parcelshipment/...` 0；不带 DSN 两上下文全部包 ok；`internal/parcelpricing/**` 零 diff。
4. 完成记录同笔；清点零差（不增删文件）。

## 地盘

`internal/transportfulfillment/ports/charge_occurrence_member_view.go`（只注释）；`internal/parcelshipment/domain/address_element.go` / `payload_canonicalization.go`（`AddressElementsOf` 一处判定 + 头注）/ `address_element_test.go`；`internal/parcelshipment/ports/address_elements_view.go`（只注释）。撞点：无在途分支碰这些文件（pp-seams/05 未开工）。

## 参照

[01](01-tf-charge-occurrence-member-object-read-view.md) Comments「评审 ← 通道 6」Standards ① / Spec ① 与「进 main 记录」；[03](03-ps-origin-destination-postal-route-read-port.md) Comments「评审 ← 通道 6」Standards ① / Spec ① 与「进 main 记录」；[05](05-ps-submission-and-source-data-versions-carry-content.md)「要裁的」（「显式清空」要不要单独成格归 PS owner——本票不预裁那一格，只让三处头注与实现同口）；PS `CONTEXT.md`「地址要素」；`internal/parcelshipment/domain/payload_canonicalization.go` `CanonicalContentEntry` 头注（「显式清空」的定义）；AGENTS「写代码注释」「改文档」。

## Comments

- 2026-09-15 11:2x · 通道 1：立票（两票评审尾巴合一张，推送方处置时点名）。只写票面，未动代码。
