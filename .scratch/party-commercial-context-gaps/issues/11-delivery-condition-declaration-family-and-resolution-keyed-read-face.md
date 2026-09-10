# 「交付条件」是服务产品版本声明、客户合同版本只能收紧的商业条件——PC 今天没有这一族，也没有按商业解析回指答「有没有交付条件」的读口

Category: enhancement
Status: resolved——2026-09-10 13:0x 通道 5 收口（单 task-4649070b-bec3-433b-ba5c-58fb8b4ac4b7，改派自通道 4；分支 `mcp5-pcgaps11` 基 `d9a1f571`，每笔已推 origin；完成记录见文末，进 main 记录归推送方）。此前 in-progress——10:5x 通道 4 认领（单 task-8359632c），分支 `mcp4-pcgaps11` 基 `84e89dc7`；按「要建什么」1→5 逐笔接。此前 ready-for-agent——由 [ADR-0133](../../../docs/adr/0133-delivery-condition-reference-is-the-acceptance-time-commercial-resolution-reference.md) 决定四与 Consequences 第二条拆出（2026-09-09，通道 2，task-f5521768，PC owner 口径代裁）；归属与对外形状已裁（越权风险点 2 单列供 owner 复核），声明的字段形状、发布通道与批文由本票按下面「要建什么」落，不再裁归属
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

## 完成记录（2026-09-10 13:0x，通道 5；分支 `mcp5-pcgaps11` 基 `d9a1f571`（= 通道 4 的票面 in-progress 一笔 `84e89dc7` + 1），每笔已推 origin 的 SHA——推送方重放进 main）

| 笔 | SHA | 内容 |
|---|---|---|
| ① | `582e9f12` | 领域层：`DeliveryConditionContent` 两层同形（`DeclareProductDeliveryConditions` / `DeclareContractDeliveryConditions` + `TightenedProductVersion`）、正文三格 `DeliveryConditionTerms`（`DeliveryMethodReference` / `DeliveryRuleReference` 全开放引用）、`TightensWithin` 核收紧、`DeliveryConditionReferenceFor` 算四格；领域测试五例；票面 Comments 记改派 |
| ② | `d7bd4132` | `ports.DeliveryConditionView` + `PublicationRegistry.SaveDeliveryConditions`；迁移 `0030_delivery_condition.sql`（父表 + 子表）；postgres `DeliveryConditions` 读口与 `CommercialPublications.SaveDeliveryConditions` 写口；真库三例；两处 `PublicationRegistry` 替身跟随 |
| ③ | `697c9dc5` | 发布用例：`CommercialDeclarations.DeliveryConditions`（`DeliveryConditionDeclaration{Tightens?, Terms}`）、`DeclarationChannel` 多一格 `DELIVERY_CONDITION`、`declarationWrites` 按版本类别选层；用例测试两组 |
| ④ | `7f18b156` | `cmd/parcel-commercial` 批文 `declarations.deliveryConditions` 一节 + 翻译测试；真库端到端一例（批文 → 0030 两表 → 固定闭包 → 读口答引用）；PC postgres 事务门禁补 `SaveDeliveryConditions` 负向证据、真库测试事务闭包不 `Fatalf`（架构门禁两条） |
| ⑤ | `aff5537d` | 机制清点在 `7f18b156` 干净检出重生成（PC 生产 119→121 / 测试 130→133 / postgres 适配器 32→33；迁移 29→30、全仓 157→158；端口声明 369→370） |
| ⑥ | 本笔 | 票面 → resolved + 本记录 |

