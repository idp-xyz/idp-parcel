# 26 awf/25 两份评审留下的可改项：方式聚合序钉 `COLLATE "C"`、渲染表独立列出、票面路径表去 `.methods`、前端头注不描述后端 SQL、钉两串字面 PCC-1 摘要

Category: chore
Status: resolved——**2026-09-10 17:4x 进 main**（推送方通道 1 重放；非作者评审由推送方自跑两轴 0 阻断——四个空闲通道都在写 ADR-0134–0137、`/code-review` 子代理仍报认证错；分支→main 对照与验证见文末「进 main 记录」）。此前 2026-09-10 17:2x 通道 3 交付（task-87e19668；分支 `mcp3-awf26` 基 `66cad4c4`，代码 tip `a30f6d67`，逐笔 SHA 与验证强度见文末「完成记录」；main 上的 SHA、非作者评审结论待推送方重放后补「进 main 记录」）。五项全落：① 方式聚合 `COLLATE "C"` + 真库一例；② 渲染表照 JSX 独立列出 + 变异红一次；③ 票 25 路径表去 `.methods`；④ `ServiceProductsPage.tsx` 头注改口；⑤ 两串字面 PCC-1 钉住 + 变异红一次。带 DSN 8 包 ok / PASS 1678 / SKIP 0；tsc 0 / run-tests 214。此前 in-progress——2026-09-10 17:0x 通道 3 认领（task-87e19668；分支 `mcp3-awf26` 接着立票笔 `1254f8f0` 开，基 `66cad4c4`）。同笔：通道 1 17:0x 裁「要裁的」1 取默认（不动，见「要裁的」下「裁决」）并补一项 ⑤（钉两串字面 PCC-1 摘要，通道 4 Spec 非阻断 (1)），本票再无待裁问题。此前 ready-for-agent——2026-09-10 16:5x 通道 3 立票（取证锚远端 main `66cad4c4`，即 awf/25 进 main 那一笔之后；分支 `mcp3-awf26`）。来源是票 25 Comments 里通道 4（Go 半边）与通道 5（admin-web 半边）两份非作者评审记为「非阻断 · 可改」的那几条，推送方在票 25「进 main 记录」尾点名可攒一张小票、由作者定——定为立。全是收口：不改任何领域规则、不新增能力、不改任何行为；一条「要裁的」带默认（不裁则不动），不挡开工。每项都能各自成笔、各自验。
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

### ⑤ 钉两串字面 PCC-1 摘要（通道 4 Spec 非阻断 (1)；通道 1 17:0x 补）

票 25 红线「PCC-1 不换号」今天由 `TestServiceProductWithoutDeliveryConditionsStillCanonicalizesToTwoFields`（nil 正文 ≡ 零值正文且恰两键）与 `TestCustomerContractDeliveryConditionsAreAThirdOptionalLayer`（整键缺席）从结构推出——「只加 omitempty nil 指针、省键时字段序无关」——没有一条钉住本节存在之前就取过证的字面串。改法：在 `publication_canonicalization_delivery_condition_test.go` 加一例两串：不带交付条件的服务产品两格文档、不带 `deliveryConditions` 的客户合同正文（夹具照既有 `customerContractBody(t, requiredControl(), appliedControl(t, "charge-prepaid"))`，全合成），各钉一串**在 `66cad4c4` 上算出的字面 `PCC-1:<hex>`**；测试头注写明「这两串变了就是换号，要走 ADR-0126 的换号路，不是改断言」。把「不换号」从结构推论变成断言，此后谁改规范化都会当场红。

## 要裁的

1. **① 的另两处要不要一并加。** 同文件 `ListAcceptanceRulePackages` 那条查询里 `ORDER BY source.source_kind` 与 `ORDER BY ref.rule_reference` 是同形先例，评审原话「属仓级判断」。**默认：不动**——本票只改交付条件这一处，那是票 25 自己许过的「同序」；派单方裁「加」就同笔带上、完成记录写明，并跑过 `ListAcceptanceRulePackages` 的真库测试。不裁按默认走，不挡开工。
   - **裁决（通道 1 · 2026-09-10 17:0x）：取默认，不动。** 理由：那两处没有一句票面要求「与领域同序」，改它要先核那一册领域有没有排序不变式，不是本票的题；完成记录点名「同形先例待另票」即可。

## 完成判据

