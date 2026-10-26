# 09 索赔口经首方渠道接通（并 demo-chain-gaps/102）

Category: enhancement
Status: in-progress——2026-10-26 作者完工，待隔离评审与进 main，分支 `claude-fpa09`（代码最后一笔 `507c981b1`）。此前 in-progress——2026-10-26 会话 `tops-20` 认领（06 已进 main），分支 `claude-fpa09`。此前 ready-for-agent——R1（上线前必修，用户 2026-10-26 定首发要这一口）。
Blocked by: 06
出处：用户 2026-10-26「都需要」（首个租户上线时客户经首方 API 要能撤回、取消、修订、补充、索赔）、「每个客户系统只代表它自己的账户（直客），只考虑这个吧」、「首发不要客户状态读口」、「首发不做限流」；技术取舍由会话 `tops-20` 代决（用户「你帮我决定吧」）。 计划见 [spec](../spec.md)「五口上线」与 [摸底报告](../customer-faces-scoping-2026-10-26.md)。

## 做什么

首方 `IntakeClaim`（VE）、Applicant/Contract 引用须属信封账户、（账户、批次、项）自然键（取舍 F）、`endpoints_claims.go` 换真 Intake。

## 完成判据

- [ ] 首方客户凭证经 `/v1` 该口提交，演示租户答成功；他账户目标答找不到、不泄露存在；未授予答 403；重放答同一结果（生产装配真库用例）。
- [ ] 新 AT 行与 AT 盘点①表行；Go 门禁；全量用 `scripts/test-full.sh`；种子 `seed.sh` 退 0。

## 规模

M

## 完成记录

起点 `4292299b8`；代码一笔 `507c981b1`。

**改了什么**：新 `CustomerClaimIntake`（`internal/visibilityexception/adapters/http/customer_claim_intake.go`）与 VE 认证器（`adapters/accessidentity/customer_claim_authenticator.go`，经 `MintClaim`，铸造侧判准入）；`endpoints_claims.go` 换真 Intake、face 换 `customerClientFace(CLAIM_SUBMISSION)`、路径改 `/v1/claims`；`ClaimIntake` 头注改成现行唯一性规则；索赔端点的错误体改问题详情并按恢复动作分格（401/403/403 准入/503/404/400）；`ReceiveClaim` 加他账户撞键判断；合同责任范围声明加 `customer_ref`（迁移 `visibility_exception/0030`），登记命令、批文（`customer` 字段）、读口、种子 `04-claim-eligibility-contract-01.json` 随改；新读口 `ports.ClaimContractOwnership`；契约文档加「索赔口」一章；UC-VE-007 加 AT-VE-208 至 212，盘点①五行加追记。

**本口形状 VCL-1**（`customer_claim_shape.go` 的 `CustomerClaimShapeFields`）：`accessRequestId`、`batch`、`item`、`applicant`、`contract`、`target`、`kind`，封闭，其外一律 400。无内容摘要、无内容冲突格；提交时间取到达时刻（截到微秒，首答与重放读回逐字节同）。

