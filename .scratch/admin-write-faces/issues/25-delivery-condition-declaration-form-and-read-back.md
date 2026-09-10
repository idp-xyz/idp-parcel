# 25 交付条件声明的管理台写面 + 读回 + 载荷 + 折进 PCC-1 一格：产品版本表单与客户合同表单各长一节「交付条件」，服务产品册从「没有正文」变成「正文可缺」

Category: enhancement
Status: draft——通道 3 于 2026-09-10 按 MCP-1 派单 task-910498f2 立票，取证锚远端 main `93349f83`（pc-gaps/11 进 main 于 `62c87e73`）；**只写票面，未动代码。** 三件（表单 / 载荷 / 折进 PCC-1）形状已裁清、照先例可直接做；第四件读回的落点是一条要裁的（后端今天没有任何读 0030 的目录口，两条路都要把地盘扩到 `ports` 与 `adapters/postgres`），见「要裁的」。裁定后本票转 ready-for-agent
Blocked by: 无——pc-gaps/11 已进 main（领域 / 端口 / 迁移 0030 / 发布通道 / 批文 `deliveryConditions` 全在）；08（公共半边）、09（产品版本表单）、10（客户合同表单）、22（共享层）均已 resolved

## 从哪里来

[pc-gaps/11](../../party-commercial-context-gaps/issues/11-delivery-condition-declaration-family-and-resolution-keyed-read-face.md) 完成记录三处点名另立 awf 票：「管理台表单（admin-write-faces）不在本票，另立」；「未碰：`apps/**`（管理台表单与 `adapters/http/publication_draft_payload_*` 载荷都归另立的 awf 票）」；判断题 4「`deliveryConditions` 没折进 PCC-1 规范化文档……归给管理台表单加这一节的 awf 票，**同笔做**才不会让载荷与文档两处说两套词」。本票就是那一张。

本票不是伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 的子票（伞票已收口；十册主路径各有其票）：交付条件不是第十一册，是**挂在两种既有版本上的一节声明**（ADR-0133 决定四），形照 `preAcceptanceControl` 之于客户合同版本表单——作版本表单里的一节，不另开一册、不另开一签。

## 今天的形状（`93349f83` 上量，逐符号名）

**领域与批文（pc-gaps/11 落的，本票只引用不改）**：`domain/delivery_condition.go`——两层同形 `DeliveryConditionContent`，产品层 `DeclareProductDeliveryConditions(owner, terms)`、合同层 `DeclareContractDeliveryConditions(owner, tightens, terms)`；正文三格 `DeliveryConditionTerms{Methods []DeliveryMethodReference, RecipientScopeRule, ProofOfDeliveryRule}` **全是开放引用**（`PAR-NET-09` / `PAR-COM-05` / `PAR-COM-06` 实例半边，本上下文只查非空、不登词表）；私有门 `declareDeliveryConditions` 守三格齐、方式至少一种且不重（`ErrDeliveryConditionNotConfigured` / `ErrConflictingDeliveryCondition`），方式按字面稳定排序；「只能收紧」由 `TightensWithin` 在**持久化写口** `CommercialPublications.SaveDeliveryConditions` 读回产品层后核（pc-gaps/11 判断题 1，评审记归 owner 复核、不挡）。批文 `cmd/parcel-commercial/translate.go` 的 `deliveryConditionDocument`：`tightens{objectId, version}`（仅合同层）/ `methods[]` / `recipientScopeRule` / `proofOfDeliveryRule`，缺键 = 这一版没有交付条件。发布用例 `CommercialDeclarations.DeliveryConditions *DeliveryConditionDeclaration{Tightens *TightenedProductVersion, Terms}`，`declarationWrites` 按版本类别选层。

**规范化（ADR-0126，本票要改的那一格）**：`domain/publication_canonicalization_service_product.go` 头注「本册**没有正文**……`PublicationContent` 与 `canonicalPublicationDocument` 都不为它加格」，`canonicalServiceProductContent()` 折两格文档 `{canonicalization, kind}`，`registerHasNoBody(kind)` 只对服务产品答真、`RehydratePublicationContent` 据以在「正文缺席」判断前早返回；`publication_canonicalization_customer_contract.go` 的 `CustomerContractBody{RulePackage, Bindings, Control *PreAcceptanceControlBody}` 与 `canonicalCustomerContractBody{contractContent, preAcceptanceControl?}`——合同级声明可缺、缺席不折进文档不影响正文那一层的字节。`publicationContentOf`（`application/publish_commercial_authority.go`）对服务产品走 `default` 答「不在场」；对客户合同「正文在不在场看 0012 那一层（contractContent）」，在场时把 `PreAcceptanceControl` 折进 `Control`。`declarationsOfContent`（`application/publication_draft.go`）是它的反向，两处今天都没有交付条件。

