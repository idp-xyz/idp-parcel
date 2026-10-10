# 13 关务适用性判断的评审尾巴：词条改引规则节、两处旧口径注释、NR 侧同国用例

Category: enhancement
Status: in-progress——2026-10-10 通道 2 认领，分支 `mcp2-rfc13`、基 `1f8b0cea`。此前 ready-for-agent——2026-10-10 通道 1 立（用户授权自决），出自 [12](12-cc-customs-applicability-judgment-for-route-candidates.md) 阻断修复两份非作者补评审（通道 3、通道 4，均钉 `18be66e8`）的非阻断项
Blocked by: [11](11-demo-network-adopted-as-reference-configuration.md)（已解：11 已进 main，通道 1 派单时核过 `internal/networkrouting` 与 `internal/customscompliance` 上无他人在途）——不是逻辑依赖，是地盘：11 正在 `internal/networkrouting` 写，等它进 main 再动，免得一个目录两个写入方
归档：不属 [psb/04](../../product-strategy-boundary/issues/04-routing-product-strategy-first-cut.md) 的子票集；放在本目录是因为出自 12 的评审。
地盘：`docs/domain/customs-compliance/CONTEXT.md`「关务适用性判断」词条；`internal/networkrouting/adapters/customscompliance/` 的注释与用例；`cmd/parcel-dispatch/assemble.go` 里 `acceptanceReachability` 的头注。
出处：12 票面 Comments「补评审 ← 通道 3」「补评审 ← 通道 4」；[ADR-0148](../../../docs/adr/0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 决定三；AGENTS.md 红线「单一权威」。

## 做什么

1. CC CONTEXT 词条「关务适用性判断」里状态未知的三格不再自列，改为引规则节，与 `e9e1bbf7` 对不可用理由的处理同形。现状两处各列一遍，且词条那份不带「两端异国」限定：字面上同国＋依赖读不到也落状态未知，与规则三相抵。
2. 注释跟上 12 的修复：
   - `ErrCustomsCatalogUnreadable` 的注释补上经初始路由视图形成的`未决`；今天只写了可达性一侧的`未形成判断`。
   - `acceptanceReachability` 头注里「目录读不到时 CC 如实答状态未知，不冒充满足」改成现行答法：这条装配上目录读不到形成的是`未形成判断`。
3. NR 侧补一例「两端同国＋目录读不到」照常作答。今天只有异国两例，同国这一格只靠 `unknownGap` 只认 `CatalogUnreadable` 守着，改坏了没有 NR 用例会红。用例要能红：让 `unknownGap` 连同国一起上抛，或让 CC 读不到时整批答缺口，它都应失败。

## 不做

- 不改答案代数，不改 12 的分诊裁定三；不改 NR 四条约定（`EvidenceGap`、`HardConstraintOutcome`、`NetworkEvidenceView`、`CustomsApplicabilitySource`）的注释与 network-routing CONTEXT。
- 不处理「日后候选带逐候选端点后混批成真」那条残余风险（通道 4 评审所记）——那是改投影形状时的事，记在这里供那张票引用。

## 完成判据

- [ ] 词条与规则节对状态未知只剩一处定义，「两端异国」限定在。
- [ ] 两处注释与现行答法一致。
- [ ] 同国用例在现状上绿、在上面两种改坏之一上红，写明怎么证的。
