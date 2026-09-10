# 26 awf/25 两份评审留下的可改项：方式聚合序钉 `COLLATE "C"`、渲染表独立列出、票面路径表去 `.methods`、前端头注不描述后端 SQL

Category: chore
Status: ready-for-agent——2026-09-10 16:5x 通道 3 立票（取证锚远端 main `66cad4c4`，即 awf/25 进 main 那一笔之后；分支 `mcp3-awf26`）。来源是票 25 Comments 里通道 4（Go 半边）与通道 5（admin-web 半边）两份非作者评审记为「非阻断 · 可改」的那几条，推送方在票 25「进 main 记录」尾点名可攒一张小票、由作者定——定为立。全是收口：不改任何领域规则、不新增能力、不改任何行为；一条「要裁的」带默认（不裁则不动），不挡开工。每项都能各自成笔、各自验。
Blocked by: 无（25 已 resolved 且进 main）

## 从哪里来

票 25 进 main 前的两份评审四轴 0 阻断。非阻断里评审自判「无需动作」的——`publication_canonicalization_customer_contract.go` 头注的计数冗余、`DeliveryConditionsCell.tsx` 的 `key={method}`、`deliveryConditionDeclared` 的 trim 口径、`declaredAt` 进 Record 不显、PCC-1 字面摘要串未钉——**不在本票**，它们在票 25 Comments 里各有一句理由，本票不重开。剩下的是评审点名「加一句即可钉死」「与同仓先例不同形」「票面与实现对不上」这三类，各自很小，攒在一起省一次派单。

## 做法（锚 `66cad4c4`，逐符号名）

### ① 方式聚合序钉 `COLLATE "C"`（通道 4 Standards 非阻断 (1)）

`internal/partycommercial/adapters/postgres/operations_catalogue.go` 的 `deliveryConditionColumns` 里，方式子查询 `json_agg(method.method_ref ORDER BY method.method_ref)` 按库默认 collation 排；领域 `declareDeliveryConditions`（`delivery_condition.go`）对方式用 Go 字节序 `sort.Slice`，`DeliveryConditionContent.Methods()` 交回的就是那个序。票 25 ② 许的是「稳定序，与领域 `Methods()` 同序」——方式引用是开放引用（pc-gaps/11 已裁不立词表），大小写混用或含非 ASCII 时两边可以不同序，目录读回与规范化正文就对不上。改法：`ORDER BY method.method_ref COLLATE "C"`，一处。

### ② `deliveryConditionRenderedPaths` 独立列出（通道 5 Standards 非阻断 (1)）

`apps/admin-web/src/pages/party/delivery-condition-section.ts` 的 `deliveryConditionRenderedPaths` 现在是 `return deliveryConditionFieldPaths(...)` 一行别名。票 22 判据 3 要渲染表「按 JSX 逐处抄」、与认领表**独立**，`publication-form-rendered-paths.test.ts` 靠两表比对抓 JSX 漂移——别名化后这一节的比对恒真，`DeliveryConditionFields.tsx` 里 path 改了谁也不会红。同仓其余 `*RenderedPaths` 都是独立列的；awf/18 的 `serviceRuleRenderedPaths`（`customer-service-rule-form.ts`）与本节同形（材料清单 / 方式清单都是「只认领到项」），可照它的形写。改法：按 `DeliveryConditionFields.tsx` 的 JSX 逐处写——节根一处 `Problems`；`recipientScopeRule` / `proofOfDeliveryRule` 各一 Field；方式文本框下按项号汇显 `methods[i]`；合同层 `tightens.objectId` / `tightens.version` 各一 Field、`tightens` 根由对象标识那格 `alsoPaths` 代显。`delivery-condition-section.test.ts` 里「渲染表 deepEqual 认领表」那两条要么保留（渲染表顺序也照认领表排），要么改成集合比对，作者定、写一句理由。

### ③ 票 25 ① 路径表去掉 `.methods` 一项（通道 5 Spec 非阻断 (1)）

票 25「做法 ①」认领路径那句列了 `serviceProduct.deliveryConditions{,.methods,.methods[i],…}`，其中 `.methods`（键本身）实现有意不认领：`deliveryConditionFieldPaths` 头注写明「方式只认领到项，不认领 `methods` 这一键本身」，Go 侧 `publication_draft_payload_delivery_condition.go` 的 `body` 也只点名 `methods[%d]`。实现与 Go 一致，是票面多列一项，无运行时后果。改法：在票 25 那句里去掉 `.methods`，并在票 25 Comments 追加一行「路径表改口 ← 票 26」；票 25 别处不动。纯 .md，与代码笔分开成笔。

### ④ `ServiceProductsPage.tsx` 头注不描述后端 SQL（通道 5 Standards 非阻断 (3)）