**载荷（ADR-0126 决定四）**：`adapters/http/publication_draft_payload.go` 的 `CommercialPublicationPayload` 尾注「服务产品册没有正文格：它的载荷就是上面的壳」；`publication_draft_payload_customer_contract.go` 的 `CustomerContractBodyPayload{contractContent, preAcceptanceControl?}`。

**表单**：`apps/admin-web/src/pages/party/service-product-form.ts` 头注「本册的载荷就是壳」，草稿只有壳四格 + 有效起止 + 引用表；`customer-contract-form.ts` 草稿 = 壳 + `rulePackage` + `bindings[]` + 合同级声明两格，`payloadOf` 把 `preAcceptanceControl` 一节**永远送**（「没选要求就送空要求……让服务端把选择交回操作者」）；可缺声明节的先例在 `acceptance-rule-package-form.ts`：`sectionDeclared(draft, section)`——一节里一格都没填就整节缺席（= 未声明），填了任一格整节原样送、空数组也送、「一行都没有」由服务端答。两张表单各挂 `ServiceProductsPage.tsx` / `PartyContractsPage.tsx`，五步走 `PublicationDraftFlow`。

**读回——今天没有能显交付条件的面。** `/commercial-service-products` 列 `ports.ServiceProductCatalogueRow`（壳 + `Form`，`api.ts` 注释「那边上列版本壳」）；`/commercial-customer-contracts` 列 `ports.CustomerContractCatalogueRow`（壳 + 0012 正文左连接：`HasContent` / `RulePackageID` / `Bindings`）；合同级 0007 声明的读回是政策页独立一册 `/commercial-policies?kind=PRE_ACCEPTANCE_CONTROL`（`ports.PreAcceptanceControlRow`、`OperationsCatalogue.ListPreAcceptanceControls`、`servePreAcceptanceControls`、admin-web `CommercialPolicyKind` 一格）。**没有「版本详情」页**；`adapters/postgres/delivery_condition.go` 的 `DeliveryConditions` 只有按回指答有没有的 `LoadDeliveryConditionReference`（TF 那一口），没有面向管理台的目录读。

**cmd 夹具**：`cmd/parcel-commercial/publish_batch_test.go` 的 `deliveryConditionBatchBody`——服务产品项带 `deliveryConditions` 且 `contentDigest` 为 `sha256:product-1`；客户合同项带 `deliveryConditions`（`tightens` 指 product-1/v1）**不带 `contractContent`**，`sha256:contract-1`。

## 为什么四件必须同笔（判断题 4 的结构性理由）

表单路径的发布不重读载荷：预览 / 录入把载荷折成 `PublicationContent` → `CanonicalizePublicationContent` → 摘要 + **文档**；待批准载体存的正文快照就是那份文档（ADR-0126 决定三）；发布那一步 `RehydratePublicationContent` 折回正文 → `declarationsOfContent` → `CommercialDeclarations` → `declarationWrites`。**不在文档里的声明，发布时就不存在。** 所以表单上加一节而不折进 PCC-1，操作者填的交付条件会在批准与发布之间静默消失——载荷、文档、`declarationsOfContent` 三处必须一次加齐。受控批文那一半（`translate.go` → `CommercialDeclarations`）不经文档，所以 pc-gaps/11 不折也能走通，那是它把这件事留给本票的原因。

## 做法

### ① 表单：两张版本表单各长一节「交付条件」