**读口四格的符号名（供 tf/14 的 TF 进程内适配器逐字对）**：端口 `pcports.DeliveryConditionView.LoadDeliveryConditionReference(ctx, tenant domain.TenantID, resolution domain.ResolutionID) (domain.DeliveryConditionReference, bool, error)`，实现 `pcpostgres.NewDeliveryConditions(db)`。四格：`found=true` → `domain.DeliveryConditionReference`（`Resolution()` 即传入的回指，`String()` 即回指拼写——引用就是回指本身，不另造，ADR-0133 决定一）；`found=false && err==nil` → 「没有交付条件」（闭包在场、已采用的产品版本与合同版本两层都无声明）；`errors.Is(err, domain.ErrDeliveryConditionClosureAbsent)` → 闭包不在场（含他租户拿同一回指：租户条件进语句，不区分「不存在」与「属于别的租户」）；`errors.Is(err, domain.ErrDeliveryConditionContractNotAdopted)` → 闭包在场却未采用客户合同版本。其余 `err` 是读不回或坏行（一层在场却过不了构造门），不属四格。**服务产品版本未被采用不是一格**：那一层只是没有可读的声明，答案落在合同层身上（`domain.DeliveryConditionReferenceFor` 头注）。

**开放引用 vs 封闭集的裁法（票面「先裁」）**：**取开放引用**。三格（交付方式、收件范围规则、交付证明规则）都是 `requiredValue` 开放串，本上下文只查非空、不登词表、不校验存在。理由：PC CONTEXT 今天没有交付方式的词条，TF CONTEXT 硬句「安全投放、智能柜、邻居代收、本人签收和其他交付方式」是举例且明写「其他」，不是封闭集；`PAR-NET-09` 把交付方式取值划在实例半边，与本上下文对资料组引用（ADR-0120 决定七）、收寄来源等的处理同形——先立词表再登是让开发方替租户封闭一个租户自己的集合。代价：库上与领域都守不住「拼错的方式」，两层的方式集合按字面比（合同层 `METHOD/in-person` 与产品层 `METHOD/In-Person` 是两种方式，收紧判为放宽拒）。owner 若要封闭集，改的是 `DeliveryMethodReference` 一处 + PC CONTEXT 立词 + GLOSSARY 加行 + 迁移加 CHECK，读口与两层的形不动。CONTEXT / GLOSSARY 因此本票未动。

**批文形状**（`cmd/parcel-commercial` 受控批文 `declarations` 下）：`"deliveryConditions": {"tightens": {"objectId", "version"}（仅合同层）, "methods": ["…"], "recipientScopeRule": "…", "proofOfDeliveryRule": "…"}`；缺键 = 这一版没有交付条件声明。

**触及**（16 件，+1671/−6，`git diff --stat d9a1f571..aff5537d`）：`internal/partycommercial/domain/{delivery_condition.go,delivery_condition_test.go}`（新）、`ports/ports.go`（加一口一 Save，不改既有签名）、`application/{publish_commercial_authority.go,publication_delivery_condition_test.go（新）,publish_commercial_authority_test.go（替身加一法）}`、`adapters/postgres/{delivery_condition.go（新）,delivery_condition_test.go（新）,transaction_guard_test.go（加一行）}`、`adapters/http/register_commercial_test.go`（替身加一法）、`migrations/party_commercial/0030_delivery_condition.sql`（新）、`cmd/parcel-commercial/{translate.go,translate_test.go,publish_batch_test.go}`、`docs/product/MECHANISM-INVENTORY.md`、本票面。**未碰**：`apps/**`（管理台表单与 `adapters/http/publication_draft_payload_*` 载荷都归另立的 awf 票）；`parcelshipment/**`、`transportfulfillment/**`、`networkrouting/**`；`PlanApplicability`；ADR-0133 / 0058 / 0114 正文；PC CONTEXT / GLOSSARY（见上一段）；`migrations.go` / `plan.go`（`all:party_commercial` 整目录嵌入）；`internal/architecture/*_baseline.txt`（新导出都有生产调用点，门禁绿，未加行）；`docs/adr/README.md`（无新 ADR）；`cmd/parcel-api` 装配（读口的消费方是 tf/14 的 TF 进程内适配器，今天还不在，见判断题 6）。

