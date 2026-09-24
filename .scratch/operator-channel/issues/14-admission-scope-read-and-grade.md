# 14 准入范围读口与「不在准入范围」一格：ADR-0055 决定五第二项

Category: enhancement
Status: resolved——2026-09-25 通道 4 做完并进 main（用户令独立完成 ADR-0151 那件，15 号票要先过本票；完成记录见文末 Comments）。此前 ready-for-agent——2026-09-24 随 ADR-0149 立（用户授权通道 4 自决）
Blocked by: 无（读口与答复格）；接进各族铸造随 10、11
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 丙轨实施
地盘：pilot-governance 生产权威区间的只读口（按租户、对象范围、能力、事实类型问当前区间）、`internal/accessidentity` 铸造前的准入判断与新答复格。
出处：[ADR-0149](../../../docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定四第二条；PN-08 的生产权威区间记录能力。

## 做什么

1. pilot-governance 出只读口：给定（租户、对象范围、能力、事实类型）与时点，答当前生产权威区间覆盖与否；读失败走依赖故障，不折成「不覆盖」。
2. 铸造前判：区间不覆盖答新格「不在准入范围」（`403`，恢复动作：阶段治理登记区间），不进业务编排。隔离形态（ADR-0091）不走这一判。

## 完成判据

- 覆盖、不覆盖、读失败三格各有用例；生产形态下无区间时一律答「不在准入范围」；隔离形态行为不变。

## Comments

### 完成记录 ← 通道 4 · 2026-09-25（作者即推送方）

**落点**：`f6a01e9f` 代码（`internal/pilotgovernance/application/read_authority_coverage.go`，`internal/accessidentity/admission_scope.go`，`operator_envelope.go` 铸造前的判断，以及两处测试）；`181cd1d1` 机制清点重生成；本笔票面。

**完成判据逐条**：
- 覆盖、不覆盖、读失败三格各有用例：`TestAuthorityCoverageAnswersWhetherThisProductHoldsTheInterval`（八种情形）、`TestAuthorityCoverageReadFailureIsADependencyFailureNotNotCovered`；铸造侧 `TestAdmissionScopeIsJudgedOnlyForFacesThatRequireItAndAfterTheGrant`（覆盖 → 铸成；不覆盖 → `ErrOutsideAdmissionScope`；读不动 → `ErrAdmissionScopeUnavailable`，且不被「不在准入范围」认出）。
- 生产形态下无区间时一律答「不在准入范围」：登记册无区间、代表本产品的权威串没配，`Covers` 都答不覆盖；铸造侧 `TestUnconfiguredAdmissionScopeAdmitsNothing`。
- 隔离形态行为不变：隔离形态不经操作者铸造，本笔没碰 `cmd/parcel-api` 的装配；`cmd/parcel-api` 带 DSN 全绿。

**判断项**：
1. **租户维**：ADR-0149 决定四写「按（租户、对象范围、能力、事实类型）读」，而治理登记册本身没有租户维（ADR-0083 决定三）。本票的取法是：只读口按登记册现有的三维问；租户到对象范围的对照属租户随试点登记的实例半边，由 `AdmissionScope` 的实现去换，没登就答不在准入范围——与 parcel-shipment 生产归属适配器的 `GovernanceScopeDirectory` 同形。
2. **「本产品」是谁**：区间的 `Authority` 须等于登记册里代表本产品的权威串才算覆盖；没配就不认领任何区间。约定与 parcel-shipment 生产归属适配器的 `SelfAuthority` 相同，两处将来装配时应取同一个值。
3. **判的位置**：判在核授予之后，没有授予的人看不出这个租户登没登区间；登记册配置写面不带准入要求（ADR-0100），运营决定口要带（ADR-0151 决定三）。
4. **不装配**：接进各族铸造随 10、11、15。届时各口的 `AdmissionRequirement`（能力与事实类型）逐口定，桥接「租户对照 + `AuthorityCoverage`」的那只适配器随第一个消费者一起立。

**验证**：`go build ./...`、`go vet ./...` 全仓过；带 DSN 的受影响范围（`internal/accessidentity/...`、`internal/pilotgovernance/application`、`cmd/parcel-access-register`、`cmd/parcel-api`、`cmd/parcel-governance-register`）加 `internal/architecture/...` 全 ok；两包 `-race` 过。进 main 前在最终 SHA 上另跑带 DSN 全量。

**评审**：推送方即作者，只有自审，**不算非作者评审**。