- **产品版本表单**（`service-product-form.ts` / `ServiceProductPublicationForm.tsx`）加一节：`methods`（**多行文本一行一项**，照 awf/18 材料清单的选形——正文里方式是「集合」不是「行」，服务端对 `…methods[i]` 逐项的话汇到文本框下带项号）、`recipientScopeRule`、`proofOfDeliveryRule` 两格串。**一节可缺**：照 `sectionDeclared` 的先例——三格一格都没填即整节缺席（这一版没有交付条件，不声明），填了任一格整节原样送、空格照送由服务端点名。不预开、不预选、不给任何方式的候选（开放引用，`PAR-NET-09`；pc-gaps/11 已裁开放引用不立词表，本票不自造下拉）。
- **客户合同表单**（`customer-contract-form.ts` / `CustomerContractPublicationForm.tsx`）加同一节，多两格 `tightens.objectId` / `tightens.version`（所收紧的服务产品版本，**手填**——壳上 `references.SERVICE_PRODUCT` 只有对象没有版本号，pc-gaps/11 判断题 2 裁不从壳推）。**「只能收紧」前端不自判**：那道门在服务端写口（`TightensWithin`），前端只显服务端拒件原词；预览那一步看不出放宽（预览无产品层可对），发布那一步才拒——这一后果写进「判断题」。
- 本地 `localProblems` **无新增**：全是字符串格；同方式两行由领域 `ErrConflictingDeliveryCondition` 答，不在本地拦（伞票硬句「表单不裁任何门」）。
- 认领路径（`*FieldPaths`）与渲染路径（`*RenderedPaths`）两张表按节长出（`serviceProduct.deliveryConditions{,.methods,.methods[i],.recipientScopeRule,.proofOfDeliveryRule}` / `customerContract.deliveryConditions{,.tightens,.tightens.objectId,.tightens.version,…}`），`publication-form-rendered-paths.test.ts`（票 22 判据 3）照比。
- `publication-draft-api.ts`：`CommercialPublicationPayload` 加 `serviceProduct?: { deliveryConditions?: DeliveryConditionPayload }`，`CustomerContractBodyPayload` 加 `deliveryConditions?`；`DeliveryConditionPayload{tightens?: {objectId, version}, methods: string[], recipientScopeRule, proofOfDeliveryRule}` 键名与批文逐字同。
- **改既有表单文件前广播占号**（通道 5 同期在 `apps/admin-web` 改注释，会先占 `PublicationFormFields.tsx` / `publication-form-shared.ts`——本票不动那两个文件；两张表单文件与 `publication-draft-api.ts` 动手前另发占号）。

### ② 读回：见「要裁的」1（裁定后补做法）

### ③ 载荷：`adapters/http/publication_draft_payload_*` 加 `deliveryConditions` 一节

- 新文件 `publication_draft_payload_delivery_condition.go`：`DeliveryConditionPayload{Tightens *TightenedProductPayload json:"tightens,omitempty"; Methods []string json:"methods"; RecipientScopeRule; ProofOfDeliveryRule}`、`TightenedProductPayload{ObjectID json:"objectId"; Version json:"version"}`——键名镜像 `translate.go` 的 `deliveryConditionDocument`；`body(problems, root)` 逐格过构造门（`NewDeliveryMethodReference` 逐项点名 `<root>.methods[i]`、两条规则各点名、`tightens` 在场时两格各点名），零方式 / 同方式两行 / 层与 `tightens` 配对（产品层带了 / 合同层缺了）留给领域 `validate`，预览与录入都答`未受理`带成因。
- `CommercialPublicationPayload` 加 `ServiceProduct *ServiceProductBodyPayload json:"serviceProduct,omitempty"`（`{DeliveryConditions *DeliveryConditionPayload json:"deliveryConditions,omitempty"}`）——服务产品册从「没有正文格」变成「一格可缺」，尾注改口；`CustomerContractBodyPayload` 加 `DeliveryConditions *DeliveryConditionPayload json:"deliveryConditions,omitempty"`（第三层，与 `preAcceptanceControl` 并列可缺）。`Publication(tenant)` 各加一支。
- 契约测试照 `publication_draft_payload_customer_contract_test.go`：同一份载荷预览 / 录入同摘要；逐格问题收齐（含 `methods[i]` 空串、`tightens.version` 空）；产品层带 `tightens`、合同层缺 `tightens` 在预览上答未受理不带摘要。

### ④ 折进 PCC-1（`omitempty` 加键，不换号）

