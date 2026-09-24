# 03 `OperatorEnvelope` 与铸造、三格答复代数

Category: enhancement
Status: resolved——2026-09-25 已进 main（完成记录见文末 Comments）。此前 in-progress——2026-09-25 通道 4 认领（用户令独立完成 ADR-0151 那件，15 号票要先过本票；前置 01、02 均已 resolved），隔离 worktree `idp-parcel-mcp4-oc03`、分支 `mcp4-oc03`（基 `2df79907`）。此前 ready-for-agent——2026-09-24 拆法经用户授权通道 4 自决认可
Blocked by: 01、02
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 甲轨
地盘：`internal/accessidentity`（信封类型与铸造）、`internal/platform/httpapi` 若需新答复格；`internal/accessidentity/doc.go`；ADR-0072 / 0085 的前向指针核对。
出处：[ADR-0100](../../../docs/adr/0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定三、决定四答复代数与 Consequences。

## 做什么

1. `OperatorEnvelope`：租户、操作者主体、授予集、来源固定为管理台；字段不导出、包外无构造函数，拿到即经过铸造；不复用 `SourceEnvelope`，编译期拿它铸不出客户委托。
2. 铸造：校验令牌（02）→ 查操作者册（01）→ 按请求的租户与能力面核授予 → 铸信封。
3. 答复代数三格不合并（ADR-0029）：发行方未配置 → `403 ACCESS_CHANNEL_NOT_CONFIGURED`；令牌缺失、过期或校验不过 → `401`；令牌有效但不在册、不绑该租户或无此能力面授予 → `403` 新格，不披露哪一半不对（ADR-0055 决定四）。
4. `doc.go` 那段「本轮既没有登记册的表，也没有凭据形态」改为「客户渠道那一半仍等 `PAR-INT-01`；操作者那一半已按 ADR-0100 立」；ADR-0072 与 ADR-0085 的部分停用前向指针，缺则补。

## 完成判据

- 三格各有用例且互不顶替；册读失败答依赖故障而不是未授予；零值信封不可用。

## Comments

### 完成记录 ← 通道 4 · 2026-09-25（作者即推送方）

**落点**：`3ba554f5` 代码（`internal/accessidentity/operator_envelope.go` 与测试、`doc.go` 一段）；`294b3151` 机制清点重生成（accessidentity 生产 10→11、测试 6→7）；本笔票面。

**完成判据逐条**（`internal/accessidentity/operator_envelope_test.go`）：
- 三格各有用例、互不顶替：`TestOperatorAnswerGradesDoNotStandInForEachOther` 九种情形，断言每种情形只被自己那一格的哨兵认出，而且都不产出铸成的信封。三格是：未配置 `ErrAccessChannelNotConfigured`；令牌不过 `ErrCredentialRejected`；不在册、绑在别的租户、请求没指名租户、无此能力面授予、授予已撤，这五种同答 `ErrOperatorNotGranted`。
- 册读失败答依赖故障：同一测试的「register cannot be read」答 `ErrOperatorRegistryUnavailable`，不答未授予。取不回发行方公钥集的 `ErrCredentialVerifierUnavailable` 同样原样交回、不折进令牌不过。
- 零值信封不可用：`TestZeroOperatorEnvelopeIsUnusable`（Minted 为假、租户为空、不持有任何授予）。
- 做什么第 4 条：`doc.go` 那段已改写。ADR-0072 与 ADR-0085 的 Status 行本就带 ADR-0100 的部分停用指针，不补。

**判断项**：
1. 答复码不在本票：本仓各上下文的 `ACCESS_CHANNEL_NOT_CONFIGURED` 都写在自己的 `adapters/http`，`internal/platform/httpapi` 里没有共用的答复格；401 与 403 新格随 04、05、15 各口换操作者 Intake 时映射，映射照本票的哨兵。
2. `OperatorRequest.TenantID` 必须指名，且只用来比对；信封的租户取自册上的绑定。一个操作者只绑一个租户，所以请求那一侧只可能答对或答未授予，不存在从请求里取身份的路径。
3. 信封带的是铸造那一刻所有生效授予的能力面，不只所请求的那一格：消费方要再问别的能力面时不必重铸。授予的生效与否按铸造时刻判，不存成状态。

**验证**：`gofmt -l` 空；全仓 `go build`、`go vet` 过；带 DSN 的受影响范围（`internal/accessidentity/...`、`cmd/parcel-access-register`、`cmd/parcel-api`、`internal/architecture/...`）全 ok；`accessidentity` 包 `-race` 过。进 main 前在最终 SHA 上另跑带 DSN 全量（见提交信）。

**评审**：推送方即作者，只有自审，**不算非作者评审**。
