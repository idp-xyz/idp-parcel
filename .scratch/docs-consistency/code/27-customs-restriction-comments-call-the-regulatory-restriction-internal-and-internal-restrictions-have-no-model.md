# 27 CC 准入代码把监管限制叫作内部合规限制；内部合规限制没有模型

Category: enhancement
Status: in-progress——2026-10-27 会话 tops-20（idp-parcel-91）接手 tops-26 断在中途的「上线前必须」一组（冻结清单「六」），分支 claude-code27（续 tops-26 已有分支与 worktree 半成品）。此前 ready-for-agent——R3（不急）：限制册在生产上没有写入（`ManageRestrictionHandler` 未装配，ADR-0206 越权风险点 1），内部合规限制没有任何入口，今天没有生产路径；**UC-CC-011 接线之前必须先做第 2 项**，否则内部合规限制要么被塞进 `RegulatoryRestriction`（让监管限制可以不凭监管结果解除），要么挡不住动作。R3 只说缺陷本身今天不出事；内部合规限制的模型是首发范围要求的实现缺口（PILOT-SCOPE 要求验证内部合规限制、关务专项基线切片 1 含它），不因 R3 而移出首发。2026-10-21 文档自洽 03 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc03。
Blocked by: 无（裁向已由 [ADR-0241](../../../docs/adr/0241-action-admission-restrictions-are-regulatory-or-internal-and-only-regulatory-restrictions-exist-today.md) 定）
出处：[audit/cc.md](../audit/cc.md) H3（B，审计钉 `6b0074d0`；2026-10-21 按 main `205eb254` 复核，现象仍在）；[02 的完成记录](../issues/02-code-defects-from-the-audit.md)「其余条件性 B 类」一句把它交给修订票 03 裁后按裁向立票。

## 现象

CC CONTEXT 与 ADR-0206 把动作准入的限制一源写作「内部合规限制」，代码里参与准入的 `domain.RegulatoryRestriction` 却是监管限制（挂监管决定、只凭责任来源接受的监管结果整条解除）。CONTEXT 里的内部合规限制（覆盖项、限制责任来源、解除条件、部分解除、四格适用性）在代码里没有模型。ADR-0241 裁定：两种限制各自成立、互不改名互不解除，`RegulatoryRestriction` 就是监管限制，内部合规限制另立对象并进准入。

## 证据

- 代码：`internal/customscompliance/domain/action_admission.go` 包注释「三源——内部合规限制、放行门禁核对、外部放行结果」、`AdmissionAnswer` 头注「外部放行不解内部限制」、`JudgeActionAdmission` 头注「外部放行不解内部限制」、`objectScope` 头注「内部限制可以直接落在一个包裹上」，测试注释 `domain/action_admission_test.go` 与 `adapters/postgres/action_admission_test.go` 里的「外部放行不解内部限制」同此；而 `ActionAdmissionInput.Restrictions` 的类型是 `[]RegulatoryRestriction`，`ports.RestrictionScopeView` 交回的也是它。
- 决定：ADR-0241 决定二、三、四；CC CONTEXT「监管限制」「内部合规限制」「动作准入判断」词条（文档自洽 03 改写后）。

## 三类

机制：两种限制的对象与册、准入的合成与内部合规限制待确认 / 冲突答未决。产品策略：无新的。租户取值：限制来源、动作矩阵与解除条件（`PAR-CUS-07`），没登的答未配置。

## 规模

1. 注释改名：S。
2. 内部合规限制模型、册与并进准入：L，随 UC-CC-011 实施（覆盖项逐项、限制责任来源解除决定、部分解除、四格适用性判断、迁移另取号），准入编排读两本册。

## 最小的诚实修法

1. **现在可做**：上列注释里的「内部合规限制」「内部限制」改称监管限制；「外部放行不解内部限制」改为「外部放行不自动解除限制——监管限制要一份引用已接受监管结果的解除」。不改类型名与行为。
2. **UC-CC-011 接线前必做**：另立内部合规限制的领域对象与登记册，不复用 `RegulatoryRestriction` 与限制册的表；准入的限制一源读两本册——任一种动作时点有效且约束本动作即阻断，内部合规限制适用性答待确认或冲突时该动作答未决并指名那条限制，技术未形成照读不动办（ADR-0241 决定三）；补 UC-CC-013 的 AT 一行（内部限制待确认答未决），AT 号接 UC-CC 现有最大号。

## 要裁的

- 监管限制的部分解除（ADR-0241 越权风险点 1）：等 `PAR-CUS-01/02` 给出扣留与解除的范围语义，不在本票。
- 谁来形成监管限制（UC-CC-008 驱动还是 UC-CC-006 接收扣留时派生，ADR-0241 决定五）：归限制写入口接线那张票。