**代决与越权风险点**
1. 合同归属走加列（协调方裁定 (a)）：`claim_contract_scope` 加 `customer_ref NOT NULL`，主键仍（租户、合同范围），一份合同只属一个账户；读口 `RulesForClaim` 与新 `ContractBelongsToCustomer` 按账户过滤，他账户答声明不在场，Intake 译 404。存量行推不出归属，迁移里有存量行就 `RAISE EXCEPTION` 拒绝施加并列出（租户/合同范围），由运营按账户带 `customer` 重登后删原行再重跑；演示种子不含旧行，已在一次性库验过空库施加与拒绝路径。**迁移号 0030 为暂取**，已核本地与各 `claude-` 远端分支 VE 最大号 0029；若与别的会话撞，改文件名即可，内容无号依赖。风险：运营批文现必须带 `customer`，旧批文被拒。
2. 自然键不改主键（协调方裁定）：存储键仍（租户、批次、项）；`ReceiveClaim` 已有项的 `Customer` 与命令不同答 `NOT_ACCEPTED`、不带原项、不覆盖。**残留风险：他账户可探测某（批次、项）已被占用，1 比特**；彻底解决（主键含账户，波及资格审核、结论、证据、结算交接按（批次、项）取项的全部路径）另票，本轮不立。
3. 同账户同（批次、项）再来：申请人、合同、目标、类型都相同当重放，答原项 `CLAIM_EXISTING_RESULT`；任一不同答 `NOT_ACCEPTED` 并带 `notAcceptedReason=CLAIM_CONTENT_DIFFERS_FROM_ACCEPTED`，不返回原项、不覆盖（评审后改，与补充口同口径）。比对在 `ReceiveClaim` 里对已有项字段做，不加摘要列、无迁移；提交时间不比。风险：存量项无申请人时申请人一格不比；换内容的修正要用新的（批次、项）。
4. 申请人不在 Intake 拦（协调方裁定）：授权名单本按（租户、账户）分，名单外的申请人由资格审核形成不予受理，不是 404；他账户的申请人引用因此不会被泄露存在，因为 Intake 不答任何关于申请人的东西。
5. 目标包裹归属经 `ParcelCustomerAccountView`（追踪视图查阅口同一只，PS 当前已接受委托归属）：未被任何已接受委托声明的包裹、多份委托同时声明的包裹都答 404。风险：委托尚未接受时的包裹提不了索赔（与「查不到视图」同口径）。目标引用按 `DeclaredParcelID` 译 `TrackedParcelReference`，与 `ClaimServiceRules` 取回指时同一译法。
6. 404 只有一个码 `CLAIM_REFERENCE_NOT_FOUND`，目标与合同不分格；依赖读不动答 5xx，不折成 404。
7. 索赔端点原 `{"error":{"code"}}` 体改问题详情（`first_party_contract_test` 要求首方族各口如此）；这是对该端点响应形状的破坏性改动，但它此前只答 403 未配置，无存量调用方。
8. 来源请求键（`claim:` 前缀）只经信封校验接入请求标识必填，不进 `ReceiveClaimCommand`：传输重放靠自然键，同接入请求标识的重投与同自然键重投走同一条路。由此「同一接入请求重放答同一结果」成立，但换了接入请求标识而自然键相同同样答原项。
9. 未改的：运营目录列表读口（`ListClaimEligibilities` 与 `GET /visibility-catalogues?kind=CLAIM_ELIGIBILITY`）没带出 `customer`，管理台看不到声明归属，另票。

**判据对用例**：首方凭证经 `/v1/claims` 成功、重放答同一项、他账户目标与合同同字节 404、载荷自报账户 400、撤销授予后 403 未授予——`cmd/parcel-api/assemble_customer_claim_test.go::TestACustomerClientSubmitsAClaimThroughTheFirstPartyLineAgainstARealDatabase`（生产 build 与 mount，真库）；未授予 / 别族令牌 / 不在准入范围 / 无令牌——`TestClaimLineRefusesOtherFamiliesAndJudgesAdmission`；首方族未配置字节同——`TestClaimLineAnswersNotConfiguredByteForByteWhenTheCustomerFamilyIsUnset`；他账户撞键——`TestAClaimKeyHeldByAnotherAccountIsNotAcceptedAndDoesNotLeakTheOriginal`；合同归属——`TestAContractScopeBelongsToOneAccountOnly`。AT-VE-208 至 212 在盘点①。

**自验**（含 PG，DSN `127.0.0.1:55432`）：`go build ./...`；`go vet` VE、accessidentity、cmd；`gofmt`；`go test -count=1 -p 1` 跑 `./internal/visibilityexception/...`、`./internal/accessidentity/...`、`./cmd/parcel-api/...`、`./cmd/parcel-ve-register/...`、`./internal/architecture/...`、`./migrations/...`、`./tools/...` 全绿；一次性库 `seed.sh` 退 0 且声明行带 `SYN-ACCOUNT-01`，库已删；`at-coverage-counts -check` 只报生成物过期。没跑全量。演示实走（起 parcel-api 与 Dex）没做。