- **领域**（`publication_canonicalization_service_product.go` / `_customer_contract.go`，不动 `delivery_condition.go`）：新输入面 `DeliveryConditionBody{Tightens *TightenedProductVersion; Terms DeliveryConditionTerms}`，`validate(layerKind)` 复用同包私有门 `declareDeliveryConditions`（三格齐、方式≥1 不重、稳定序）并判层配对（产品层 `Tightens == nil`、合同层 `!= nil`，否则 `ErrDeliveryConditionNotConfigured` / `ErrDeliveryConditionOwner` 原词）；`ServiceProductBody{DeliveryConditions *DeliveryConditionBody}` 作本册**可缺**的正文；`CustomerContractBody` 加 `DeliveryConditions *DeliveryConditionBody`（第三层可缺，`validate` 在场时过门）。
- **文档**：`canonicalPublicationDocument` 加 `ServiceProduct *canonicalServiceProductBody json:"serviceProduct,omitempty"`（`{DeliveryConditions *canonicalDeliveryConditions json:"deliveryConditions,omitempty"}`）；`canonicalCustomerContractBody` 加 `DeliveryConditions *canonicalDeliveryConditions json:"deliveryConditions,omitempty"`；`canonicalDeliveryConditions{Tightens *{objectId, version} omitempty; Methods []string（领域已稳定序）; RecipientScopeRule; ProofOfDeliveryRule}` 键名照批文。`PublicationContent` 加 `ServiceProduct *ServiceProductBody`。
- **`CanonicalizePublicationContent`**：服务产品——`content.ServiceProduct == nil` 或其 `DeliveryConditions == nil` 时仍折**两格文档**（字节与今天逐字节同，既有产品版本的摘要一个不变）；在场时三格；`content.ServiceProduct != nil && Kind != ServiceProductObject` 进 kind 不符那一格。客户合同——`validate` 后 `canonicalCustomerContractBodyOf` 多折一键。
- **`RehydratePublicationContent`**：`decoded.ServiceProduct != nil` 一支；`registerHasNoBody` 改名改义为「正文可缺的册」（两格文档折回 `PublicationContent{Kind: SERVICE_PRODUCT}` 仍合法、不答 `ErrPublicationContentAbsent`），头注「本册没有正文」那一句连同 `publicationCanonicalizationVersion` 注释里「服务产品无正文，文档只有两格」、`publication_draft_payload.go` 尾注、`service-product-form.ts` 头注一并改口，不留旧话。`IsRegisterCanonicalized` 不动。
- **应用两向**：`publicationContentOf(ServiceProductObject)` 加一支——`declarations.DeliveryConditions == nil` 答「不在场」（壳单独发布，照今天登记声明的串），在场时折成 `ServiceProduct.DeliveryConditions`；`CustomerContractObject` 那一支在 `ContractContent` 在场时把 `DeliveryConditions` 折进 `Control` 旁那一格（在不在场的判据仍看 0012 那一层，**不改**——ADR-0126 边界「不改受控批文既有字段语义」）。`declarationsOfContent` 反向各一支：载体上有这一层就交 `CommercialDeclarations.DeliveryConditions`，没有就不交。
- **cmd 夹具**：`deliveryConditionBatchBody` 服务产品项的 `sha256:product-1` 换成 `CanonicalizePublicationContent` 对那份正文算出的 `PCC-1:…`（对账门自此对带交付条件的产品项开门）；客户合同项不带 `contractContent` → 仍「不在场」→ `sha256:contract-1` 不动。**带 DSN 跑 `./cmd/parcel-commercial/`**（awf/13 那次夹具摘要就是带 DSN 才红；评审记录明写「下次同形票应把这条写进完成判据」，本票写进）。`publish_commercial_authority_test.go` 里若有带交付条件的产品 / 合同用例壳，同笔换算出的串。
- **PCC-1 不换号**：加的全是 `omitempty` 键；写一条测试钉「服务产品两格文档的摘要与本票前逐字节同」「不带交付条件的合同正文摘要与本票前逐字节同」。

## 本册规范化判断（照 awf/09「本册规范化判断」那一节的形；票内小裁决，不改 ADR-0126 一句）

**问题**：服务产品册从「没有正文」变成「正文可缺（交付条件一节）」，对账门与 awf/09 留给伞票的那道题（无正文册的「已接」与对账门开门拆开、何时拒收旧式 `sha256:` 串）怎么办？

