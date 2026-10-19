# 09 送站预报的提交与撤回口：挂首方客户渠道族，一口一词，NO 定载荷形状

Category: enhancement
Status: in-progress——2026-10-19 会话 `idp-parcel-9b` 认领，分支 `claude-psxm09`，access_identity 迁移号 0014；与票 14 并行。此前 ready-for-agent——2026-10-19 票 07 的作者子代理照 ADR-0212 拆出。代决：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」，2026-10-19 答「需要」
Blocked by:
- [08](./08-pre-advice-and-handover-note-model-and-window-register.md) 进 main：预报册与领域。
- 首方客户渠道 Intake 已由 [first-party-api/00](../../first-party-api/issues/00-first-party-customer-intake.md) 进 main（`38916b00`），照它落地的形状挂，不设阻塞边。能力粒度与 1:N 绑定在 [first-party-api/01](../../first-party-api/issues/01-customer-api-scopes-and-account-binding-cardinality.md)，两票同改客户面能力的枚举与 CHECK，谁后进谁 rebase。
父票：[07](./07-verifiable-delivery-scope-for-customer-numbers.md)
出处：[ADR-0212](../../../docs/adr/0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md) 决定二、九；[ADR-0139](../../../docs/adr/0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) 决定四、五、八；[UC-NO-006](../../../docs/application/node-operations/UC-NO-006-ESTABLISH-A-VERIFIABLE-DELIVERY-SCOPE.md) 应用流程步骤 1、2 与 `AT-NO-069`、`AT-NO-070`。
地盘：
- `internal/accessidentity`：客户面能力 `CustomerCapability` 加两个词；`CustomerClientMinter` 加两口的铸造方法与首方族推导口；按命令分型的信封；各自测试
- `migrations/access_identity/`：客户面能力那条 CHECK 加两个词，取当时下一个空号（另一会话正在连续取号，不写死号）
- `internal/nodeoperations/adapters/accessidentity`（首方认证适配，新文件）、`adapters/http`（两口与规范化形状）、`application`（提交与撤回编排）及测试
- `cmd/parcel-api`：端点表两行、装配、真库用例新文件
- `scripts/demo-seeds`：合成货主系统 `syn-customer-client-01` 的两格授予、准入范围对照两行、一份交预报的脚本（照 `submit-one-shipment.sh` 先取令牌）；README 与 `docs/design/synthetic-demo-journey-script.md` 相应一段
- `docs/product/MECHANISM-INVENTORY.md`（重生成）、`.scratch/at-coverage-inventory.md`

## 做什么

1. **能力词**。照 first-party-api/00 代决「一口一词」加两个：送站预报提交、送站预报撤回。字面量本票定，建议 `DELIVERY_PRE_ADVICE_SUBMISSION`、`DELIVERY_PRE_ADVICE_WITHDRAWAL`。枚举与库上 CHECK 同笔改；词写进完成记录，供票 01 归并时对照。
2. **载荷形状**。NO 定一版带形状版本的规范化形状（ADR-0139 决定八），摘要进信封。
   - 提交载荷：节点、预计送达时刻、客户包裹件号集合、可选的交付方线索、接入请求标识。
   - 撤回载荷：预报标识、接入请求标识。
   - 接入请求标识是信封元数据，不进摘要（ADR-0139 决定五）。
   - 封闭解码：自报的租户、客户账户、来源都落在未知字段，答坏载荷。
3. **铸造**。照 `MintSubmission` 加两口，校验次序同 `admit`：令牌、册、证书绑定、授予、准入范围。
   - 这两口是生产写口，准入范围在铸造侧判（ADR-0139 决定八）；能力与事实类型照 ADR-0180 决定四的词登进准入范围对照。
   - 来源请求键由首方族推导口从接入请求标识取，口径同提交口（去两端空白、此外一字不动）。
   - 撤回的推导由本票为预报撤回定，与委托撤回无关（first-party-api/00 越权风险点里委托撤回那一条不在本票）。
4. **编排**。
   - 提交：按来源请求键认重放（同内容答已有结果，异内容答来源冲突）；首次签发预报标识，落 08 的册，答回标识。
   - 撤回：只撤信封里那个客户账户的预报；指名别的客户账户的预报，与查无此预报同答。
5. **答复分格**。信封格照 ADR-0139 决定四（`ACCESS_CHANNEL_NOT_CONFIGURED`、`CUSTOMER_CLIENT_CREDENTIAL_REJECTED`、`CUSTOMER_CLIENT_NOT_GRANTED`、`OUTSIDE_ADMISSION_SCOPE`、`IDENTITY_DEPENDENCY_UNAVAILABLE`）；业务格照 UC-NO-006 结果语义契约，线上名本票定、写进完成记录。
6. **演示**。种子给 `syn-customer-client-01` 两格授予，登准入范围对照；脚本取令牌后交一份预报，打印预报标识，供票 11 的演示动线接着用。证据记 `S`。

## 不做

- 收寄带预报标识、按预报判范围（票 11）。
- 客户面读口（查自己的预报列表）：另立，本票不开。
- 1:N 绑定与能力归并（first-party-api/01）。
- 契约文档生成与 `/v1` 前缀（ADR-0140 的实施票）。

## 完成判据

- [ ] HTTP 与铸造用例（`AT-NO-069`）：令牌无效 401、不在册或没有这一格授予 403、不在准入范围 403、依赖读不动 503、发行方未配置 403；别族令牌（操作者、集成客户端）一律拒；坏载荷各种答 400，含请求自带租户或客户账户、号码为空或重复。
- [ ] 编排用例（`AT-NO-069`、`AT-NO-070` 的撤回半）：首次答已登记并签发标识；同一接入请求原样重投答已有结果，换内容答来源冲突；撤自己的预报落撤回，撤别的客户账户的与查无此预报同答。
- [ ] 真库用例（`cmd/parcel-api`，经生产装配）：合成客户端取令牌、交预报、撤回，册上各一行；撤销授予之后再交答 `CUSTOMER_CLIENT_NOT_GRANTED`。
- [ ] 演示实走（`S`）：`seed.sh` 退 0，脚本交出一份预报并打印标识。
- [ ] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/accessidentity/...`、`internal/nodeoperations/...`、`cmd/parcel-api`、`cmd/parcel-access-register`、`migrations/...`、`scripts/demo-seeds/...`、`internal/architecture/...` 全绿；机制清点重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。

## 规模

M