**验收对照**（票面完成判据逐项）：1 领域构造门——产品层零方式拒 ✓（`ErrDeliveryConditionNotConfigured`）/ 合同层出现产品层没有的方式拒（放宽）✓（`TightensWithin` → `ErrDeliveryConditionWidened`；持久化写口对着读回的产品层调它）/ 合同层子集通过 ✓ / 拥有对象非对应种类或未生效拒 ✓（`ErrDeliveryConditionOwner`：接单规则包、已发布未生效、两层互相冒名各一例）；2 读口四格各一例（替身）✓（领域 `TestDeliveryConditionReferenceIsAnsweredFromTheClosureAndTheTwoLayers` 四格全摆出）、真库往返 ✓ 覆盖「产品层有、合同层无」「两层都无 → 没有」「闭包不在场 → error」外加「两层都有」「跨租户」「未采用合同」（`TestDeliveryConditionReferenceIsAnsweredByResolutionReference`），DSN 缺席走 `pgtest` SKIP 不 PASS ✓；3 发布通道端到端 ✓（`TestAPublishedDeliveryConditionIsAnsweredByTheResolutionKeyedReadFace`：进程口批文两项 → 册上两版 + 0030 两表 → 按同一范围键解出并固定的闭包回指 → 读口答引用）；4 验证见下 ✓，清点在干净检出重生成 ✓（⑤）。红线三条 ✓：无任何租户取值（夹具全是 `METHOD/…` / `RULE/…` 合成串）；读口只答有没有、不认识包裹；三份 ADR 正文未动。

**验证强度**（隔离树 `D:/tops/idp-parcel-mcp5-pcgaps11`，代码 tip `7f18b156`，`status --untracked-files=all` 空）：`gofmt -l ./internal ./cmd ./migrations ./tools` 零输出；`go build ./...`、`go vet ./...` 全仓退 0；**带 DSN** `go test -p 1 -count=1 -v` PC 四包 + `./migrations/...` + `./internal/platform/migrate/...` + PC ports 五个反向依赖包（`networkrouting/adapters/partycommercial`、`parcelshipment/adapters/{partycommercial,postgres}`、`settlementaccounting/adapters/partycommercial`、`visibilityexception/adapters/partycommercial`，`go list -f '{{.ImportPath}} {{.Deps}}' ./...` 筛出）+ `./internal/architecture/...` + `./cmd/...` → 23 包 ok / 0 FAIL，`--- PASS` 2201 / `--- SKIP` 0 / `--- FAIL` 0（约 48 秒；此前一轮架构门禁两条红——`SaveDeliveryConditions` 缺无事务负向证据、真库测试事务闭包里 `Fatalf`——随 ④ 修）；新增真库用例探针 `-v` 下 PASS 非 SKIP。未跑全量（作者范围口径）、未跑 `-race`。日志 `%TEMP%\pc11-author-run.log`，仓内无残留。占 / 释 55432 各两轮均广播。

**与 main 碰面**（`git fetch` 后 `git merge-tree --write-tree origin/main mcp5-pcgaps11`，origin/main = `c0cdebba`，干跑未动树）：唯一冲突是 `docs/product/MECHANISM-INVENTORY.md`（main 在 `84e89dc7` 之后为 nr/03、psr/06、psr/07 各重生成过一次），生成物按「谁的笔在后谁的数字盖前面」由推送方在 tip 重生成兑底即解；`.go` / `.sql` 无重叠（`git log d9a1f571..origin/main -- internal/partycommercial cmd/parcel-commercial migrations/party_commercial` 只有清点笔）。

**判断题**（给评审与推送方，都不阻断；1、4 尤其请 owner 看一眼）：

