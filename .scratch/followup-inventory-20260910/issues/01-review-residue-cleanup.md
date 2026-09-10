# 01 评审痕迹清剪：七处注释里的过时计数、序位引用与复述，不改一行行为

Category: enhancement
Status: resolved——2026-09-10 15:5x 通道 5 完成（单 task-14712757；分支 `mcp5-residue` 基 main `93349f83`：`ae0b26b2` 立票 + report 尾行 / `13ea493d` 注释六处（11 文件 +25/−21，只改注释与测试头注）/ 本笔票面；每笔已推 origin，main 上的 SHA 由推送方重放后补）。七处里六处改成不含计数 / 序位 / 复述的写法，第 1 处（0028 迁移头注）**不剪**，理由见完成记录。验证：gofmt 空、`go build ./...` / `go vet ./...` 0、`go test -count=1` ports + application + architecture ok、admin-web tsc 退 0 / run-tests 198 pass、`tools/mechanism-inventory` 重生成零差。此前 in-progress——15:3x 通道 5 立票即认领（通道 1 派；隔离树 `D:/tops/idp-parcel-mcp5-residue`）。理由：零待裁——七处全是各票非作者评审已点过、票已 resolved、只剩注释没改的，见 [report.md](../report.md)「三、痕迹可剪」，取证锚 `c7e3522c`。
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

## 完成记录（2026-09-10，通道 5；分支 `mcp5-residue`，基 `93349f83`）

逐处原句 → 新句（都在 `13ea493d`）：

1. **不剪**。`migrations/migrations.go` 的 `checksumOf(content)` 对整份文件（含注释）算校验和，`internal/platform/migrate/runner.go` 的 `ErrChecksumDrift` 对「已施加迁移的校验和与当前工件不一致」阻断启动——改一行注释就会让所有已施加 0028 的库（含 55432 与各通道隔离库）在下次 `migrate` 时被拦。仓内先例：wbr/03 完成记录「`0020` 头注守 checksum 不改，那句改以 `credit_policy.go` 头注与 ADR-0127 为准」。awf/08 那句「与此不符」的正确口径已在 `domain/publication_canonicalization.go` `CanonicalizePublicationContent` 头注（「文档只盖正文」那段）与 PG 适配器写列处；日后若 0028 之上再立迁移碰这张表，可在新迁移头注里写明「`content_document` 是 jsonb、字节序不等于摘要输入」。
2. 三张表单头注「十册没有一类是矩阵，模板导入只针对上百格的价卡」→「本册正文不是矩阵，模板导入（决定二）只针对上百格的价卡」。句子只说本册，封闭集增减不影响它。
3. 四张 party 表单头注「决定一那三条的直接读数」→「决定一判据（登记频次 × 操作者角色 × 载荷结构）的直接读数」；`pricing/SeriesRegistrationForm.tsx`「判据是决定一那三条的直接读数」→「判据是决定一（登记频次 × 操作者角色 × 载荷结构）的直接读数」。点名判据本身，不数它们。`SeriesReviewPanel.tsx`「这三条上的直接读数」未动：前一句已逐字列出三条，数的是本注释里的东西。
4. `policy-rows.test.ts`「……钉住的三行：只有壳、……」→「……钉住的几种形状各取一行：……（夹具取形不照抄取值，那边加减行本文件不跟）」；「价格政策册那三行的形状（……）：带汇率……三处可缺的键」→「价格政策册各行的形状（……），每种形状取一行：……可缺的键」。
5. `PublicationFormFields.tsx`「十张表单全从这里导入」→「各册表单全从这里导入」；「让九张表单并行写……让九个会话撞同一个文件；代价是同形副本九份，五张票的评审都点了」→「让各册表单并行写……让并行的会话撞同一个文件；代价是每张表单各留一份同形副本，各票的非作者评审都点了」。`publication-form-shared.ts`「代价是同形副本九份、改一处显示规则要改九处」→「代价是每张表单各留一份同形副本、改一处规则要逐张改」。
6. `ports/delivery_place_reference_view.go`「ADR-0130 决定二与 Consequences 第一条」→「ADR-0130 决定二，与其 Consequences『PS `ports` 另立一个按（租户，包裹身份）的窄读口……不拓宽既有 `ShipmentRequestRepository` 一类写口』那条」；`ports/commercial_resolution_reference_view.go`「ADR-0133 决定二与 Consequences 第一条」→「ADR-0133 决定二，与其 Consequences『PS `ports` 另立一个按（租户，包裹身份）答商业解析回指的窄读口……一口一问、不合并；不拓宽既有写口』那条」。引文取自两篇 ADR Consequences 首项原句（省略号处是同一句的中段）。
7. `application/submit_shipment_request.go` `blockedOutcome` 头注「评估未决——部分确认、超时、查询不可用、失败、通道未配置、范围不符——」→「评估未决（哪几种算未决、各自的证据形由 domain.HandoffObservation 与 domain.HandoffUnresolvedReason 定，这里不复述）——」。

**没改标识符**：没有一处需要动到符号名才能消计数。**没碰**：`ports/ports.go`（wbr/01 评审同条点名的 `ProductionHandoffObservation` 头注，`93349f83` 上 `git grep 'type ProductionHandoffObservation'` 零命中，那只端口已换名或已并入别处，无可改）。

**验证**（隔离树 `D:/tops/idp-parcel-mcp5-residue`，`13ea493d` 内容）：`git diff -U0 -- '*.go' '*.ts' '*.tsx'` 过滤后非注释行零条；`gofmt -l ./internal/parcelshipment` 空；`go build ./...` 0；`go vet ./...` 0；`go test -count=1 ./internal/parcelshipment/ports/... ./internal/parcelshipment/application/... ./internal/architecture/...` ok（ports 无测试文件）；admin-web `node node_modules/typescript/bin/tsc --noEmit` 退 0、`node scripts/run-tests.mjs` pass 198 / fail 0（`node_modules` 走 junction 借共享树，验完 `rmdir`）；`tools/mechanism-inventory` 重生成 `docs/product/MECHANISM-INVENTORY.md` 零差。未用 55432。

## 进 main 记录 · 通道 1 · 2026-09-10 16:0x

分支 `mcp5-residue` 三笔在 `0e8d048a` 上 cherry-pick 全干净——`ae0b26b2→84a5f939` / `13ea493d→82a998c5` / `2a8df802→637d77da`；11 件代码文件 + 本目录两件 .md 与分支逐字相同。**推送方自审**（纯注释 / 测试头注，按 parallel-sessions「不评什么」不派评审）：`git diff -U0 0e8d048a..637d77da -- '*.go' '*.ts' '*.tsx'` 过滤注释行后零条。验证钉 `637d77da`（隔离 detached 树 `%TEMP%\idp-replay-residue`）：`gofmt -l` 空；`go build ./...` / `go vet ./...` 退 0；占 55432 广播后含 DSN `go test -p 1 -count=1 ./...` 退 0，**104 ok / 0 FAIL / 15 无测试 / 0 cached**（120 s）；admin-web `tsc --noEmit` 退 0、`run-tests` **pass 198 / fail 0**（junction 借共享树 node_modules，验完拆）；释号。本笔（本节 + tasks.md）在 `637d77da` 之上，纯 .md；`ls-remote` 核 `0e8d048a` 未动后 ff 并 `push <sha>:main`。分支指针改 `merged/mcp5-residue`、远端删、树由作者拆。0028 迁移头注「不剪」的理由（`checksumOf` 对全文件算校验和、`ErrChecksumDrift` 阻断启动、先例 wbr/03）成立，认可。
