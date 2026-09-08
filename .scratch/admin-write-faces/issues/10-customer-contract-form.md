# 10 `CUSTOMER_CONTRACT` 版本的运营主路径：逐字段表单（正文 + 按费用范围的控制约定可加行 + 合同级控制声明）

Category: enhancement
Status: resolved——2026-09-08 15:0x，MCP-4（task-39a6f0cd）在分支 `mcp4-awf10`（基 main `0ef63897`）五笔完成（其中一笔是借通道 5 公共半边 `283aa7d3` 的 cherry-pick），逐笔 SHA 与验证强度见文末「完成记录」；main 上的 SHA 待非作者评审与重放后对照。完成判据三条全落：页面一签五步走通、tsc / run-tests 绿、Go 侧只加本册一格。此前 in-progress——14:3x 通道 4 认领，Go 先做，表单本体做成纯函数，流程组件等通道 5 的「[公共半边落点]」广播再接。此前 ready-for-agent——形状已裁清（逐字段表单 + 绑定表可加行，两层声明分两节；本票无待裁问题），Blocked by 08 已于 2026-09-08 resolved；伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 拆出（MCP-6 2026-09-07，锚 `95182b9d`；接管会话 2026-09-07 于 `92579b0a` 逐句核过票面，见伞票 Comments）
Blocked by: 08

## 册与载荷

显示在**客户与合同页**。版本壳之外，`declarations` 里两格归它：`contractContent{rulePackage, bindings[{chargeScope,
policy | inapplicabilityBasis}]}`（0012 正文：指名接单规则包；按费用范围指名一份接受前财务控制策略**或**给出显式
不适用依据，二者恰一，库上 CHECK）与 `preAcceptanceControl{requirement, notApplicableBasis?}`（0007 合同级
「要不要」声明，依据只在`不适用`时在场）。

## 选形与理由（ADR-0101 决定八）

**逐字段表单，绑定表可加行。** 频次低、配置员操作、正文是几个引用 + 一张几行的约定表，不是矩阵。表单：
规则包引用（从接单规则包册选）、绑定行（费用范围 × 「指名策略 / 显式不适用」二选一：策略从接受前财务控制策略册
选——票 [06](./06-pre-acceptance-financial-control-policy-versions-have-no-read-face.md) 落地后那本册有了）、合同级
控制声明（要求 / 不适用 + 依据）。**恰一与「不适用必带依据」两条由服务端裁**，表单用二选一控件呈现但不代判：
两格都空提交上去，答的是构造门的拒绝，不是表单的静默补齐。

## 硬句

伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md)「硬句」一节逐字适用（不在此复述，那一节是唯一口径）；另两条本册特有：「明确无控制」只能经合同两层声明表达（ADR-0115 Decision 一），表单不得在
策略侧给出「无控制」选项；`preAcceptanceControl` 与 `contractContent.bindings` 是两层（合同级「要不要」与按范围
「用哪份 / 不适用」），表单分两节、不合并。

## 完成判据

客户与合同页多一签「发布合同版本」（表单 → 预览摘要 → 待批准 → 批准 → 发布），结果在同页目录读面（含绑定列）
立刻可见；tsc / run-tests 绿；Go 侧只加本册规范化一格。

## 边界

不动 0007 / 0012；不动闭包解析。

## 完成记录（2026-09-08，MCP-4；分支 `mcp4-awf10`，基 main `0ef63897`；main 上的 SHA 由进 main 记录补）

**逐笔（分支 SHA）**：

| 分支 SHA | 内容 |
|---|---|
| `fe8a522f` | 领域：客户合同接进 PCC-1（加册不换号）。`PublicationContent` / `canonicalPublicationDocument` 各加 `CustomerContract` 一格，`CanonicalizePublicationContent` 一支、`IsRegisterCanonicalized` 一册、`RehydratePublicationContent` 一支；新文件 `publication_canonicalization_customer_contract.go`：`CustomerContractBody`（规则包 × 约定表 × 合同级声明可缺）、`PreAcceptanceControlBody`，canonical 文档键名镜像批文 `contractContent` / `preAcceptanceControl`，约定表按费用范围排序写出。`NewCustomerContract` / `DeclarePreAcceptanceControl` 的组合判抽成 `declaredBindingsByScope` / `preAcceptanceControlDeclared`，预览的 `validate` 与发布走同一道门；新增 `PreAcceptanceControlRequirementNamed` |
| `ab771dec` | 应用层双向：`publicationContentOf` 一 case（正文在不在场看 contractContent 那一层；只带合同级声明的批文项照旧登记声明的串）、`declarationsOfContent` 一支（载体正文折回两通道，声明缺席不代填）。**seed** `publish-batch.json` 的 `SYN-CONTRACT-01/v1` 的 `contentDigest` 换成算出的 `PCC-1:06de9a9c…`（票 08 未做「seed 摘要换算（各子票）」本册那一份）。`TestContractContentMustAgreeWithTheShellReference` 改用算出的摘要 |
| `4e9d0970` | 传输面：`CommercialPublicationPayload.CustomerContract` 一格 + `Publication()` 一段；新文件 `publication_draft_payload_customer_contract.go`：`CustomerContractBodyPayload{contractContent, preAcceptanceControl?}`，约定行恰一在解码点名 `bindings[i]`，配对与同范围重复由领域门在预览上答`未受理`带成因 |
| `1e7b9f41` | **借的**：通道 5 公共半边 `mcp5-awf16@283aa7d3` 的 cherry-pick（四口客户端、五步状态机、`PublicationDraftFlow`），内容与作者不动；重放时推送方跳它 |
| `3f4d4b53` | 管理台：`publication-draft-api.ts` 加 `customerContract` 一格 + 载荷类型；新文件 `customer-contract-form.ts`（+ .test.ts，草稿 → 载荷纯函数、认领路径）与 `CustomerContractPublicationForm.tsx`（版本壳 / 合同正文 / 合同级声明三节，约定表可加行，两册候选走既有 `listCommercialPolicies`，读不到退回手填）；`PartyContractsPage.tsx` 加「发布合同版本」签，载体到发布那一步合同表重读 |

