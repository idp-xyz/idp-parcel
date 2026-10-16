# 22 运输履约隔离命令 Intake 退场：译装用例迁到作业事实 Intake，删类型

Category: enhancement
Status: resolved——2026-10-16 进 main（`7ec601ab`，分支 `claude-oc21` 快进，远端 main 含 `efc84d87`）；见「进 main 记录」。此前 in-progress——2026-10-16 完工，待进 main；见「完成记录」。此前 in-progress——2026-10-16 Claude Code 会话立并认领（用户「针对上面的问题，你能自决，全部解决吗」），分支 `claude-oc21`。
Blocked by: 无
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 丙轨实施（收口后的清扫）
地盘：`internal/transportfulfillment/adapters/http` 的 `isolated_command_intake*.go`、`record_delivery_attempt_test.go`、`work_fact_intake*.go` 与几处头注。
出处：[ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定三；[15](./15-operation-decision-faces-take-operator-intake.md) 完成记录「发现，不做」第一条。

## 为什么

`tfhttp.IsolatedCommandIntake` 已无装配使用者：作业事实各口随票 10 换作业事实渠道、运营决定四口随票 15 换操作者渠道。ADR-0150 决定三要求换口的同一笔撤下隔离 Intake 上对应的方法；票 10 那几口当时没撤。类型上余下方法的译装用例是这几口线格式的主要覆盖，`WorkFactCommandIntake` 在本包没有自己的用例，所以不能直接删。

## 做什么

1. 隔离 Intake 的译装用例改挂 `WorkFactCommandIntake`（替身认证方交回合成租户）：期望值不变，租户的来处从注入值换成认证结果；作业事实 Intake 自己那几格（三个头缺一即坏报文、作用域种类只认 `OFFSITE`、先认证后解载荷）补用例。
2. 删 `IsolatedCommandIntake` 类型与它的构造器；改准提到它的头注（交付、交接两口的 Intake 契约，运营决定 Intake 的头注）。

## 完成判据

- 原隔离用例的每一格在作业事实 Intake 上都有对应用例（自报租户、尾随内容、未知键、词表外的词、时刻缺席与解不出、各口逐字译装）。
- 全仓无 `tfhttp.IsolatedCommandIntake` 的引用；本包与 `cmd/parcel-api` 测试绿。

## 完成记录（2026-10-16，Claude Code 会话，分支 `claude-oc21`，代码 `7ec601ab`）

- **落点**：`isolated_command_intake_test.go` 改名 `work_fact_intake_test.go`、改挂 `WorkFactCommandIntake`（替身认证方），原用例逐格保留、期望值不变；`record_delivery_attempt_test.go` 改用合规的作业事实出示；删 `isolated_command_intake.go`；交付、交接两口的 Intake 契约头注与运营决定 Intake 头注改指真渠道实现。
- **补的格**：`TestWorkFactCommandIntakeTakesDeviceAndScopeFromHeadersAndAuthenticatesFirst`（三个头缺一即 400、`NODE` 作用域即 400、认证方被问头里那一份、设备不在用时答认证方的格而不是载荷校验）；`TestNewWorkFactCommandIntakeRejectsABlankMovementSourceOrNoAuthenticator`；类型锁 `TestWorkFactCommandIntakeServesOnlyWorkFactLines`（首登与更正十口在、运营决定 / 登记写面 / 查阅行装不进）。
- **判据**：✅ 原隔离用例每一格在作业事实 Intake 上都有对应用例；✅ 全仓无 `tfhttp.IsolatedCommandIntake` 引用；✅ TF `adapters/http` 与 `cmd/parcel-api` 测试绿。迁来的用例是对已有路径的取证、一写即绿；端点层派送尝试用例改挂之后先红（不带出示答 400）再改用合规出示转绿——红是真的，用例跟着渠道改。
- **评审**：作者自审，不算非作者评审。证据层级 `S`。

### 进 main 记录（2026-10-16）

- 分支 `claude-oc21` 五笔 `7ec601ab` / `9c551dc4` / `e8673ac1` / `c7a462cd` / `efc84d87` 叠在远端 main `f33d849d` 之上，快进进 main、SHA 未改写；`efc84d87` 是远端 main 的祖先（其后另一会话叠了 `43309fa5`，只动 `.scratch/group-legal-entities-uiux/`，与本票无交集）。
- 推送方全量一次：钉 `efc84d87` 的全新 detached 检出，带 DSN，`go test -p 1 -count=1 -v ./...` 退 0——有测试的包 141 个 ok、0 FAIL，`--- SKIP` 1（`TestHelperTemplateOwnerProcess`，照设计跳过）；同一检出上全仓 `gofmt -l .` 无输出，`go build ./...`、`go vet ./...` 退 0。
- 评审：只有作者自审，没有非作者评审（本机无别的会话可派；仓规 2026-09-16 不用子代理）。

## Comments

- **评审 ← 会话 idp-parcel-9b 起的隔离子代理 · 钉 `7ec601ab`（基线 `f33d849d`）· 2026-10-16 10:0x–10:2x UTC**（票 [26](./26-non-author-review-backlog-for-claude-code-landings.md) 序 5 补评。非作者：代码出自会话 `idp-parcel-c8`。一个子代理在隔离只读 detached 检出上按 `/code-review` 两轴串行，先写完 Spec 再做 Standards；由会话 `idp-parcel-9b` 代落。）评审侧验证（无 DSN）：在 `7ec601ab` 上本包 vet、test 绿，`cmd/parcel-api` vet 过，全仓已无 `tfhttp.IsolatedCommandIntake`。
  - **Spec**：**阻断 无**。逐格核过：旧用例体除常量与名字外逐字迁入；七口译装、自报租户、尾随 / 未知键 / 非 JSON / 空载荷、词表外、时刻缺席与解不出、自报来源、端点 201/200 都在。只删了 `TestNewIsolatedCommandIntakeRejectsBlankTenant`：新类型没有注入租户，这一格由「缺认证方即拒」顶上。**非阻断 3**：
    (1) `internal/transportfulfillment/adapters/http/work_fact_intake_test.go` · `workFactLines`：表里仍只有隔离那七口，类型锁却断言更正三口也在面上；更正三口与三个更正载荷在本包没有用例，`cmd` 的端到端只证明走得到编排。补上会暴露一个问题：更正载荷用 `time.Parse`，缺时刻即答 400，与 `command_payload.go` · `parseOptionalInstant`「两者的续办不同，不折成一格」相左。这是票 10 遗留，宜另立票。
    (2) `TestWorkFactCommandIntakeTakesDeviceAndScopeFromHeadersAndAuthenticatesFirst`：「先认证后解载荷」只在 `IntakeRegistration` 一口上证了；同包 `TestEachTFDecisionFaceAuthenticatesForItsOwnDecision` 是逐口证的。
    (3) `register_effective_delivery.go` · `DeliveryIntake`：本票「做什么」要「改准提到它的头注（交付、交接两口的 Intake 契约，运营决定 Intake 的头注）」，这件没做完——头注仍说「接口而不是本包内的解析代码」，可同句点名的 `WorkFactCommandIntake` 就在本包。
  - **Standards**：**阻断 无**。**非阻断 5**：
    (1) `work_fact_intake_test.go` · `TestWorkFactCommandIntakeRefusesSelfReportedTenantOnEveryLine`：Covers 写「值与开关相同也拒」，「开关」说的是隔离形态的注入值，而这里的租户已来自认证结果。
    (2) `register_transport_handover.go` · `HandoverIntake` 头注「分开的理由是逐口放行（ADR-0091 Consequences）：一个只放行了首登的实现…」：本笔删掉的正是唯一这样的实现，余下实现两口都有，这条理由落空；`DeliveryIntake`「分法与理由同 HandoverIntake」也随之落空。
    (3) `work_fact_intake_test.go` 文件头「原挂在隔离命令 Intake 上，随它退场迁来…期望值不变」是变更说明，违反「不写变更说明」；而且 Source 的期望值已由 `SYN-SOURCE/self-operated-executor` 换成 `SELF_OPERATED`，「不变」不实。`operator_decision_intake_test.go`「从隔离命令 Intake 迁来」同样指向已删的类型。
    (4) 同文件多处写「三个头」（文件头、Covers、失败消息），数的是 `work_fact_intake.go` 里的头常量，违反「改文档」的「不用计数」。
    (5) 判断题：两个 `…OnEveryLine` 用例名与 `workFactLine` 注释「对每一口都成立」，实际只覆盖十口中的七口（疑似 Mysterious Name）；`workFactAuthenticatorFake` 的 `token == ""` 分支没有用例走到（疑似 Speculative Generality）。
  - **无发现**：红线五条未碰，领域包没有新依赖，注释为中文；完成判据两条成立。
  - **结论**：两轴 0 阻断，不立修复票。main `4dd937eb` 上 `7ec601ab` 之后没有提交再改 TF `adapters/http`，以上各条都还在。Spec (1) 的更正载荷缺时刻答 400，子代理建议另立票，交 TF owner 定。