1. ① 真库测试一例（在 `operations_catalogue_delivery_condition_test.go` 加一子例即可）：方式引用大小写混排（合成串，如 `METHOD/b` / `METHOD/A` / `method/a`），目录读回的 `Methods` 序 == 领域 `Methods()` 序（Go 字节序）；带 DSN 跑 `-v` 是 PASS 不是 SKIP。若测试库默认 collation 本来就是 `C`，这条改前就绿不算错——它钉的是「不随库设置变」，完成记录写明改前是红是绿。
2. ② `deliveryConditionRenderedPaths` 函数体不再调用 `deliveryConditionFieldPaths`；`publication-form-rendered-paths.test.ts` 与 `delivery-condition-section.test.ts` 全绿；变异一次：在渲染表里漏掉 `methods[i]`（或任一条认领过的路径），`publication-form-rendered-paths.test.ts` 要红，还原后绿。**变异落在渲染表上、不落在 JSX 上**：那份测试比的是两份 TS 声明（认领表 / 渲染表），它的头注自己写明 JSX 与渲染表的一致靠改 JSX 的人同步改声明与非作者评审对照——单改 JSX 红不出来，不是本票能改的，也不是缺。
3. ③ 票 25 ① 那句里不再出现 `.methods`（键本身），`.methods[i]` 保留；票 25 Comments 多一行。
4. ④ `ServiceProductsPage.tsx` 那段头注不再含表名 `service_product_form` 与「左连接」；tsc 0。
5. ⑤ 一例两串在场，串是 `66cad4c4` 上算出的字面 `PCC-1:<hex>`（完成记录写明在哪个检出算的）；变异一次：临时给服务产品两格文档多一键（或改一个键名），这一例要红，还原后绿。
6. 验证：`gofmt -l` 空、`go build ./...` / `go vet ./...` 退 0；`go test -count=1` 跑 PC 四包（`domain` / `application` / `adapters/http` / `adapters/postgres`，最后一个带 DSN）+ `./internal/architecture/...`；admin-web `tsc --noEmit` 退 0、`run-tests` 全绿。
7. 完成记录逐笔 SHA、逐项对上面五条，写明「要裁的」1 按裁决未动、② 那两条 deepEqual 怎么处置。

## 边界

只动：`operations_catalogue.go`（① 那一处 `ORDER BY`；另两处已裁不动）及其真库测试；`delivery-condition-section.ts` / `delivery-condition-section.test.ts`；`ServiceProductsPage.tsx` 那一段头注；票 25 一句 + Comments 一行；`publication_canonicalization_delivery_condition_test.go`（⑤ 只加测试）。**不动**：领域包的非测试文件、`ports.go`、http 层、迁移、`DeliveryConditionFields.tsx` 的 JSX（改法是让渲染表照 JSX 抄，不是反过来改 JSX）、`PublicationFormFields.tsx` / `publication-form-shared.ts`。不改任何行为：① 只钉序，② 只换列法，③④ 只改文字。

## 参照

- 票 25 Comments：通道 4 评审 Standards (1)、通道 5 评审 Standards (1) / (3) 与 Spec (1)、「进 main 记录」尾「评审非阻断随票记」一段——四条的原始判据都在那里，本票只引不复述。
- 票 22 判据 3（渲染表与认领表独立、由 `publication-form-rendered-paths.test.ts` 比对）。
- pc-gaps/11（交付方式是开放引用、不立词表——① 为什么会有大小写 / 非 ASCII 的可能）。

## 完成记录（2026-09-10，通道 3 一任会话；分支 `mcp3-awf26`，基 main `66cad4c4`；task-87e19668）

**逐笔（分支 SHA；main 上的 SHA 由进 main 记录补）**：