**载荷形状（各册对照用）**：`customerContract: { contractContent: { rulePackage, bindings?: [{ chargeScope, policy? | inapplicabilityBasis? }] }, preAcceptanceControl?: { requirement, notApplicableBasis? } }`——一格两层，内层键名 = 受控批文 `declarations` 下两键；规则包同时进壳上 `references.ACCEPTANCE_RULE_PACKAGE`，两处相等由服务端核。

**完成判据逐项**：

1. 客户与合同页多一签「发布合同版本」，五步由 `PublicationDraftFlow` 走；载体到达发布那一步时 `onPublished` 拨 `publishedKey`，`CustomerContractsTable` 重读（含绑定列）——`3f4d4b53`。四口今天挂 `UnconfiguredIntake{}`，页面答 403 是诚实答案（ADR-0085 两阶段），组件如实显示。
2. `tsc --noEmit` 退 0；`run-tests` 92 pass / 0 fail（含本册 7 条）——钉 `3f4d4b53`，隔离 detached 检出 `%TEMP%\idp-verify-awf10`。
3. Go 侧本册规范化一格：共享文件只加本册那几行（两个 Go 接线文件 + 应用层两处代码注释自己点名「各册在此加一分支」的函数），本册正文 / 文档 / 载荷节全在新文件。

**硬句对照**：表单不算摘要、不裁任何门——`customer-contract-form.ts` 没有任何领域校验，二选一控件只决定送哪一格，没选 / 选了没填照样送，答回来的是构造门对那一行 / 那一节的拒绝；「恰一」在解码点名 `bindings[i]`，「不适用必带依据」与同范围重复由 `CustomerContractBody.validate` 经与发布同一道门在预览上答；策略侧无「无控制」选项（行的二选一是「指名策略 / 显式不适用」，合同级是「要求 / 不适用」，ADR-0115 Decision 一）；两层分两节。

**验证强度（钉 `3f4d4b53`，隔离 detached 检出）**：`gofmt -l .` 空；`go build ./...` / `go vet ./...` 退 0；含 DSN `go test -p 1 -count=1 ./internal/partycommercial/... ./cmd/parcel-commercial/ ./cmd/parcel-api/ ./internal/architecture/` 全 ok；探针 `cmd/parcel-api -run TestTheWiredPublicationDraftPath` 含 DSN **PASS**；PC postgres 包含 DSN `-v` **189 PASS / 0 SKIP**（59.7s）。未跑全仓 `./...`（本机同刻另有通道在跑，全仓由推送方在 tip 上兑底）。证据层级 **S**（隔离合成）。

**实施中量到的三处（记下让 09 / 11 及后续各册别再撞）**：

- **seed 摘要要随册换算，且换了之后老演示库重放会答 `CONTENT_CONFLICT`**：一册接进规范化后，`publish-batch.json` 里该册带正文的项若仍是 `sha256:syn-…` 会被对账门 `NOT_ACCEPTED`（seed.sh 跑不过），所以本册把那一项换成算出的串；已用旧串施加过的演示库要重建再 seed。供应商协议等册若 seed 里带正文声明，接进规范化时同样要换。
- **「正文在不在场」按册要单独裁**：本册两层里 `contractContent` 是 0012 正文、`preAcceptanceControl` 可缺；只带合同级声明的批文项没有可比对象、照旧登记声明的串——这是既有批文语义，不是漏堵。
- **合同级声明一节在表单上永远送**：「本版不作声明」在批文里是合法缺键，但运营主路径上不由表单替人默认，没选就送空要求让服务端答「集合外的控制要求」。

**未做（各归其票）**：`cmd/parcel-commercial/translate.go` 的 `controlRequirementFrom` / `controlBindingFrom` 与领域新加的 `PreAcceptanceControlRequirementNamed` / `financialControlBindingOf` 是同一条规则的两份（批文翻译层不在本票地盘，合并归 CLI 侧收口）；旧式 `sha256:` 声明串何时开始拒收（伞票收口时裁）；接入渠道未配置前四口答 403，点亮归装配（PAR-INT-01）。