**答：本票不碰那道题，也不需要碰。** 对账门的既有规则是「已接的册 **且** 正文在场才比」（`publicationContentOf` 第二返回值；ADR-0126 决定二「正文缺席的版本没有可比对象，照今天登记声明的串」）——信用政策壳无正文照发、客户合同只带合同级声明不带 `contractContent` 照发，都是这一规则。本票让 `publicationContentOf(SERVICE_PRODUCT)` 在 `deliveryConditions` 在场时答「在场」，不在场时**照旧**「不在场」：

- 不带交付条件的产品版本（今天全部存量、`scripts/demo-seeds/data/commercial/publish-batch.json` 两项 `sha256:syn-SYN-PROD-*`、`cmd/parcel-dispatch/syn_pc_seed_test.go`）一字不动、门不开、行为与摘要字节都与今天同。
- 带交付条件的产品版本是**有正文的版本**，与任何有正文的册同规则：批文必须抄算出的串（本票 cmd 夹具那一处换串就是它第一次成立）。
- awf/09 评审非阻断 (1) 说 `IsRegisterCanonicalized` 头注「对账门用它分辨……」对无正文册失真——本票之后服务产品册**有了**正文通道，那句注释对本册重新成立，本票不改它、也不替伞票裁「何时拒收旧串」。

**替代（否决）**：借本票之机对全部产品版本开门（`publicationContentOf` 对服务产品恒答在场、两格文档也比）。否决：那正是 awf/09 留给伞票收口的「何时拒收旧串」本身，seed 两项与 `syn_pc_seed_test.go` 随之换串，不是本票的题；且把「没有正文」的版本读成「有一份空正文」与 ADR-0126 决定二那句相悖。

## 要裁的

1. **读回落在哪一面；两条路都要把地盘扩到 `ports.go` 与 `adapters/postgres/operations_catalogue.go`（派单地盘未列）。** 后端今天没有任何面向管理台的 0030 读口，「版本详情里显声明」没有现成的面可挂。
   - **甲 · 政策页新一册 `?kind=DELIVERY_CONDITION`**（照 0007 合同级声明 `PRE_ACCEPTANCE_CONTROL` 那一册的形，一字不改）：`ports.DeliveryConditionRow{OwnerKind, ObjectID, Version, Tightens?, Methods, RecipientScopeRule, ProofOfDeliveryRule, DeclaredAt}` + `OperationsCatalogue.ListDeliveryConditions`（一条查询父子两表，两层同列带层别）+ `query_commercial_policies.go` 一 kind 常量 / 一 `serve*` / `CommercialPolicyCatalogueReader` 加一法（→ `query_commercial_catalogue_test.go` 等替身跟随、**`cmd/parcel-api/unwired_orchestration.go` 的 `unwiredCommercialCatalogue` 加一法**）+ admin-web `CommercialPolicyKind` 加格 / Record / 列向 / 标签 / 页签与来源提示句（awf/06 / 21 的做法，本通道做过 21）。好处：一处列两层，合同层收紧对着哪一版产品层一眼可见；照先例零设计。代价：写在产品页 / 合同页、读在政策页，写签读签分两页（伞票判据「写签跟着读签走」反着来）；动 `cmd/parcel-api` 一处。
   - **乙 · 折进两张既有目录行**（照 0012 合同正文左连接进 `CustomerContractCatalogueRow` 的形）：`ServiceProductCatalogueRow` / `CustomerContractCatalogueRow` 各加一格可缺的 `DeliveryConditions`（合同行另带 `Tightens`）+ 两条查询各 LEFT JOIN 0030 父表并聚合子表方式行 + `query_service_products.go` / `query_commercial_relations.go` 两个 body 各加一可缺节（`omitempty`，没声明就没有键——照 `contentRegistered` 那条「页面先看布尔 / 键在不在」的纪律）+ admin-web `ServiceProductRecord` / `CustomerContractRecord` 各加可缺一节、两张表各显一节（没声明显「未声明」不显默认）。好处：声明显在它挂的版本旁，写签读签同页，与派单原话「版本详情里显声明」一致；不动 `cmd/parcel-api`。代价：两处改而不是一处；`/commercial-service-products` 从「上列版本壳」变成壳 + 一节可缺正文（与 Go 侧本册的变化同向，`api.ts` 那句注释随改）；合同层「收紧对着哪一版」要在合同行里显 `tightens`，两层不在一处对照。
   - **倾向乙**：派单原话与伞票「写签跟着读签走」都指向它，合同目录行已经这样带着 0012 正文；甲的唯一优势（两层同列对照）可以由乙里合同行显 `tightens` 补一半。**不自裁**：两条路的地盘不同（甲多动 `cmd/parcel-api`，乙多动两个查询处理器），且都超出派单地盘。