| SHA | 内容 |
|---|---|
| `1254f8f0` | 立票（四项，ready-for-agent，一条要裁的带默认） |
| `1acf5066` | 票面转 in-progress；同笔记通道 1 裁决（要裁的 1 不动）与补项 ⑤ |
| `bd6ba2b7` | ⑤ `publication_canonicalization_delivery_condition_test.go` 加 `TestDigestsPinnedBeforeTheDeliveryConditionSectionsStillHold`：两串字面 `PCC-1:<hex>` |
| `d7a6d4ce` | ① `operations_catalogue.go` `deliveryConditionColumns` 的方式 `ORDER BY` 加 `COLLATE "C"`、头注写理由；`operations_catalogue_delivery_condition_test.go` 加 `TestServiceProductCatalogueOrdersMethodsByBytesRegardlessOfCollation` |
| `f88680d5` | ② `deliveryConditionRenderedPaths` 照 JSX 独立列出、按 JSX 顺序；`delivery-condition-section.test.ts` 两条 deepEqual 改集合比对 |
| `814450f5` | ④ `ServiceProductsPage.tsx` 列定义头注改口（语义与代码未动） |
| `a30f6d67` | ③ 票 25「做法 ①」路径表去 `.methods` + Comments 一行 |
| 本笔 | 票面完成记录 + Status resolved；顺手把票 25 那行 Comments 的时刻从「17:2x」改准为「17:1x」（写时按估、未看钟） |

**逐项对完成判据**：

1. ① 真库一例在场（`METHOD/A` / `METHOD/_x` / `METHOD/b` / `method/a`，读回序 == `content.Methods()` 序 == 字节序），带 DSN `-v` PASS 非 SKIP。**改前就绿**——本机测试库 `compose.yaml` 以 `--locale=C.UTF-8` 起，`template1` 与每用例库 `datcollate` 均为 `C.UTF-8`，与字节序同；这条钉的是「不随部署库 collation 变」。取证（同一组串在测试库上跑 `string_agg(... ORDER BY m COLLATE …)`，2026-09-10 17:1x）：`"C"` 与库默认都是 `METHOD/A METHOD/_x METHOD/b method/a`；`"en_US.utf8"` 是 `method/a METHOD/A METHOD/b METHOD/_x`——不钉 collation，部署库若是 en_US 之类语言排序，目录序就与领域 `Methods()` 不同。**要裁的 1 按裁决未动**：`ListAcceptanceRulePackages` 里 `ORDER BY source.source_kind` / `ORDER BY ref.rule_reference` 两处同形先例待另票。
2. ② `deliveryConditionRenderedPaths` 函数体不再调 `deliveryConditionFieldPaths`；`publication-form-rendered-paths.test.ts` 与 `delivery-condition-section.test.ts` 全绿（214 / 214）。变异一次：渲染表里去掉 `methods[i]` 那行 → `run-tests` 3 fail（service product / customer contract 两条「都有处显」+ 本节集合比对那条），还原后 214 / 214。**那两条 deepEqual 的处置**：改成排序后比对——渲染表按 JSX 顺序（节根 → tightens 两格 → 方式每项 → 两条规则），认领表按载荷顺序，两表各写各的，比的是同一集合不是同一顺序；一句理由写在测试里。
3. ③ 票 25「做法 ①」那句不再含 `.methods`（键本身），`.methods[i]` 保留并补一句为什么只认领到项；票 25 Comments 多一行指向本票。
4. ④ `ServiceProductsPage.tsx` 那段头注 `git grep` 不再命中 `service_product_form` 与「左连接」；改成说 `ServiceProductRecord.deliveryConditions` 可缺、键缺席即未声明；tsc 0。
5. ⑤ 一例两串在场：服务产品两格文档 `PCC-1:fd0683ea642de60ddd1faee0d037dff9994f72bd2049155aa7b699c3fab9a641`、不带 `deliveryConditions` 的客户合同正文（夹具 `customerContractBody(t, requiredControl(), appliedControl(t, "charge-prepaid"))`）`PCC-1:c32a6c319174053d452007daf0ce0fdefd1027484281f43ea953527ecd4b8363`。**在哪算的**：分支树 Go 代码与 `66cad4c4` 逐字节同（写测试前 `git diff --stat 66cad4c4 -- internal/ cmd/ apps/ migrations/` 为空），先以占位串跑红两次取得实际值再钉。变异一次：`canonicalizeServiceProduct` 对 nil 正文临时交一节空 `serviceProduct` → 本例与 `…StillCanonicalizesToTwoFields` 同红（摘要变为 `PCC-1:4ceab9a5…`），还原后绿。头注写明「这两串变了就是换号，走 ADR-0126」。
6. 验证见下。
7. 本节。

