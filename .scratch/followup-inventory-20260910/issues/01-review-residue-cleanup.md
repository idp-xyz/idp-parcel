# 01 评审痕迹清剪：七处注释里的过时计数、序位引用与复述，不改一行行为

Category: enhancement
Status: in-progress——2026-09-10 15:3x 通道 5 立票即认领（单 task-14712757，通道 1 派；分支 `mcp5-residue`，隔离树 `D:/tops/idp-parcel-mcp5-residue`，基 main `93349f83`）。理由：零待裁——七处全是各票非作者评审已点过、票已 resolved、只剩注释没改的，见 [report.md](../report.md)「三、痕迹可剪」，取证锚 `c7e3522c`。
Blocked by: 无

## 缺口（锚 `93349f83`）

七处都在 `c7e3522c` 上取过证，`93349f83` 上复核仍在。每处：文件 · 原句 · 谁点的 · 为什么过时。

1. `migrations/party_commercial/0028_publication_draft_and_approval_duty_rule.sql` 头注「正文快照（content_document）就是服务端规范化文档（PCC-<n>）的字节：它恰是 content_digest 盖住的那些」——awf/08 完成记录：「迁移头注『就是……的字节』与此不符，日后谁在 SQL 侧直接哈希列会对不上——改注释或改 `bytea`」。过时点：列是 `jsonb`，jsonb 重排键序，存下的字节不再是算摘要的那串。
2. `apps/admin-web/src/pages/party/PreAcceptanceFinancialControlPolicyPublicationForm.tsx` / `SettlementPolicyPublicationForm.tsx` / `SupplierAgreementPublicationForm.tsx` 头注「十册没有一类是矩阵」——awf/13 评审 Standards 非阻断 (1)：「数的是 `CommercialObjectKind` 封闭集（别处的东西）」。封闭集加一册，这句无声变假。
3. 同上三张 + `PricePolicyPublicationForm.tsx` + `apps/admin-web/src/pages/pricing/SeriesRegistrationForm.tsx` 头注「决定一那三条的直接读数」——awf/15 评审 Standards 非阻断 (2)：「数了 ADR-0101 里的条目（AGENTS「不用计数」）」。`SeriesReviewPanel.tsx` 那句「这三条上的直接读数」前一句已把三条判据逐字点名，数的是本注释里的东西，不在此列。
4. `apps/admin-web/src/pages/party/policy-rows.test.ts` 两处夹具头注：「`TestPoliciesEndpointTranscribesCustomerServiceRuleContentOnlyWhenRegistered` 钉住的三行」（awf/21 评审 Standards 非阻断 (1)：跨文件计数，且夹具非照抄）与「价格政策册那三行的形状」（awf/14 评审 Standards 非阻断 (1)）。后端测试加减一行，这里就错。
5. `apps/admin-web/src/pages/party/PublicationFormFields.tsx` 头注「代价是同形副本九份，五张票的评审都点了同一条 Duplicated Code」与 `publication-form-shared.ts` 头注「代价是同形副本九份、改一处显示规则要改九处」——awf/22 评审 Standards 非阻断 (5)：「属 AGENTS「不用计数」字面（数已冻结、风险零，建议改措辞或锚 `3d90130c`）」。
6. `internal/parcelshipment/ports/delivery_place_reference_view.go` 头注「ADR-0130 决定二与 Consequences 第一条」、`commercial_resolution_reference_view.go` 头注「ADR-0133 决定二与 Consequences 第一条」——psr/06 评审判断题 (2)、psr/07 评审非阻断 1：「按序位指别的文件里无标签的列表项……序位与行号同构（插一条就无声指错）」；psr/07 评审说「随 tf/12 / tf/14 一并改成引文」，tf/14 已进 main（`5dda0fb2`）没顺手改。
7. `internal/parcelshipment/application/submit_shipment_request.go` `blockedOutcome` 头注「评估未决——部分确认、超时、查询不可用、失败、通道未配置、范围不符——」——wbr/01 评审 Standards 非阻断 ①：「各自复述 domain `HandoffObservation` 的格名与第六格证据形——AGENTS「一决策一处定义、只引用不复制」」。domain 加一格，这里少一格。

## 完成判据

1. 七处各改成不含计数 / 序位 / 复述的写法：序位换引文或符号名；计数换成不随别处增减而变假的句子；复述 domain 各格的换成指向 domain 符号名的一句；与实现不符的改准。改法照 AGENTS.md「改文档」那条。
2. 第 1 处先查 `internal/platform/migrate` 对已施加迁移的校验和纪律；若改注释会变校验和、或仓内纪律不许碰已施加迁移，那一处**不剪**，本票写「不剪，理由」，算完成。
3. `git diff` 里除注释与测试描述串外零行代码变化：Go 只改注释，TS 只改注释；若某处要改标识符才能消计数，不改，票面记。
4. 验证：gofmt 空；`go build ./...` / `go vet ./...` 0；`go test -count=1` 动过注释的包 + `./internal/architecture/...`；admin-web `tsc --noEmit` 退 0、`node scripts/run-tests.mjs` 全 pass。

## 边界

只动上列文件的注释。不动 `apps/admin-web` 任何组件行为、不动 PS 端口签名、不动迁移 SQL 本体；不碰同期在途的 `internal/parcelshipment/adapters/partycommercial/**`、`cmd/parcel-api/**`（通道 2 psr/03）与 admin-web 新表单文件（通道 3）。
