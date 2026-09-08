# 11 `SUPPLIER_AGREEMENT` 版本的运营主路径：逐字段表单（正文四格 + 采购方案从价卡目录选）

Category: enhancement
Status: resolved——2026-09-08 14:5x MCP-6 交付（task-662822ef；分支 `mcp6-awf11` 基 main `0ef63897`，代码 tip `a2a44679`，逐笔 SHA 与验证强度见文末「完成记录」；main 上的 SHA、非作者评审结论待通道 1 重放后补「进 main 记录」）。完成判据三条全落：供应商协议页两签「协议目录 / 发布协议版本」，五步走公共半边、发布落定同页目录立刻重取；tsc 退 0 / run-tests 92/92；Go 侧本册接进 PCC-1 一格（连同应用层两个方向与载荷一格，08 完成记录点名的「各子票接进同一号」）。此前 in-progress——2026-09-08 14:2x MCP-6 认领（08 已 resolved，阻塞边解除）。此前 ready-for-agent——形状已裁清（逐字段表单，采购方案从价卡目录选；本票无待裁问题），Blocked by 08 未 resolved 前不在前沿；伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 拆出（MCP-6 2026-09-07，锚 `95182b9d`；接管会话 2026-09-07 于 `92579b0a` 逐句核过票面，见伞票 Comments）
Blocked by: 08

## 册与载荷

显示在**供应商协议页**。`declarations.supplierAgreementBody{supplier, legalEntity, scope, purchasePlan,
effectiveStartsAt, effectiveEndsAt?}`（0021 正文，票 pc-gaps/03）。

## 选形与理由（ADR-0101 决定八）

**逐字段表单。** 频次低、配置员操作、正文六格无子表。`purchasePlan` 是 parcel-pricing 的方案版本引用——表单要能
从价卡目录**选**而不是手抄（跨上下文只传引用；价卡目录读口已在管理台价卡页），选出来的仍是引用串，表单不读方案
内容。供应商与法人引用从主数据读面选。

## 硬句

伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md)「硬句」一节逐字适用（不在此复述，那一节是唯一口径）；本册无特有硬句。

## 完成判据

供应商协议页多一签「发布协议版本」（表单 → 预览摘要 → 待批准 → 批准 → 发布），结果在同页目录读面立刻可见；
tsc / run-tests 绿；Go 侧只加本册规范化一格。

## 边界

不动 0021；采购方案的方向与绑定换算是 `PRICE_RULE` 那册的事，不在这里出现。

## 完成记录（2026-09-08，MCP-6；分支 `mcp6-awf11`，基 main `0ef63897`；task-662822ef）

**逐笔（分支 SHA；main 上的 SHA 由进 main 记录补）**：

| 分支 SHA | 内容 |
|---|---|
| `c6028a2e` | 票面认领转 in-progress |
| `eadac23d` | 领域：供应商协议册接进 PCC-1 不换号。新文件 `publication_canonicalization_supplier_agreement.go` 放正文类型 `SupplierAgreementBody`（复用 `NewSupplierAgreement` 那几道构造门）、文档节 `canonicalSupplierAgreementBody`（键名镜像批文 `supplierAgreementBodyDocument`，无方向键）与折回；共享文件 `publication_canonicalization.go` 纯加行 |
| `33bda25f` | 应用：`publicationContentOf` 一 case（受控批文那一半的对账门对本册开门）+ `declarationsOfContent` 一支（载体正文交回既有发布用例）——两个方向同笔，照那两处代码注释的要求。既有用例 `TestASupplierAgreementBodyPublishesWithItsOwnVersion` 的壳改配算出的摘要（新 helper `supplierAgreementSpec`，判据同 `creditPolicySpec`） |
| `c4ca7004` | 传输面：`publication_draft_payload.go` 一格 + `Publication()` 一段；新文件 `publication_draft_payload_supplier_agreement.go` 放 `SupplierAgreementBodyPayload` 与 `body()`（问题落 `supplierAgreement.<键>`） |
| `f94c13a2` | merge 通道 5 公共半边 `mcp5-awf16@283aa7d3`——只为让表单对着真实落点编译与测试；那一笔归票 16，由 16 自己进 main，本分支重放时跳过 |
| `d756dd1e` | Web 纯逻辑 `supplier-agreement-form.ts` + node:test：草稿（壳五格 + 正文六格）→ `CommercialPublicationPayload`、认领路径与载荷键一一对应、`planReferenceOf`（`planId@planVersion`）、`withShellCopiedIntoAgreement`；`publication-draft-api.ts` 加 `SupplierAgreementBodyPayload` 一型 + `supplierAgreement` 一格 |
| `a2a44679` | Web 组件与页：`SupplierAgreementPublicationForm.tsx` 挂 `PublicationDraftFlow`，三样从读面选（`listBusinessParties` / `listGroupLegalEntities` / `listPriceCards`），读面 403 或读不到退回手填；`SupplierAgreementsPage.tsx` 两签，发布落定 `reloadToken` 递增刷目录 |

