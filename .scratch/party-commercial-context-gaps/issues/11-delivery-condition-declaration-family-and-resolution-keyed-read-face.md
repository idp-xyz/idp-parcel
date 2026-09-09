# 「交付条件」是服务产品版本声明、客户合同版本只能收紧的商业条件——PC 今天没有这一族，也没有按商业解析回指答「有没有交付条件」的读口

Category: enhancement
Status: ready-for-agent——由 [ADR-0133](../../../docs/adr/0133-delivery-condition-reference-is-the-acceptance-time-commercial-resolution-reference.md) 决定四与 Consequences 第二条拆出（2026-09-09，通道 2，task-f5521768，PC owner 口径代裁）；归属与对外形状已裁（越权风险点 2 单列供 owner 复核），声明的字段形状、发布通道与批文由本票按下面「要建什么」落，不再裁归属
Blocked by: 无

## 从哪里来

ADR-0114 决定三让 TF 按 `DeliveryConditionSource` 拉「交付条件引用」；ADR-0133 定了引用是委托接受时固定的商业解析回指，PC 按（租户，回指）答「交付条件引用 / 没有交付条件 / error」。今天 PC 里没有任何一个叫「交付条件」的对象：终局规则族（ADR-0058）说的是责任结果怎么算终局，合同委派（ADR-0116）说的是谁能替谁决定，客户服务规则版本（ADR-0104）说的是披露 / 响应 / 索赔——**没有一处说「这份服务允许哪些交付方式、要什么交付证明」**，而 TF CONTEXT 硬句「安全投放、智能柜、邻居代收、本人签收和其他交付方式只能在适用服务产品、国家地区和客户合同允许且证据满足规则时形成有效交付」预设了它在 PC。票 [tf/14](../../tf-segment-lifecycle-closure/issues/14-delivery-condition-reference-seam-party-commercial.md) 的适配器等这一口。

取证于 `main = dec37d78`（逐符号名）：PC `ports.CommercialResolutionView.LoadResolution` 按（租户，回指）交回闭包，闭包 `AdoptedFor(kind)` 能取已采用的服务产品版本与客户合同版本；`PublicationRegistry` 具名 Save 一族与 `application.DeclarationChannel` 封闭集里没有交付条件那一格；`migrations/party_commercial/` 最大序号 `0028`（`0029` 由 wbr/10 在途）；PC CONTEXT「保价条件」词条同样是「产品声明、合同收紧」的形，但那一族今天也没有表——本票是这个形的第一份落地，保价条件日后照抄。

## 要建什么（形状建议，字段取舍归本票作者；归属与读口三格已裁不改）

1. **声明族「交付条件」**，拥有对象两层：**服务产品版本**（`object_kind` = 服务产品版本那一格）声明产品级条件；**客户合同版本**（`object_kind = 2`）可以声明收紧。形照 `0007` / `0012` / `0027`：随版本发布登记（「声明只能随发布」），更正走新版本，不开行级改写。正文至少三格：允许的交付方式集合（封闭集？开放引用？——**先裁**：交付方式词今天散在 TF CONTEXT 那句硬句里，PC 只登引用不登词表则与 `PAR-NET-09` 实例半边一致；若作封闭集要先在 CONTEXT 立词）、收件范围规则引用、交付证明规则引用；合同层只许出现产品层已有的方式（收紧判据同保价条件「只能在其内收紧」），构造门拒放宽。**没有默认**：两层都缺席就是没有交付条件；不内置「本人签收」。
2. **读口 `pcports.DeliveryConditionView`**（名可议）：按（租户，`ResolutionID`）答封闭四格——`交付条件引用`（闭包在场、采用了客户合同版本与服务产品版本、两层至少一层有声明；交回的引用就是这个回指，不另造）/ `没有交付条件`（闭包在场、两者都无声明——商业责任方去登）/ error：闭包不在场（`LoadResolution` `found=false` 对一份已接受委托的回指是提供方缺数据，ADR-0080 决定四同一判据：不冒充「没登」）/ error：闭包未采用客户合同版本（ADR-0062 决定三同一判据）。**只答有没有，不交内容**——内容读口（一线作业端按引用取允许集与证据规则）是第二个消费方，UC-TF-006 步骤 5 的实施票另立。
3. **写口**：`PublicationRegistry.SaveDeliveryConditions(ctx, content)` 具名 Save，先读回再判重放 / 冲突（同 `SaveSourceDataAmendmentAllowance` 的纪律）；发布用例多一条 `DeclarationChannel`；受控批文 `declarations` 多一节；三处 `PublicationRegistry` 替身跟随；`cmd/parcel-commercial` 的 `declarationsDocument` 多一节。
4. **迁移一份**：序号以开工那刻 `party_commercial` 最大序号 + 1 取；父子两表或一表带层级列由作者定，`object_kind` 两值进 CHECK。
5. **CONTEXT**：「交付条件」词条与 Rules 一句已随 ADR-0133 同笔进 PC CONTEXT；本票若把交付方式立成封闭集，词条要补那一句并在 GLOSSARY 加行。管理台表单（admin-write-faces）不在本票，另立。

## 红线

- 不为任何租户拟一条交付方式、收件范围或证据规则的取值（`PAR-NET-09` / `PAR-COM-05` / `PAR-COM-06`）；没登就是没有。
- 读口不交内容、不替 TF 判有效交付；PC 不读 PS、不认识包裹（对象 → 回指是 [ps-port-remainder/07](../../ps-port-remainder/issues/07-commercial-resolution-reference-by-parcel-read-face.md)）。
- 不改 ADR-0133 / 0058 / 0114 正文；归属若 owner 复核后改（ADR-0133 越权风险点 2），走 supersede。

## 完成判据

1. 领域构造门测试：产品层零声明拒 / 合同层出现产品层没有的方式拒（放宽）/ 合同层子集通过；拥有对象非对应种类或未生效拒。
2. 读口四格各一例（替身）；postgres 真库往返（带 DSN）覆盖「产品层有、合同层无」「两层都无 → 没有」「闭包不在场 → error」；DSN 缺席 SKIP 不 PASS。
3. 发布通道端到端一例（进程口批文 → 册上一版 → 读口答「交付条件引用」）。
4. 验证（作者层）：gofmt 空、`go build ./...` / `go vet ./...` 0、`go test -count=1` PC 四包 + `cmd/parcel-commercial`（带 DSN）+ `./internal/architecture/...`；清点在干净检出重生成。

## 参照

ADR-0133 决定一 / 二 / 四与越权风险点 2；ADR-0058（为何不归接单规则包版本）；ADR-0062 决定三、ADR-0080 决定四（两格 error 的判据）；ADR-0120（声明族落地的完整先例：表族 / 读口 / 具名 Save / 通道 / 批文）；PC CONTEXT「交付条件」「保价条件」词条；`internal/partycommercial/ports/ports.go` 的 `CommercialResolutionView` / `PublicationRegistry`；TF CONTEXT 交付方式那句硬句；UC-TF-006 输入契约「派送要求」行与步骤 5。

## Comments

- 2026-09-09 · 通道 2（task-f5521768）：立票（ready-for-agent）。本目录 spec.md「四票一览」表自 05 起未再扩行（05–10 只在 Status 行点名），本票照旧不改表。ADR-0133 越权风险点 2（归属）与 3（闭包不在场答 error 而非未配置）若 owner 复核后改口径，本票随之改拥有对象 / 读口那一格，不回改 ADR。