**验证强度（钉 `a30f6d67`，在分支树 `D:/tops/idp-parcel-mcp3-awf26` 上跑；`status --untracked-files=all` 零行，`apps/admin-web/node_modules` 是指向共享树的 junction、被忽略，等价于干净检出）**：`gofmt -l ./internal ./cmd ./migrations ./tools` 零输出；`go build ./...`、`go vet ./...` 退 0；**带 DSN** `go test -p 1 -count=1 -v` PC 四包（`domain` / `application` / `adapters/http` / `adapters/postgres`）+ `./internal/architecture/...` + PC postgres 的反向依赖（`go list` 反查：`cmd/parcel-api` / `cmd/parcel-commercial` / `cmd/parcel-dispatch`）→ **8 包 ok / 0 FAIL，`--- PASS` 1678 / `--- SKIP` 0，29 秒**；探针 `TestAPublishedDeliveryConditionIsAnsweredByTheResolutionKeyedReadFace` PASS 非 SKIP。admin-web（分支树，junction 借共享树 `node_modules`）：`node node_modules/typescript/bin/tsc --noEmit` 退 0；`node scripts/run-tests.mjs` **214 / 214**（与 main 同数：本票只改既有测试、不加 TS 测试）。机制清点在同一检出重生成：**零 diff**（本票没有新增或删除文件、迁移、端口、端点）。未跑全量（作者范围口径）、未跑 `-race`。证据层级 **S**。占 / 释 55432 各两轮均广播。**时刻**：本记录前几条广播里写的「17:2x / 17:3x」是估的，本机钟当时在 17:0x–17:1x；本记录起按 `Get-Date` 写。

**触及**：上表八个文件。**未碰**：领域包非测试文件（⑤ 变异已还原，`git status` 核过）、`ports.go`、http 层、迁移、`DeliveryConditionFields.tsx` 的 JSX、`PublicationFormFields.tsx` / `publication-form-shared.ts`、`ListAcceptanceRulePackages` 两处 `ORDER BY`。

**给评审的判断题**：(1) ① 只钉交付条件这一处、另两处按裁决不动——同意否；(2) ② 渲染表按 JSX 顺序、测试改集合比对，而不是让渲染表照认领表的顺序排——哪种更贴票 22 判据 3「按 JSX 逐处抄」；(3) ⑤ 失败信息把处置指向「走 ADR-0126 的换号路」而不是「改断言」——口径对否；(4) ① 的真库一例改前就绿、红不出来是库 collation 决定的——记为「钉不变式」而非「红绿循环」可否接受；(5) ④ 头注改口后仍留「不以空列伪装已实现」那句——它说的是页面显什么，不是后端形状，留着对否。

## Comments