1. **收紧的核在持久化写口不在用例**：`declarationWrites` 刻意不读库（全部构造先于全部写入），而核「合同层方式 ⊆ 产品层方式」要产品层在手；发布用例的依赖只有 `PublicationRegistry`（加构造参数要动 ~140 处调用），所以核放在 `CommercialPublications.SaveDeliveryConditions` 里：同一事务内 `LoadForScope(合同版本的范围)` → `Lookup` 所收紧的产品版本 → 读回产品层 → `content.TightensWithin(product)`，不在册 / 无产品层 / 放宽都返回 error 随事务回滚。领域的门 `TightensWithin` 仍是唯一判据、领域测试直接钉它；持久化只负责把对手方读出来。代价：这是本仓第一处「Save 返回 error 表达业务拒件」（ADR-0031 的三格算术只覆盖重放 / 冲突），进程口把它报成该项失败，与构造门拒件同一落点。替代是 `PublicationRegistry` 加一只按版本的读口让用例先读再构造——多一口、三处替身跟随，且读口与写口同居一个接口，本票没取。
2. **合同层必须指名所收紧的产品版本**（`tightens`），不从合同壳的指名引用推：壳上 `ReferenceTo(ServiceProductObject)` 只有对象没有版本号，而收紧是对着一版说的话（产品 v2 可能放宽了 v1）。所收紧的产品版本在**合同自己的范围册**上找（闭包解析按同一把范围键同时采用产品与合同，跨范围的产品对这份合同的客户不可见）。日后闭包采用的产品版本 ≠ 合同层指名的那一版时，本读口照答「有」（只答有没有），内容读口届时要报「版本错配」——归第二个消费方那张票。
3. **两条规则引用在两层都必填**：一层登了方式却没说凭什么算有效交付，等于让 TF 自己补一条规则，正是 UC-TF-006「不以通用签名规则替代合同」要拦的；收紧判据只落在方式集合上——规则引用是开放引用，本上下文判不了两条规则谁更严，合同层给出的就是该合同采用的那条。若 owner 认为合同层可以「不说就沿用产品层的规则」，改 `DeclareContractDeliveryConditions` 放开两格 + 迁移两列改可空。
4. **`deliveryConditions` 没折进 PCC-1 规范化文档**（服务产品册今天「没有正文」、客户合同册的 `canonicalCustomerContractBody` 只有 `contractContent` / `preAcceptanceControl` 两键）：先例是挂在产品版本上的 `pendingRoutingBasis` 同样不在文档里，且 ADR-0126 边界写明「不改受控批文既有字段语义」；对账门对这两册的行为一字未变。代价：受控批文声明的摘要今天盖不住这一节，两份只差交付条件的批文可以带同一个 `PCC-1:` 串过对账门（`pendingRoutingBasis` 今天同样如此）；待批准发布（ADR-0126 决定三）的快照也带不上它。折进去的那天要动 `PublicationContent` / `canonicalServiceProductContent`（「本册没有正文」那句要改）/ `registerHasNoBody` / `publicationContentOf` 与 `declarationsOfContent` / 载荷四处，带 `omitempty` 不换号——归给管理台表单加这一节的 awf 票，同笔做才不会让载荷与文档两处说两套词。
5. **读口对「服务产品版本未被采用」不单列一格**（ADR-0133 决定二只列了闭包不在场与未采用合同两格）：那一层只是没有可读的声明，答案落在合同层身上；闭包能不能不采用产品是解析键的事。
6. **`pcpostgres.DeliveryConditions` 今天没有进程装配它**：消费方是 tf/14 的 TF 进程内适配器（ADR-0133 Consequences 第三条），今天还不在；架构门禁绿是因为新导出的函数都有生产调用点、`New*` 构造子按前缀跳过。tf/14 接线那天在 `cmd/parcel-api`（或 TF 所在进程）装配。
7. **一表带层级列（`object_kind` 1 / 2）而不是父子两套表**：两层同形，只多两列所收紧的产品版本；CHECK 按层钉住那两列的在场性。保价条件日后照抄这一形。

## Comments

- 2026-09-10 12:2x · 改派通道 5（单 task-4649070b-bec3-433b-ba5c-58fb8b4ac4b7），分支 `mcp5-pcgaps11` 基 `d9a1f571`（= 通道 4 的票面 in-progress 一笔）；通道 4 二次崩前无半成品。每步一笔并推。
- 2026-09-09 · 通道 2（task-f5521768）：立票（ready-for-agent）。本目录 spec.md「四票一览」表自 05 起未再扩行（05–10 只在 Status 行点名），本票照旧不改表。ADR-0133 越权风险点 2（归属）与 3（闭包不在场答 error 而非未配置）若 owner 复核后改口径，本票随之改拥有对象 / 读口那一格，不回改 ADR。