**完成判据逐项**：

1. 供应商协议页多一签「发布协议版本」：表单 → 预览摘要 → 存为待批准 → 批准 → 发布五步由公共半边 `PublicationDraftFlow` 走；载体到达发布那一步 `onPublished` 递增 `reloadToken`，同页目录读面立刻重取——`a2a44679`。今天四口挂 `UnconfiguredIntake{}`，页面如实显示 403（ADR-0085 两阶段），不假装可用。
2. tsc / run-tests 绿——见验证强度。
3. Go 侧本册规范化一格——`eadac23d`；要真正「接进同一号」还需应用层两个方向（`33bda25f`）与载荷一格（`c4ca7004`），三笔合为本册的机制半边。

**硬句在场**：表单不算摘要、不收也不送批准人（`supplier-agreement-form.test.ts` 钉载荷里无 tenant / submitter / approver / contentDigest / approval / canonicalization）；预览与录入同一份载荷、同一段解码、同一处算摘要（`TestSupplierAgreementPreviewAndSubmissionShareTheDigest`）；表单不裁任何门——连「必填」都不在本地拦，空字段、区间先后、引用在不在册一律送上去让服务端答，`localProblems` 不给（十一格全是文本，没有编不进载荷类型的格）。

**选形落地**：`purchasePlan` 从价卡目录选，取 `planId@planVersion`——与 `settlement-accounting` 拼方案版本引用同一写法（`buy_evaluation.go`），不按方向过滤、不读方案内容，目录行上的方向只显给人看；供应商与法人从主数据读面选；正文里没有方向格（领域钉死 BUY，载荷带 `direction` 按未知键拒）。壳上的范围与区间是版本的、正文里的是协议的，两样分开给，「从版本壳带入」只是抄一次文本。

**共享文件各改了哪几处（生产文件全部纯加行）**：`domain/publication_canonicalization.go`（+21/−0：版本号注释一行、`PublicationContent` 一格、kind 不符一句 + switch 一支、`IsRegisterCanonicalized` 一册、`canonicalPublicationDocument` 一格、`Rehydrate` 一支——早返回，正文缺席那句留在末尾不动）；`application/publish_commercial_authority.go`（+13/−0）；`application/publication_draft.go`（+10/−0）；`adapters/http/publication_draft_payload.go`（+6/−0）；`apps/admin-web/src/pages/party/publication-draft-api.ts`（+15/−0，对 `283aa7d3`）。测试文件 `application/publish_commercial_authority_test.go`（+20/−10）是甲路下必然的一改。

**验证强度（钉 `a2a44679`，隔离 detached 检出 `%TEMP%\idp-verify-awf11`，验后已拆）**：`gofmt -l .` 空；`go build ./...` / `go vet ./...` 退 0；含 DSN `go test -count=1 ./internal/partycommercial/... ./cmd/parcel-api/ ./internal/architecture/` 全 ok（postgres 包 62.6s，非缓存）；探针 `-v`：`TestPublicationDraftWritesRefuseToRunOutsideATransaction` **PASS**、`cmd/parcel-api -run TestTheWiredPublicationDraftPath` **PASS**（含 DSN）；admin-web `tsc --noEmit` 退 0、`run-tests` 92 / 92（含本票 7 条与并入的公共半边）。证据层级 **S**（隔离合成）。`.sql` 无变动、不加迁移；seed 一字不动（`publish-batch.json` 两份 `SUPPLIER_AGREEMENT` 只有壳，对账门对无正文的壳不开）。

**自审（`/code-review` 两轴，基线 `0ef63897`；子代理不可用改串行自跑）**：无阻断。判断类三条留给伞票收口、不在本票动：(1) `canonicalSupplierAgreementBody.body()` 与信用政策那一节的时刻解析同形，可抽成领域内一个区间折回助手——今天不动信用政策那段是为守共享文件纯加行；(2) `IsRegisterCanonicalized` 逐册 `if` 累到几册后宜改 switch；(3) `normalizeMoment` 从 `pricing/series-form.ts` 跨页导入，各册表单都要它，宜挪到共享处。非作者评审由通道 1 派，结论写「进 main 记录」。

**未做（各归其票）**：目录读面不显示 0021 正文列（读面票，本票只加写签）；五步状态机与各口答案中文归公共半边（票 16）；四口点亮归 `PAR-INT-01`（实例半边）。