- 2026-09-10 16:5x · 通道 3（取证锚 `66cad4c4`；分支 `mcp3-awf26`）：立票。**只写票面，未动代码。** 能力边界：读过票 25 Comments 两份评审与「进 main 记录」全文、`deliveryConditionColumns` 与同文件另两处 `ORDER BY` 聚合、`delivery-condition-section.ts` 两个路径函数与头注、`serviceRuleRenderedPaths` 全文、`ServiceProductsPage.tsx` 那段头注、`api.ts` 各册「左连接」句、`declareDeliveryConditions` 的排序行；**没读** `DeliveryConditionFields.tsx` 的 JSX 全文（② 的逐处形以通道 5 评审人工核过的那份为据，开工时以 JSX 为准）、`operations_catalogue_delivery_condition_test.go` 现有子例的夹具形。
- **评审 ← 通道 1（推送方自评）· 钉 `05a240df` · 17:3x**（无空闲非作者通道：2 / 4 / 5 / 6 均在写 ADR-0134–0137，`/code-review` 子代理报认证错；按合入前评审那节「没有空闲通道时推送方自己跑」；读的是 `git diff 66cad4c4 05a240df -- internal/ apps/` 全文 150 行 + 票面；两轴串行）。
  - **Standards**：**阻断 0 · 非阻断 1**。(1) `operations_catalogue_delivery_condition_test.go` 新例头注写「本机测试库由 compose.yaml 以 C.UTF-8 起」——一条关于另一份文件的环境事实，compose 换 locale 时无人路过这句（与 AGENTS.md「计数与行号同构」同族：陈述别处的状态）；它解释了「改前就绿」，留着有用，可改成「取证时（`66cad4c4`）测试库是 C.UTF-8」把时点钉上。核过在场：注释全中文；跨文件引用全用符号名 / 票号；夹具全合成（`method/a` / `METHOD/b` / `METHOD/_x` / `METHOD/A` / `charge-prepaid` / `product-1`）；领域测试只 import 标准库 + `domain`；`delivery_condition.go` / `migrations/` / `cmd/parcel-api` / `PublicationFormFields.tsx` / `publication-form-shared.ts` 零 diff；`ListAcceptanceRulePackages` 两处 `ORDER BY source.source_kind` / `ORDER BY ref.rule_reference` 原样（tip 上 `Select-String 'ORDER BY'` 逐行核）。
  - **Spec**：**阻断 0 · 非阻断 1**。(1) ② 测试改集合比对后，`delivery-condition-section.test.ts` 不再钉渲染表的顺序；顺序由 JSX 决定、无机器比对，与票 22 那份测试「比两份 TS 声明、不渲染 JSX」是同一格空白（作者完工报 (b) 已如实写），非本票引入。**逐项**：① `COLLATE "C"` 只加在 `deliveryConditionColumns` 的方式聚合一处（tip 上全文件十六处 `ORDER BY` 只此一处带 collation）；真库例用大小写 + 下划线混排四串钉「大写 < 下划线 < 小写」，并先断言领域 `Methods()` 与字节序同（先例变了会先红在那一句）。② `deliveryConditionRenderedPaths` 独立列出：根、合同层 `tightens.objectId` / `tightens` / `tightens.version`、`methods[i]` 逐项、两条规则——与通道 5 评审人工核过的 JSX（root `Problems`、tightens 两 Field 带 alsoPaths、`methods[` 前缀汇显、两条规则 Field）逐处对得上，无漏无多；头注写明「写成一行别名会让那份比对恒真」。③ 票 25 路径表去 `.methods` 一项 + Comments 一行（3 行 diff）。④ `ServiceProductsPage.tsx` 头注不再提 `service_product_form` / 左连接 / 0030，改说照 `ServiceProductRecord.deliveryConditions` 显、键缺席显「未声明」。⑤ 两串是字面常量（`PCC-1:fd0683ea…` / `PCC-1:c32a6c31…`），分别覆盖服务产品两格文档与不带 `deliveryConditions` 的合同正文，失败信息与头注都把处置指向「走 ADR-0126 的换号路」。要裁的 1 按裁决未动。
  - **五道判断题**：(1) 同意；(2) 渲染表按 JSX 序、测试比集合更贴「按 JSX 逐处抄」——认领表按载荷序，两表本就不同序，硬对齐顺序反而让一张表照另一张抄；(3) 同意，处置指向换号路是对的；(4) 接受为「钉不变式」——测试名 `…RegardlessOfCollation` 与头注都说清了它守的是部署库换 locale 那一格；(5) 留着对，那句说的是页面显什么。**结论：可合入。**
- **进 main 记录 · 通道 1 · 2026-09-10 17:4x**：分支 `mcp3-awf26` 八笔在 `8313c23f` 上 cherry-pick 全干净——`1254f8f0→47802a95` / `1acf5066→a70fc772` / `bd6ba2b7→637e6326` / `d7a6d4ce→72a9447d` / `f88680d5→8e33e2f9` / `814450f5→61928a5e` / `a30f6d67→e6624896` / `05a240df→1ddaa384`（本票八件 `git diff 05a240df 1ddaa384` 零行）；`66cad4c4..8313c23f` 只动 `.scratch/**`，与本票零重叠。清点在 tip 重生成零差（无新增文件）。验证钉重放 tip（隔离 detached 树 `%TEMP%\idp-replay-awf26`）：`gofmt -l` 空、`go build ./...` / `go vet ./...` 退 0；**带 DSN** `go test -p 1 -count=1 ./...` **104 ok / 0 FAIL / 15 无测试 / 0 cached · 126 s**；admin-web `tsc --noEmit` 退 0、`run-tests` **214 / 214**（junction 借共享树 node_modules、验完拆）。本笔（本条 + 评审 + Status 行 + tasks.md）在重放 tip 之上，纯 .md；`ls-remote` 核 `8313c23f` 未动后 ff 并 `push <sha>:main`。分支指针改 `merged/mcp3-awf26`、远端删；`D:/tops/idp-parcel-mcp3-awf26` 由作者拆（先 `cmd /c rmdir` node_modules junction）。两条非阻断随票记不另立票。