`ServiceProductsPage.tsx` 列表列定义上方那段头注写「读面交回的就是 `service_product_form` 的版本行左连接 0030」——前端注释描述后端表名与连接形状，后端改查询写法时没人会路过这句。改法：只改这一句，改成说读面契约（`ServiceProductRecord.deliveryConditions` 可缺、键缺席即这一版没有声明，与 `api.ts` 上那条字段注释同口径），不点表名、不说连接。`api.ts` 里各册「壳 + 正文左连接」那类句子是仓内既有口径、说的是读面两层各自可缺，不在本票。

## 要裁的

1. **① 的另两处要不要一并加。** 同文件 `ListAcceptanceRulePackages` 那条查询里 `ORDER BY source.source_kind` 与 `ORDER BY ref.rule_reference` 是同形先例，评审原话「属仓级判断」。**默认：不动**——本票只改交付条件这一处，那是票 25 自己许过的「同序」；派单方裁「加」就同笔带上、完成记录写明，并跑过 `ListAcceptanceRulePackages` 的真库测试。不裁按默认走，不挡开工。

## 完成判据

1. ① 真库测试一例（在 `operations_catalogue_delivery_condition_test.go` 加一子例即可）：方式引用大小写混排（合成串，如 `METHOD/b` / `METHOD/A` / `method/a`），目录读回的 `Methods` 序 == 领域 `Methods()` 序（Go 字节序）；带 DSN 跑 `-v` 是 PASS 不是 SKIP。若测试库默认 collation 本来就是 `C`，这条改前就绿不算错——它钉的是「不随库设置变」，完成记录写明改前是红是绿。
2. ② `deliveryConditionRenderedPaths` 函数体不再调用 `deliveryConditionFieldPaths`；`publication-form-rendered-paths.test.ts` 与 `delivery-condition-section.test.ts` 全绿；变异一次：把 `DeliveryConditionFields.tsx` 里任一 Field 的 path 改坏（或在渲染表里漏掉 `methods[i]`），比对测试要红，还原后绿。
3. ③ 票 25 ① 那句里不再出现 `.methods`（键本身），`.methods[i]` 保留；票 25 Comments 多一行。
4. ④ `ServiceProductsPage.tsx` 那段头注不再含表名 `service_product_form` 与「左连接」；tsc 0。
5. 验证：`gofmt -l` 空、`go build ./...` / `go vet ./...` 退 0；`go test -count=1` 跑 `./internal/partycommercial/adapters/postgres/`（带 DSN）+ `./internal/architecture/...`；admin-web `tsc --noEmit` 退 0、`run-tests` 全绿。
6. 完成记录逐笔 SHA、逐项对上面四条，写明「要裁的」1 裁了什么（或按默认未动）、② 那两条 deepEqual 怎么处置。

## 边界

只动：`operations_catalogue.go`（① 那一处 `ORDER BY`；裁了才动另两处）及其真库测试；`delivery-condition-section.ts` / `delivery-condition-section.test.ts`；`ServiceProductsPage.tsx` 那一段头注；票 25 一句 + Comments 一行。**不动**：领域包、`ports.go`、http 层、迁移、`DeliveryConditionFields.tsx` 的 JSX（改法是让渲染表照 JSX 抄，不是反过来改 JSX）、`PublicationFormFields.tsx` / `publication-form-shared.ts`。不改任何行为：① 只钉序，② 只换列法，③④ 只改文字。

## 参照

- 票 25 Comments：通道 4 评审 Standards (1)、通道 5 评审 Standards (1) / (3) 与 Spec (1)、「进 main 记录」尾「评审非阻断随票记」一段——四条的原始判据都在那里，本票只引不复述。
- 票 22 判据 3（渲染表与认领表独立、由 `publication-form-rendered-paths.test.ts` 比对）。
- pc-gaps/11（交付方式是开放引用、不立词表——① 为什么会有大小写 / 非 ASCII 的可能）。

## Comments

- 2026-09-10 16:5x · 通道 3（取证锚 `66cad4c4`；分支 `mcp3-awf26`）：立票。**只写票面，未动代码。** 能力边界：读过票 25 Comments 两份评审与「进 main 记录」全文、`deliveryConditionColumns` 与同文件另两处 `ORDER BY` 聚合、`delivery-condition-section.ts` 两个路径函数与头注、`serviceRuleRenderedPaths` 全文、`ServiceProductsPage.tsx` 那段头注、`api.ts` 各册「左连接」句、`declareDeliveryConditions` 的排序行；**没读** `DeliveryConditionFields.tsx` 的 JSX 全文（② 的逐处形以通道 5 评审人工核过的那份为据，开工时以 JSX 为准）、`operations_catalogue_delivery_condition_test.go` 现有子例的夹具形。
