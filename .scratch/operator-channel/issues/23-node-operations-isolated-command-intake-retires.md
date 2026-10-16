# 23 节点作业隔离命令 Intake 退场：共享译装挪出，用例迁到作业事实收寄 Intake，删类型与门里的构造

Category: enhancement
Status: resolved——2026-10-16 进 main（`9c551dc4`，分支 `claude-oc21` 快进，远端 main 含 `efc84d87`）；见「进 main 记录」。此前 in-progress——2026-10-16 完工，待进 main；见「完成记录」。此前 in-progress——2026-10-16 Claude Code 会话立并认领（用户「针对上面的问题，你能自决，全部解决吗」），分支 `claude-oc21`。
Blocked by: 无
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 丙轨实施（收口后的清扫）
地盘：`internal/nodeoperations/adapters/http` 的 `isolated_command_intake*.go`、`work_fact_intake*.go`、`receive_delivered_unit.go` 头注；`cmd/parcel-api/assemble_isolated_write.go` 与 `isolated_write_test.go` 里节点作业那一格。
出处：[ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定三；[15](./15-operation-decision-faces-take-operator-intake.md) 完成记录「发现，不做」第二条。

## 为什么

收寄口随票 10 换作业事实渠道之后，`nodeopshttp.IsolatedCommandIntake` 只剩隔离写门里的一次构造，装配不读它（`isolatedWriteAdmission` 头注自承）。收寄载荷的译装（`receptionDocument`、`receptionCommand`、`applyReceptionClaim`、`decodeClosedDocument`）住在隔离 Intake 的文件里、被作业事实收寄 Intake 共用，用例也只挂在隔离类型上。

## 做什么

1. 收寄载荷的译装挪到独立文件，行为不变。
2. 隔离 Intake 的用例改挂 `WorkFactReceptionIntake`（替身认证方交回合成租户与节点）；补作业事实那几格（三个头缺一即坏报文、作用域种类只认 `NODE`）。
3. 删 `IsolatedCommandIntake` 类型；隔离写门里节点作业那一格（字段、访问器、构造、合成节点常量）与「随门构造」用例一并删去。

## 完成判据

- 原隔离用例的每一格在作业事实收寄 Intake 上都有对应用例。
- 全仓无 `nodeopshttp.IsolatedCommandIntake` 的引用；本包与 `cmd/parcel-api` 测试绿。

## 实施中量到的（不在本票做）

收寄命令的 `ServiceMarkers`（PS 服务结果引用：已取消 / 终局 / 无路由 / 受限）按 `ReceiveDeliveredUnitCommand` 的契约「由接入层查好带入」，它不阻止接收、只限制后续方向性作业（UC-NO-002 的 `AT-NO-020`–`022`）。票 08 当时记过「隔离形态没有那道查询，留空」；票 10 换上作业事实渠道之后，`WorkFactReceptionIntake` 同样不查——生产上已取消或已终局的包裹到站收寄时不带标记。立为 [25](./25-node-reception-carries-parcel-shipment-service-result-markers.md)。

## 完成记录（2026-10-16，Claude Code 会话，分支 `claude-oc21`，代码 `9c551dc4`）

- **落点**：收寄载荷译装原样挪到 `reception_payload.go`；删 `isolated_command_intake.go`；`isolated_command_intake_test.go` 改名 `work_fact_intake_test.go`、改挂 `WorkFactReceptionIntake`（替身认证方交回合成租户与节点），原用例逐格保留；`receive_delivered_unit.go` 的 Intake 契约头注改指真渠道实现；`cmd/parcel-api` 隔离写门删节点作业那一格（字段、访问器、构造、合成节点常量）与「随门构造」用例。
- **补的格**：`TestWorkFactReceptionIntakeTakesDeviceAndScopeFromHeadersAndAuthenticatesFirst`（三个头缺一即 400、`OFFSITE` 作用域即 400、认证方被问头里那一份、先认证后解载荷）；`TestNewWorkFactReceptionIntakeRejectsAMissingAuthenticator`；类型锁 `TestWorkFactReceptionIntakeServesOnlyTheReceptionLine`。
- **判据**：✅ 原隔离用例每一格都有对应用例；✅ 全仓无 `IsolatedCommandIntake` 引用（两上下文都已退场，隔离写门只剩提交口那一行）；✅ NO `adapters/http` 与 `cmd/parcel-api` 测试绿。
- **评审**：作者自审，不算非作者评审。证据层级 `S`。实施中量到的缺口见上节，立为 25。

### 进 main 记录（2026-10-16）

- 分支 `claude-oc21` 五笔 `7ec601ab` / `9c551dc4` / `e8673ac1` / `c7a462cd` / `efc84d87` 叠在远端 main `f33d849d` 之上，快进进 main、SHA 未改写；`efc84d87` 是远端 main 的祖先（其后另一会话叠了 `43309fa5`，只动 `.scratch/group-legal-entities-uiux/`，与本票无交集）。
- 推送方全量一次：钉 `efc84d87` 的全新 detached 检出，带 DSN，`go test -p 1 -count=1 -v ./...` 退 0——有测试的包 141 个 ok、0 FAIL，`--- SKIP` 1（`TestHelperTemplateOwnerProcess`，照设计跳过）；同一检出上全仓 `gofmt -l .` 无输出，`go build ./...`、`go vet ./...` 退 0。
- 评审：只有作者自审，没有非作者评审（本机无别的会话可派；仓规 2026-09-16 不用子代理）。

## Comments

- **评审 ← 会话 idp-parcel-9b 起的隔离子代理 · 钉 `9c551dc4`（基线 `7ec601ab`）· 2026-10-16 10:0x–10:2x UTC**（票 [26](./26-non-author-review-backlog-for-claude-code-landings.md) 序 6 补评。非作者：代码出自会话 `idp-parcel-c8`。一个子代理在隔离只读 detached 检出上按 `/code-review` 两轴串行，先写完 Spec 再做 Standards；由会话 `idp-parcel-9b` 代落。）评审侧验证（无 DSN）：本包与 `cmd/parcel-api` 测试绿。
  - **Spec**：**阻断 无**。**非阻断 1**：`internal/nodeoperations/adapters/http/work_fact_intake_test.go` · `receptionAuthenticatorFake`：替身认证方交回的节点，和 `receptionRequest` 里 `Parcel-Work-Scope-Reference` 头的值相同，都是 `SYN-NODE-SHA-HUB`。所以即使 `IntakeReception` 改成直接拿头值铸节点，用例照样全绿，测不出文件头那句「租户与节点两格只来自认证结果」；完成判据「原隔离用例的每一格…都有对应用例」里节点那一格因此辨别力不足。今天没有实害，因为真认证方交回的是核过设备登记范围之后的值。让替身交回一个和头值不同的节点即可补上。
  - **Standards**：**阻断 无**。**非阻断 4**：
    (1) `reception_payload.go` · `decodeClosedDocument`：注释写「两道判据同 partycommercial 的隔离身份 Intake」，那个类型在 `4735f9dd` 已删，是死引用。按「改文档」应改用符号名，即 `party_identity_payload.go` 的 `decodeClosedDocument`。评审会话复核属实。
    (2) 本笔新写的几处注释是变更说明，违反「写代码注释」的「不写变更说明」：`reception_payload.go` 文件头「它原住在隔离命令 Intake 的文件里…行为一字未改」、`isolatedWriteAdmission` 里「…节点作业那一格随票 23」、测试文件头「原挂在隔离命令 Intake 上…期望值不变」。其中「票 23」没带命名空间，`.scratch` 下有多张 23 号票。
    (3) `cmd/parcel-api` 里与隔离形态相关的残项只剩命名和注释，早于本笔、不在本票地盘：`isolated_write_reception_test.go` 及其用例名，自 `ec00b795` 起测的已是作业事实渠道（TF 侧同类文件已在 `60244fa6` 改名）；`endpoints.go`「命令端点在上，走写开关的逐口变量（票 operator-channel/08）」已不属实。
    (4) 删 `IntakeReception` 时，把「留空的后果照实说」那句也删了，测试失败消息还改写成把空的 `ServiceMarkers` 当作正确。已由 `0214a62a`（票 25）修掉。
  - **无发现**：两条完成判据都成立——原用例逐格有对应、坏报文表逐字一致、全仓无引用、本包与 `cmd/parcel-api` 测试绿；译装挪到新文件后与原文逐字节一致；门里节点作业那一格（字段、访问器、构造、合成节点常量）删干净了，两份放行名单只剩 `/shipment-requests`；做法与 TF 票 22 一致。
  - **结论**：两轴 0 阻断，不立修复票。Spec 那条与 Standards (1)–(3) 在 main `4dd937eb` 上仍在。