## 红线

- 不写任何真实交付方式 / 收件范围 / 证据规则取值（`PAR-NET-09` / `PAR-COM-05` / `PAR-COM-06`）；表单不给方式候选、不内置「本人签收」、不预开一行；夹具全是 `METHOD/…` / `RULE/…` 合成串。
- 表单不算摘要、不裁任何门、不收也不送批准人（伞票 07 硬句）；「只能收紧」由服务端写口答，前端只显原词。
- PCC-1 不换号：只加 `omitempty` 键；既有产品版本（两格文档）与不带交付条件的合同正文的摘要字节一个不变，测试钉住。
- 不动 PC 领域声明形状（`delivery_condition.go`）与迁移 0030；不动 `transportfulfillment/**`；不改受控批文既有字段语义（合同项在不在场仍看 `contractContent`）。
- 服务产品壳单独发布仍合法：两格文档不答 `ErrPublicationContentAbsent`。

## 完成判据（非作者评审逐项对；②那一条按裁定的路补）

1. 产品版本表单与客户合同表单各多一节「交付条件」：一格没填整节缺席（载荷里无键）、填任一格整节送、合同层多 `tightens` 两格——node:test 各钉；`publication-form-rendered-paths.test.ts` 两张表单的认领 / 渲染路径表含本节全部路径。
2. 载荷：新文件契约测试——同一份载荷预览 / 录入同摘要；逐格问题按 `serviceProduct.deliveryConditions.*` / `customerContract.deliveryConditions.*` 收齐；层配对错在预览上答未受理不带摘要。
3. 折进 PCC-1：领域测试钉「产品两格文档摘要不变」「合同不带本节摘要不变」「带本节的文档键名镜像批文、方式稳定序、换方式行序摘要不变」「折回再算同串」；应用两向各一例（产品带本节 → 对账门开门；产品不带 → 不在场照旧；合同折进 / 折回）；`registerHasNoBody` 改义后两格文档仍折回合法正文。
4. cmd：`deliveryConditionBatchBody` 产品项换算出的 `PCC-1:` 串，**带 DSN** `go test -count=1 ./cmd/parcel-commercial/` PASS 非 SKIP；合同项不动且仍落地。
5. 读回（按「要裁的」1 裁定的路）：发布落定后声明在那一面立刻可见；没声明的版本显「未声明」不显默认；合同行 / 册显 `tightens`。
6. 四处旧话改口：`publication_canonicalization_service_product.go` 头注、`publicationCanonicalizationVersion` 注释「服务产品无正文」句、`publication_draft_payload.go` 尾注、`service-product-form.ts` 头注。
7. 验证照 awf/18：admin-web `tsc --noEmit` 0 / `run-tests` 全 pass；Go `gofmt -l` 空、`go build` / `go vet` 0、PC 四包 + `cmd/parcel-commercial`（**带 DSN**）+ `./internal/architecture/...` ok；清点在干净检出重生成。证据层级 S。

## 地盘

`apps/admin-web/src/pages/party/`（`service-product-form.ts` + 测试、`ServiceProductPublicationForm.tsx`、`customer-contract-form.ts` + 测试、`CustomerContractPublicationForm.tsx`、`publication-draft-api.ts`、`publication-form-rendered-paths.test.ts`；**不动** `PublicationFormFields.tsx` / `publication-form-shared.ts`——通道 5 在占）；`internal/partycommercial/domain/`（`publication_canonicalization.go` 加格加支、`_service_product.go` / `_customer_contract.go` 加层、新测试；**不动** `delivery_condition.go`）；`internal/partycommercial/application/`（`publish_commercial_authority.go` 的 `publicationContentOf` 一支、`publication_draft.go` 的 `declarationsOfContent` 一支、测试）；`internal/partycommercial/adapters/http/`（`publication_draft_payload.go` 一格、`_customer_contract.go` 一格、新 `_delivery_condition.go` + 测试）；`cmd/parcel-commercial/publish_batch_test.go` 一串。**待裁定后扩**：`internal/partycommercial/ports/ports.go`、`adapters/postgres/operations_catalogue.go` + 测试、`adapters/http/query_*.go` + 测试、admin-web `api.ts` 与页面（乙）或 `cmd/parcel-api/unwired_orchestration.go`（甲）。共享文件动手前逐份占号。

## 参照

pc-gaps/11 完成记录「触及 / 未碰」与判断题 1 / 2 / 4；ADR-0133 决定四；ADR-0126 决定一 / 二 / 三 / 四与边界句；`internal/partycommercial/domain/delivery_condition.go`（`DeliveryConditionTerms`、`declareDeliveryConditions`、`TightensWithin`）；`publication_canonicalization.go`（`PublicationContent`、`canonicalPublicationDocument`、`RehydratePublicationContent`、`IsRegisterCanonicalized`）、`publication_canonicalization_service_product.go`（`canonicalServiceProductContent`、`registerHasNoBody`）、`publication_canonicalization_customer_contract.go`（`CustomerContractBody`、`canonicalCustomerContractBody`）；`application/publish_commercial_authority.go`（`CommercialDeclarations.DeliveryConditions`、`DeliveryConditionDeclaration`、`publicationContentOf`）、`application/publication_draft.go`（`declarationsOfContent`）；`adapters/http/publication_draft_payload.go`、`publication_draft_payload_customer_contract.go`；`cmd/parcel-commercial/translate.go`（`deliveryConditionDocument`）、`publish_batch_test.go`（`deliveryConditionBatchBody`）；`apps/admin-web/src/pages/party/{service-product-form.ts, customer-contract-form.ts, acceptance-rule-package-form.ts（sectionDeclared）, api.ts}`；`ports.ServiceProductCatalogueRow` / `CustomerContractCatalogueRow` / `PreAcceptanceControlRow`；`adapters/postgres/operations_catalogue.go`；`adapters/http/query_commercial_policies.go`；票 [09](./09-service-product-version-form.md)「本册规范化判断」与评审非阻断 (1)、[10](./10-customer-contract-form.md)、[13](./13-pre-acceptance-financial-control-policy-form.md)（评审补记「跑含 DSN 的 cmd 层发布用例」）、[18](./18-customer-service-rule-form.md)（材料多行文本选形；Go 侧四件的读法）、[06](./06-pre-acceptance-financial-control-policy-versions-have-no-read-face.md) / [21](./21-customer-service-rule-register-read-face.md)（政策页加一册的做法）、伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 硬句与 Status 口径。

## Comments

- 2026-09-10 · 通道 3（task-910498f2，取证锚 `93349f83`；分支 `mcp3-awf25`）：立票。**只写票面，未动代码。** 能力边界：读过 pc-gaps/11 完成记录全文、ADR-0133 全文、`delivery_condition.go` 全文、`publication_canonicalization.go` 全文与服务产品 / 客户合同两份规范化文件全文、`publish_commercial_authority.go` 的 `CommercialDeclarations` 与 `publicationContentOf`、`publication_draft.go` 的 `declarationsOfContent`、`publication_draft_payload.go` 与 `_customer_contract.go` 全文、`translate.go` 批文节、`publish_batch_test.go` 的交付条件夹具、`service-product-form.ts` / `customer-contract-form.ts` 全文、`acceptance-rule-package-form.ts` 的 `sectionDeclared`、`api.ts` 三个 Record 与三条读路径、`query_commercial_policies.go` 的 kind 分派与 `servePreAcceptanceControls`、`ports.go` 三个目录行、awf/09 / 13 / 18 全文与伞票 07 Status 口径；**没读** `operations_catalogue.go` 两条查询的 SQL 全文、`ServiceProductPublicationForm.tsx` / `CustomerContractPublicationForm.tsx` 的 JSX、`publication-form-rendered-paths.test.ts`——「要裁的」1 两条路的件数按接口与端点形状估，开工时以代码为准。派单点名的两问核过都**不是**裁决：交付方式怎么填——pc-gaps/11 已裁开放引用，表单照 awf/18 多行文本一行一项、不自造词表；对账门怎么开——见「本册规范化判断」，本票避得开伞票那道题。
